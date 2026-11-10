package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesKeyAndValidManifest(t *testing.T) {
	dir := t.TempDir()
	if err := Install(dir, "http://127.0.0.1:8777/", "ANAHTAR123"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "config.js"))
	if !strings.Contains(string(cfg), `"ANAHTAR123"`) || !strings.Contains(string(cfg), `"http://127.0.0.1:8777"`) {
		t.Fatalf("config.js: %s", cfg)
	}
	mb, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var m struct {
		MV      int `json:"manifest_version"`
		Content []struct {
			JS []string `json:"js"`
		} `json:"content_scripts"`
	}
	if err := json.Unmarshal(mb, &m); err != nil || m.MV != 3 {
		t.Fatalf("manifest: %v %s", err, mb)
	}
	for _, js := range m.Content[0].JS {
		if _, err := os.Stat(filepath.Join(dir, js)); err != nil {
			t.Errorf("manifest dosyası eksik: %s", js)
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "config.js")); fi.Mode().Perm() != 0o600 {
		t.Errorf("config.js izni %v", fi.Mode().Perm())
	}
	// Gömülmemiş ama manifestte/HTML'de geçen dosya kalmamalı.
	for _, f := range []string{"background.js", "popup.html", "popup.js", "sidepanel.html", "sidepanel.js", "wa-pure.js", "content.js"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("eksik: %s", f)
		}
	}
}
