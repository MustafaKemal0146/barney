package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/MustafaKemal0146/barney/internal/server"
)

// runDoctor: barney doctor [--port 8777] — kurulumu kontrol eder, hiçbir şey değiştirmez ve
// hiçbir gizli değeri yazdırmaz (API anahtarları için yalnızca "var/yok").
func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	port := fs.Int("port", 8777, "sunucu portu")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ok := true
	line := func(good bool, msg, fix string) {
		mark := "✓"
		if !good {
			mark, ok = "✗", false
		}
		fmt.Printf("  %s %s\n", mark, msg)
		if !good && fix != "" {
			fmt.Printf("      → %s\n", fix)
		}
	}
	dir := dataDir()
	fmt.Println("Barney kurulum kontrolü")

	secret, err := server.LoadSecret(dir)
	line(err == nil, "eşleşme anahtarı okunuyor", "barney serve ilk çalışmada üretir")

	extDir := filepath.Join(dir, "extension")
	missing := []string{}
	for _, f := range []string{"manifest.json", "background.js", "content.js", "wa-pure.js", "config.js", "icons/icon128.png"} {
		if _, e := os.Stat(filepath.Join(extDir, f)); e != nil {
			missing = append(missing, f)
		}
	}
	line(len(missing) == 0, "uzantı klasörü tam: "+extDir, fmt.Sprintf("eksik %v — barney extension install", missing))

	keys := []string{"DEEPSEEK_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"}
	have := []string{}
	for _, k := range keys {
		if os.Getenv(k) != "" {
			have = append(have, k)
		}
	}
	if len(have) > 0 {
		line(true, fmt.Sprintf("ortam değişkeninde anahtar: %v", have), "")
	}

	if secret == "" {
		return nil
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/state", *port), nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		line(false, "sunucu çalışmıyor", "barney serve --profile <sohbet> --owner \"Adın\"")
	} else {
		defer resp.Body.Close()
		var st struct {
			Connected bool `json:"connected"`
			KeySet    bool `json:"key_set"`
			HasPeople bool `json:"has_people"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&st)
		line(resp.StatusCode == 200, "sunucu yanıt veriyor (127.0.0.1)", "")
		line(st.Connected, "Chrome uzantısı bağlı", "chrome://extensions → paketlenmemiş öğe yükle, sonra web.whatsapp.com'u aç")
		line(st.HasPeople, "en az bir kişi eklenmiş", "arayüzde Kişi ekle ile WhatsApp dışa aktarımını (zip/txt) yükle")
		line(st.KeySet, "model anahtarı tanımlı", "arayüzde Ayarlar → Model anahtarı")
	}
	if ok {
		fmt.Println("Her şey hazır.")
	}
	return nil
}
