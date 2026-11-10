// Package memory bir kişi hakkındaki olguları ve ilişki aşamasını sohbetten çıkarır, saklar.
// Kullanıcı görür ve silebilir; silinenler tekrar eklenmez.
package memory

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MustafaKemal0146/barney/internal/mask"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
)

// Fact bir olgu.
type Fact struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Data diske yazılan hafızadır.
type Data struct {
	Facts     []Fact   `json:"facts"`
	Stage     string   `json:"stage"`
	Dismissed []string `json:"dismissed,omitempty"` // silinen olguların kimlikleri
}

func path(dir string) string { return filepath.Join(dir, "memory.json") }

func id(text string) string {
	h := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(text))))
	return hex.EncodeToString(h[:4])
}

// Load hafızayı okur.
func Load(dir string) Data {
	d := Data{Facts: []Fact{}}
	if b, err := os.ReadFile(path(dir)); err == nil {
		_ = json.Unmarshal(b, &d)
	}
	if d.Facts == nil {
		d.Facts = []Fact{}
	}
	return d
}

func save(dir string, d Data) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(d)
	return os.WriteFile(path(dir), b, 0o600)
}

// Delete bir olguyu siler ve tekrar eklenmesini engeller.
func Delete(dir, factID string) error {
	d := Load(dir)
	keep := d.Facts[:0]
	for _, f := range d.Facts {
		if f.ID == factID {
			d.Dismissed = append(d.Dismissed, f.ID)
			continue
		}
		keep = append(keep, f)
	}
	d.Facts = keep
	return save(dir, d)
}

// SetStage aşamayı kullanıcı değiştirdiğinde saklar.
func SetStage(dir, stage string) error {
	d := Load(dir)
	d.Stage = stage
	return save(dir, d)
}

const maxFacts = 25

// Refresh modelden son sohbete dayanarak olguları ve aşamayı yeniler (tek çağrı). Yalnız sohbette
// açıkça geçen şeyleri çıkarmasını ister.
func Refresh(ctx context.Context, c reply.Caller, dir, owner, partner string, msgs []model.Message) (Data, error) {
	d := Load(dir)
	var b strings.Builder
	m := mask.New()
	start := 0
	if len(msgs) > 300 {
		start = len(msgs) - 300
	}
	for _, x := range msgs[start:] {
		if x.Kind != model.KindText || strings.TrimSpace(x.Text) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", x.Sender, strings.ReplaceAll(m.Apply(x.Text), "\n", " "))
	}
	if b.Len() == 0 {
		return d, errors.New("sohbet boş")
	}
	sys := fmt.Sprintf(`Aşağıdaki sohbetten %s hakkında KALICI ve YARARLI olguları çıkar (sevdikleri/sevmedikleri, işi/okulu, planları, önemli tarihler, hassas konular, ilişki durumu). Yalnızca sohbette açıkça geçenleri yaz; tahmin etme, uydurma. Her olgu kısa bir Türkçe cümle. En fazla 15 olgu.
Ayrıca %s ile %s arasındaki ilişki aşamasını tek kısa ifadeyle ver (ör. "yeni tanışıyorlar", "yakınlaşıyorlar", "buluşma planlıyorlar", "yakın arkadaşlar", "iş ilişkisi").
Çıktı YALNIZCA şu JSON: {"gercekler":["..."],"asama":"..."}`, partner, owner, partner)
	raw, _, err := c.Complete(ctx, sys, b.String(), 900)
	if err != nil {
		return d, err
	}
	a, z := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if a < 0 || z <= a {
		return d, errors.New("model çıktısı geçersiz")
	}
	var o struct {
		Gercekler []string `json:"gercekler"`
		Asama     string   `json:"asama"`
	}
	if err := json.Unmarshal([]byte(raw[a:z+1]), &o); err != nil {
		return d, fmt.Errorf("model çıktısı geçersiz: %w", err)
	}
	dismissed := map[string]bool{}
	for _, x := range d.Dismissed {
		dismissed[x] = true
	}
	have := map[string]bool{}
	for _, f := range d.Facts {
		have[f.ID] = true
	}
	for _, t := range o.Gercekler {
		t = strings.TrimSpace(m.Unmask(t))
		if t == "" || len(t) > 240 {
			continue
		}
		fid := id(t)
		if dismissed[fid] || have[fid] || len(d.Facts) >= maxFacts {
			continue
		}
		d.Facts = append(d.Facts, Fact{ID: fid, Text: t})
		have[fid] = true
	}
	if s := strings.TrimSpace(o.Asama); s != "" && d.Stage == "" {
		d.Stage = s
	}
	return d, save(dir, d)
}

// Texts istemde kullanılacak olgu metinleri.
func Texts(d Data) []string {
	out := make([]string, 0, len(d.Facts))
	for _, f := range d.Facts {
		out = append(out, f.Text)
	}
	return out
}
