// Package history öneri geçmişini ve geri bildirimi (düzeltmeleri) kişi klasöründe tutar.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/MustafaKemal0146/barney/internal/reply"
)

// Kind kayıt türü.
const (
	Suggested = "suggested"
	Sent      = "sent"     // öneri olduğu gibi gönderildi
	Edited    = "edited"   // düzenlenip gönderildi
	Rejected  = "rejected" // reddedildi
	Approved  = "approved" // otomatik moddaki bekleyen cevap onaylandı
)

// Entry bir kayıt.
type Entry struct {
	TS    int64  `json:"ts"`
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Final string `json:"final,omitempty"`
}

func path(dir string) string { return filepath.Join(dir, "history.jsonl") }

// Add kayıt ekler.
func Add(dir string, e Entry) error {
	if e.TS == 0 {
		e.TS = time.Now().Unix()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path(dir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, err = f.Write(append(b, '\n'))
	return err
}

// List tüm kayıtları (eskiden yeniye) verir.
func List(dir string) ([]Entry, error) {
	f, err := os.Open(path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Entry{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Kind != "" {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Examples son n düzeltmeyi/reddi örnek olarak verir (geri bildirimden öğrenme).
func Examples(dir string, n int) []reply.Example {
	all, _ := List(dir)
	var ex []reply.Example
	for i := len(all) - 1; i >= 0 && len(ex) < n; i-- {
		switch all[i].Kind {
		case Edited:
			ex = append(ex, reply.Example{Suggested: all[i].Text, Final: all[i].Final})
		case Rejected:
			ex = append(ex, reply.Example{Suggested: all[i].Text})
		}
	}
	return ex
}

// Clear geçmişi siler.
func Clear(dir string) error {
	err := os.Remove(path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
