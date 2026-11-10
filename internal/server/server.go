// Package server, Barney'nin yerel (yalnızca loopback) sunucusudur: Chrome uzantısı WebSocket ile
// bağlanır, "yeni mesaj" olaylarını Gözcü'ye (watch) iletir, Gözcü'nün eylemlerini (öner/gönder)
// uzantıya geri yollar.
//
// Güvenlik: yalnızca 127.0.0.1/localhost Host'u kabul edilir (DNS rebinding), WebSocket Origin'i boş
// ya da chrome-extension:// olmalıdır ve eşleşme anahtarı sabit zamanlı karşılaştırılır.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/MustafaKemal0146/barney/internal/history"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/watch"
)

// Subprotocol WebSocket protokol adıdır; ikinci alt protokol eşleşme anahtarıdır.
const Subprotocol = "barney.v1"

// LoadSecret eşleşme anahtarını diskten okur, yoksa üretip 0600 ile kaydeder.
func LoadSecret(dir string) (string, error) {
	path := filepath.Join(dir, "pairing.json")
	if b, err := os.ReadFile(path); err == nil {
		var v struct{ Secret string }
		if json.Unmarshal(b, &v) == nil && len(v.Secret) >= 32 {
			return v.Secret, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]string{"Secret": secret})
	return secret, os.WriteFile(path, b, 0o600)
}

// ProfileInfo kurulum sayfasında gösterilen stil kartı özetidir.
type ProfileInfo struct {
	Owner      string `json:"owner"`
	Messages   int    `json:"messages"`
	Confidence string `json:"confidence"`
}

// Info kurulum sayfasının gösterdiği, sunucu başlarken bilinen bilgilerdir.
type Info struct {
	ExtDir        string       `json:"ext_dir"`
	Profile       *ProfileInfo `json:"profile"` // nil: stil kartı yok
	Allow         []string     `json:"allow"`
	Provider      string       `json:"provider"`
	Providers     []string     `json:"providers"` // arayüzde seçilebilen sağlayıcılar
	KeySet        bool         `json:"key_set"`
	Council       bool         `json:"council"`
	AutoCfg       bool         `json:"auto_send_configured"`
	Model         string       `json:"model"`
	Notifications bool         `json:"notifications"`
	LearnFeedback bool         `json:"learn_feedback"`
	LAN           bool         `json:"lan"`
	History       HistoryCfg   `json:"history"`
	AutoLearn     bool         `json:"auto_learn"`
	BulkSpeed     string       `json:"bulk_speed"`
}

// HistoryCfg geçmiş okuma derinliği ayarıdır.
type HistoryCfg struct {
	MaxMessages int  `json:"max_messages"`
	MaxSeconds  int  `json:"max_seconds"`
	All         bool `json:"all"`
}

// Server uzantı köprüsüdür.
type Server struct {
	Secret    string
	OwnerName string
	LogPath   string // gönderilen her mesajın JSONL günlüğü (boşsa yazılmaz)
	Info      Info
	// OnSettings arayüzden gelen sağlayıcı/anahtar/ayarı uygular (doğrular ve kaydeder).
	OnSettings func(SettingsIn) error
	Svc        Services
	Web        fs.FS // gömülü arayüz (boşsa "make web" uyarısı)

	imp       importer
	learnAt   map[string]time.Time
	learnNext map[string]wireChat
	hub       hub
	live      *liveState
	lastCands map[string][]string // sohbet → son önerilen metinler (gönderim eşleştirmesi)
	autoSent  map[string]bool     // sohbet → otomatik gönderim planlandı

	mu    sync.Mutex
	brain *watch.Brain
	conn  *websocket.Conn
}

// UpdateInfo arayüzde gösterilen bilgiyi kilit altında günceller.
func (s *Server) UpdateInfo(f func(*Info)) { s.mu.Lock(); f(&s.Info); s.mu.Unlock() }

// SetBrain Gözcü'yü bağlar. Brain'in emit işlevi Push olmalıdır.
func (s *Server) SetBrain(b *watch.Brain) { s.mu.Lock(); s.brain = b; s.mu.Unlock() }

// SetKeyState arayüzde gösterilen sağlayıcı ve "anahtar var mı" bilgisini günceller.
func (s *Server) SetKeyState(provider string, set bool) {
	s.mu.Lock()
	s.Info.Provider, s.Info.KeySet = provider, set
	s.mu.Unlock()
}

func (s *Server) getBrain() *watch.Brain { s.mu.Lock(); defer s.mu.Unlock(); return s.brain }

// cookieName kurulum sayfasının oturum çerezidir.
const cookieName = "barney_session"

// Handler HTTP yönlendiricisini verir.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", s.handleWS)
	s.apiRoutes(mux)
	mux.Handle("GET /", s.spaHandler())
	return s.hostGuard(mux)
}

// authed yalnızca Authorization: Bearer <anahtar> kabul eder (arayüz anahtarı #token= ile alır;
// çerez kullanılmadığı için CSRF yüzeyi yoktur).
func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); tok != "" && s.authOK(tok) {
			h(w, r)
			return
		}
		http.Error(w, "yetkisiz", http.StatusUnauthorized)
	}
}

func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		switch strings.Trim(host, "[]") {
		case "127.0.0.1", "localhost", "::1":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "yalnızca yerel erişim", http.StatusForbidden)
		}
	})
}

func (s *Server) authOK(got string) bool {
	return len(got) == len(s.Secret) && subtle.ConstantTimeCompare([]byte(got), []byte(s.Secret)) == 1
}

// ---- tel biçimi

type wireMsg struct {
	From string `json:"from"` // "me" | "them"
	Text string `json:"text"`
	TS   int64  `json:"ts"` // unix saniye (0 = bilinmiyor)
	Kind string `json:"kind,omitempty"`
}

type wireChat struct {
	ID          string    `json:"id"`
	Partner     string    `json:"partner"`
	History     []wireMsg `json:"history"`
	IncomingKey string    `json:"incoming_key"`
}

type inFrame struct {
	Type     string   `json:"type"`
	Chat     wireChat `json:"chat"`
	ChatID   string   `json:"chat_id"`
	Index    int      `json:"index"`
	Text     string   `json:"text"`
	Reason   string   `json:"reason"`
	Platform string   `json:"platform"`
	// tarayıcıdan okuma / komut sonuçları
	JobID    string          `json:"job_id"`
	Seq      int             `json:"seq"`
	Partner  string          `json:"partner"`
	Owner    string          `json:"owner"`
	Messages []wireMsg       `json:"messages"`
	Loaded   int             `json:"loaded"`
	Note     string          `json:"note"`
	ChatName string          `json:"chat_name"`
	I        int             `json:"i"`
	N        int             `json:"n"`
	Chats    []chatRow       `json:"chats"`
	OK       bool            `json:"ok"`
	Error    string          `json:"error"`
	Fallback bool            `json:"fallback"`
	Data     json.RawMessage `json:"data"`
}

type chatRow struct {
	Name   string `json:"name"`
	Last   string `json:"last"`
	Unread int    `json:"unread"`
}

type outFrame struct {
	Type   string         `json:"type"`
	JobID  string         `json:"job_id,omitempty"`
	Cmd    string         `json:"cmd,omitempty"`
	Args   map[string]any `json:"args,omitempty"`
	Title  string         `json:"title,omitempty"`
	Body   string         `json:"body,omitempty"`
	Action *watch.Action  `json:"action,omitempty"`
	Stats  *watch.Stats   `json:"stats,omitempty"`
}

func (s *Server) toChat(w wireChat) watch.Chat {
	c := watch.Chat{ID: w.ID, Partner: w.Partner, IncomingKey: w.IncomingKey}
	const maxHistory = 60
	h := w.History
	if len(h) > maxHistory {
		h = h[len(h)-maxHistory:]
	}
	for _, m := range h {
		mm := model.Message{Kind: model.KindText, Text: m.Text}
		if m.Kind != "" {
			mm.Kind = m.Kind
		}
		if m.TS > 0 {
			mm.TS = time.Unix(m.TS, 0)
		}
		if m.From == "me" {
			mm.Sender = s.OwnerName
		} else {
			mm.Sender = w.Partner
		}
		c.History = append(c.History, mm)
	}
	return c
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "chrome-extension://") {
		http.Error(w, "geçersiz kaynak", http.StatusForbidden)
		return
	}
	key := ""
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if p = strings.TrimSpace(p); p != Subprotocol && p != "" {
			key = p
		}
	}
	if !s.authOK(key) {
		http.Error(w, "yetkisiz", http.StatusUnauthorized)
		return
	}
	// Origin yukarıda elle denetlendi; kütüphanenin aynı-kaynak denetimi chrome-extension:// ile çalışmaz.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Subprotocol}, InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c.SetReadLimit(1 << 20)
	s.mu.Lock()
	if s.conn != nil {
		_ = s.conn.Close(websocket.StatusPolicyViolation, "yeni bağlantı")
	}
	s.conn = c
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.conn == c {
			s.conn = nil
		}
		s.mu.Unlock()
		_ = c.CloseNow()
	}()
	ctx := r.Context()
	st := s.getBrain().Stats()
	s.write(ctx, c, outFrame{Type: "hello_ack", Stats: &st})
	for {
		var f inFrame
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		s.handleFrame(ctx, c, f)
	}
}

func (s *Server) handleFrame(ctx context.Context, c *websocket.Conn, f inFrame) {
	b := s.getBrain()
	switch f.Type {
	case "incoming":
		s.setLive(f.Chat, f.Platform)
		b.OnIncoming(s.toChat(f.Chat))
	case "live_update": // yalnız görünüm + öğrenme; Gözcü/yapay zekâ tetiklenmez
		s.setLive(f.Chat, f.Platform)
	case "sync_live":
		s.resyncLive(f)
	case "manual":
		s.setLive(f.Chat, f.Platform)
		b.Manual(s.toChat(f.Chat))
	case "history_progress", "history_chunk", "chats", "command_result", "command_error":
		s.importFrame(f)
	case "pause":
		b.Pause()
	case "resume":
		b.Resume()
	case "sent":
		s.logLine(map[string]any{"event": "sent", "chat_id": f.ChatID, "index": f.Index, "text": f.Text})
		s.onSent(f.ChatID, f.Text)
	case "aborted":
		s.logLine(map[string]any{"event": "aborted", "chat_id": f.ChatID, "reason": f.Reason})
		s.hub.publish(map[string]any{"type": "aborted", "chat_id": f.ChatID, "reason": f.Reason})
	}
	if f.Type == "pause" || f.Type == "resume" || f.Type == "state" {
		st := b.Stats()
		s.write(ctx, c, outFrame{Type: "state", Stats: &st})
		s.publishState()
	}
}

func (s *Server) write(ctx context.Context, c *websocket.Conn, f outFrame) {
	data, _ := json.Marshal(f)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = c.Write(ctx, websocket.MessageText, data)
}

// Push Gözcü eylemini arayüze (SSE) duyurur ve gerekiyorsa bağlı uzantıya yollar.
func (s *Server) Push(a watch.Action) { s.pushAction(a, true) }

func (s *Server) pushAction(a watch.Action, fromBrain bool) {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	personID := ""
	if v, ok := s.Svc.People.Find(a.ChatID); ok {
		personID = v.ID
	}
	if fromBrain {
		switch a.Kind {
		case "suggest", "send":
			d := watch.Decision{Cands: a.Cands, Recommended: a.Recommended}
			if a.Kind == "send" {
				// otomatik gönderim: öneri kartı da arayüzde görünsün
				d.Cands = []watch.Cand{{Tur: "otomatik", Steps: a.Steps, Confident: true}}
				d.Recommended = 0
			}
			s.remember(personID, a.ChatID, d)
			s.hub.publish(map[string]any{"type": "suggestions", "chat_id": a.ChatID, "incoming_key": a.IncomingKey, "person_id": personID,
				"cands": candsOut(d), "recommended": d.Recommended, "note": a.Reason, "pending_approval": a.PendingApproval})
			s.mu.Lock()
			notify := s.Info.Notifications
			s.mu.Unlock()
			if notify && a.Kind == "suggest" {
				s.hub.publish(map[string]any{"type": "notify", "title": a.ChatID, "body": "Cevap önerisi hazır"})
				if c != nil {
					s.write(context.Background(), c, outFrame{Type: "notify", Title: "Barney · " + a.ChatID, Body: "Cevap önerisi hazır"})
				}
			}
			if a.Kind == "send" {
				s.mu.Lock()
				if s.autoSent == nil {
					s.autoSent = map[string]bool{}
				}
				s.autoSent[a.ChatID] = true
				s.mu.Unlock()
			}
		case "error":
			s.hub.publish(map[string]any{"type": "aborted", "chat_id": a.ChatID, "reason": a.Reason})
		}
		s.publishState()
	}
	if a.Kind == "send" {
		s.logLine(map[string]any{"event": "send_planned", "chat_id": a.ChatID, "steps": a.Steps, "auto": fromBrain})
	}
	if c == nil {
		return
	}
	st := s.getBrain().Stats()
	s.write(context.Background(), c, outFrame{Type: "action", Action: &a, Stats: &st})
}

// onSent uzantı bir mesajı gönderdiğinde: SSE olayı ve geri bildirim kaydı (öneriyle aynıysa "sent",
// farklıysa "edited").
func (s *Server) onSent(chatID, text string) {
	s.mu.Lock()
	auto := s.autoSent[chatID]
	cands := s.lastCands[chatID]
	s.mu.Unlock()
	s.hub.publish(map[string]any{"type": "sent", "chat_id": chatID, "text": text, "auto": auto})
	v, ok := s.Svc.People.Find(chatID)
	if !ok {
		return
	}
	dir := s.Svc.People.Dir(v.ID)
	norm := func(x string) string { return model.Norm(x) }
	for _, c := range cands {
		for _, part := range strings.Split(c, " / ") {
			if norm(part) == norm(text) {
				_ = history.Add(dir, history.Entry{Kind: history.Sent, Text: text})
				return
			}
		}
	}
	if len(cands) > 0 {
		_ = history.Add(dir, history.Entry{Kind: history.Edited, Text: cands[0], Final: text})
	} else {
		_ = history.Add(dir, history.Entry{Kind: history.Sent, Text: text})
	}
}

func (s *Server) logLine(v map[string]any) {
	if s.LogPath == "" {
		return
	}
	v["ts"] = time.Now().Format(time.RFC3339)
	b, _ := json.Marshal(v)
	if err := os.MkdirAll(filepath.Dir(s.LogPath), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(s.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}
