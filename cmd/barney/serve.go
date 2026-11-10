package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MustafaKemal0146/barney/extension"
	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/people"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/server"
	"github.com/MustafaKemal0146/barney/internal/watch"
	"github.com/MustafaKemal0146/barney/web"
)

func dataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "barney")
	}
	return filepath.Join(os.Getenv("HOME"), ".barney")
}

// settings arayüzden girilen ayarlardır; yalnızca sahibine açık dosyada tutulur.
type settings struct {
	Provider           string `json:"provider"`
	Key                string `json:"key"`
	Model              string `json:"model"`
	Notifications      bool   `json:"notifications"`
	LearnFeedback      bool   `json:"learn_feedback"`
	HistoryMaxMessages int    `json:"history_max_messages"`
	HistoryMaxSeconds  int    `json:"history_max_seconds"`
	HistoryAll         bool   `json:"history_all"`
	AutoLearn          bool   `json:"auto_learn"`
	BulkSpeed          string `json:"bulk_speed"`
}

func loadSettings(dir string) settings {
	st := settings{Provider: "deepseek", LearnFeedback: true, HistoryMaxMessages: 3000, HistoryMaxSeconds: 300, AutoLearn: true, BulkSpeed: "yavas"}
	defer func() {}()
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if _, ok := providers[st.Provider]; !ok {
		st.Provider = "deepseek"
	}
	if st.HistoryMaxMessages <= 0 {
		st.HistoryMaxMessages = 3000
	}
	if st.HistoryMaxSeconds <= 0 {
		st.HistoryMaxSeconds = 300
	}
	if st.BulkSpeed != "hizli" {
		st.BulkSpeed = "yavas"
	}
	return st
}

func saveSettings(dir string, st settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(st)
	tmp := filepath.Join(dir, ".settings.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "settings.json"))
}

// holder ayarları kilitli tutar; model çağırıcıyı her istekte güncel ayardan kurar.
type holder struct {
	mu  sync.Mutex
	dir string
	st  settings
}

func (h *holder) get() settings { h.mu.Lock(); defer h.mu.Unlock(); return h.st }

// effective sağlayıcıyı, modeli ve anahtarı (ortam değişkeni önce) çözer.
func (h *holder) effective() (provider, string, string) {
	st := h.get()
	p := providers[st.Provider]
	key := os.Getenv(p.keyEnv)
	if key == "" {
		key = st.Key
	}
	mdl := st.Model
	if mdl == "" {
		mdl = p.model
	}
	return p, key, mdl
}

func (h *holder) caller() (reply.Caller, error) {
	p, key, mdl := h.effective()
	if key == "" {
		return nil, errors.New("API anahtarı yok: Ayarlar sayfasından " + p.id + " anahtarını gir")
	}
	return reply.LLMCaller{Transport: llm.NewTransport(p.id, p.baseURL, key), Model: mdl, Temperature: 0.8}, nil
}

// runServe: barney serve [--port 8777]. Kişiler, anahtar ve ayarlar arayüzden yönetilir.
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8777, "yerel port (yalnız 127.0.0.1)")
	settle := fs.Int("settle", 8, "son mesajdan sonra beklenecek saniye (art arda mesajlar tek çağrı olur)")
	perDay := fs.Int("max-calls-per-day", 200, "günde en çok Gözcü yapay zekâ çağrısı")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := dataDir()
	secret, err := server.LoadSecret(dir)
	if err != nil {
		return err
	}
	store, err := people.Open(filepath.Join(dir, "people"))
	if err != nil {
		return err
	}
	h := &holder{dir: dir, st: loadSettings(dir)}

	extDir := filepath.Join(dir, "extension")
	if err := extension.Install(extDir, fmt.Sprintf("http://127.0.0.1:%d", *port), secret); err != nil {
		return err
	}
	webFS, werr := web.FS()
	srv := &server.Server{Secret: secret, OwnerName: "Siz", LogPath: filepath.Join(dir, "sent.jsonl"),
		Svc: server.Services{People: store, Caller: h.caller, Timing: reply.DefaultTiming}}
	if werr == nil {
		srv.Web = webFS
	}
	refreshInfo := func() {
		p, key, mdl := h.effective()
		st := h.get()
		srv.UpdateInfo(func(i *server.Info) {
			i.ExtDir, i.Provider, i.Model, i.KeySet = extDir, p.id, mdl, key != ""
			i.Notifications, i.LearnFeedback = st.Notifications, st.LearnFeedback
			i.History = server.HistoryCfg{MaxMessages: st.HistoryMaxMessages, MaxSeconds: st.HistoryMaxSeconds, All: st.HistoryAll}
			i.AutoLearn, i.BulkSpeed = st.AutoLearn, st.BulkSpeed
			i.Providers = nil
			for n := range providers {
				i.Providers = append(i.Providers, n)
			}
			sort.Strings(i.Providers)
		})
	}
	refreshInfo()
	srv.OnSettings = func(in server.SettingsIn) error {
		h.mu.Lock()
		st := h.st
		if in.Provider != "" {
			if _, ok := providers[in.Provider]; !ok {
				h.mu.Unlock()
				return fmt.Errorf("bilinmeyen sağlayıcı %q", in.Provider)
			}
			if in.Provider != st.Provider {
				st.Provider, st.Key, st.Model = in.Provider, "", "" // anahtar sağlayıcıya özeldir
			}
		}
		if in.Key != nil {
			k := *in.Key
			if k != "" && (len(k) < 8 || len(k) > 400 || strings.ContainsAny(k, " \t\r\n")) {
				h.mu.Unlock()
				return errors.New("anahtar biçimi geçersiz görünüyor (boşluk içermemeli)")
			}
			st.Key = k
		}
		if in.Model != nil {
			st.Model = strings.TrimSpace(*in.Model)
		}
		if in.Notifications != nil {
			st.Notifications = *in.Notifications
		}
		if in.LearnFeedback != nil {
			st.LearnFeedback = *in.LearnFeedback
		}
		if in.HistoryMaxMessages != nil {
			st.HistoryMaxMessages = max(50, min(100000, *in.HistoryMaxMessages))
		}
		if in.HistoryMaxSeconds != nil {
			st.HistoryMaxSeconds = max(15, min(3600, *in.HistoryMaxSeconds))
		}
		if in.HistoryAll != nil {
			st.HistoryAll = *in.HistoryAll
		}
		if in.AutoLearn != nil {
			st.AutoLearn = *in.AutoLearn
		}
		if in.BulkSpeed != nil {
			if *in.BulkSpeed == "hizli" {
				st.BulkSpeed = "hizli"
			} else {
				st.BulkSpeed = "yavas"
			}
		}
		if err := saveSettings(dir, st); err != nil {
			h.mu.Unlock()
			return err
		}
		h.st = st
		h.mu.Unlock()
		refreshInfo()
		return nil
	}
	brain := watch.New(watch.Config{Settle: time.Duration(*settle) * time.Second, MaxCallsPerDay: *perDay}, srv.Decider(), srv.Push)
	brain.SetPolicy(srv.Policy)
	srv.SetBrain(brain)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = hs.Shutdown(ctx)
	}()

	fmt.Printf("\nBarney çalışıyor.\n\n  Arayüz (tarayıcıda aç):\n  http://%s/#token=%s\n\n", addr, secret)
	fmt.Printf("kişiler: %d · uzantı klasörü: %s\n", len(store.List()), extDir)
	if werr != nil || srv.Web == nil {
		fmt.Println("uyarı: arayüz dosyaları gömülü değil; önce `make web` ile derle.")
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// runExtension: barney extension install [klasör] [--port 8777]
func runExtension(args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("kullanım: barney extension install [klasör] [--port 8777]")
	}
	fs := flag.NewFlagSet("extension install", flag.ContinueOnError)
	port := fs.Int("port", 8777, "sunucu portu")
	var pos []string
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) > 0 {
			pos, rest = append(pos, rest[0]), rest[1:]
		}
	}
	dir := filepath.Join(dataDir(), "extension")
	if len(pos) > 0 {
		dir = pos[0]
	}
	secret, err := server.LoadSecret(dataDir())
	if err != nil {
		return err
	}
	if err := extension.Install(dir, fmt.Sprintf("http://127.0.0.1:%d", *port), secret); err != nil {
		return err
	}
	fmt.Printf("Uzantı yazıldı: %s\n\nChrome'da: chrome://extensions → Geliştirici modu → \"Paketlenmemiş öğe yükle\" → bu klasörü seç.\nSonra: barney serve --profile <sohbet> --owner \"Adın\" --allow \"Kişi\"\n", dir)
	return nil
}

// failing her çağrıda açık bir hata döndüren karar verici (eksik kurulum).
