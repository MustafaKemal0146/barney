// Package extension, Chrome uzantısının dosyalarını ikiliye gömer; `barney extension install` bunları
// bir klasöre çıkarır ve eşleşme anahtarını config.js'e yazar.
package extension

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed manifest.json config.js background.js content.js wa-pure.js popup.html popup.js sidepanel.html sidepanel.js icons/icon16.png icons/icon48.png icons/icon128.png
var files embed.FS

// Install uzantıyı dir içine çıkarır. baseURL ve key config.js'e yazılır.
func Install(dir, baseURL, key string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		if p == "config.js" {
			data = []byte(fmt.Sprintf("// barney extension install tarafından yazıldı.\nexport const DEFAULTS = {\n  baseUrl: %q,\n  key: %q,\n};\n", strings.TrimRight(baseURL, "/"), key))
		}
		// Anahtar içerdiği için config.js yalnızca sahibine açık yazılır.
		dst := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600)
	})
}
