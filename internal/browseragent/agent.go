// Package browseragent yapay zekânın WhatsApp Web sekmesini adım adım gezmesini sağlar (okuma amaçlı).
// Model yalnızca kısa bir eylem kümesinden seçer; her eylem burada VE uzantıda beyaz listeyle doğrulanır:
// kaydır, sol listeden sohbet seç, arama kutusuna yaz, mesajları oku. Mesaj yazma/gönderme eylemi yoktur.
package browseragent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/MustafaKemal0146/barney/internal/reply"
)

// Eylemler (beyaz liste).
const (
	ScrollUp     = "scroll_up"
	ScrollDown   = "scroll_down"
	SideDown     = "scroll_side_down"
	ClickChat    = "click_chat"
	TypeSearch   = "type_search"
	ClearSearch  = "clear_search"
	Read         = "read"
	Extract      = "extract"
	Done         = "done"
	Fail         = "fail"
	maxSteps     = 40
	maxSearchLen = 60
)

var allowed = map[string]bool{ScrollUp: true, ScrollDown: true, SideDown: true, ClickChat: true, TypeSearch: true, ClearSearch: true, Read: true, Extract: true, Done: true, Fail: true}

// Row bir görünür liste öğesidir.
type Row struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// Snapshot sayfanın kompakt görünümüdür.
type Snapshot struct {
	Title      string `json:"title"`
	SideRows   []Row  `json:"side_rows"`
	PanelTop   int    `json:"panel_top"`
	PanelH     int    `json:"panel_height"`
	PanelCount int    `json:"panel_rows"`
	AtTop      bool   `json:"at_top"`
	HasSearch  bool   `json:"has_search"`
	TextTail   string `json:"text_tail,omitempty"`
	Read       int    `json:"read_total"`
}

// Msg çıkarma yedeğinde modelin döndürdüğü mesajdır.
type Msg struct {
	From string `json:"from"` // me | them
	Text string `json:"text"`
	TS   string `json:"ts,omitempty"` // ISO8601, bilinmiyorsa boş
}

// Action modelin kararı.
type Action struct {
	Action   string `json:"eylem"`
	Ref      string `json:"ref,omitempty"`
	Text     string `json:"metin,omitempty"`
	Why      string `json:"neden,omitempty"`
	Messages []Msg  `json:"mesajlar,omitempty"`
}

// Driver tarayıcı tarafıdır (uzantı komutları).
type Driver interface {
	Snapshot(ctx context.Context, withText bool) (Snapshot, error)
	Act(ctx context.Context, a Action) (note string, err error)
	Log(line string)
}

// Goal ajanın görevi.
type Goal struct {
	OpenName    string // doluysa önce bu sohbeti aç
	MaxMessages int
	All         bool
}

// Result ajanın topladığı mesajlar: Extracted yalnızca çıkarma yedeği kullanıldıysa dolu.
type Result struct {
	Extracted []Msg
	Steps     int
	Calls     int
}

const system = `Sen WhatsApp Web sekmesini gezen bir okuma ajanısın. YALNIZCA şu eylemleri kullanabilirsin (JSON): 
{"eylem":"read|scroll_up|scroll_down|scroll_side_down|click_chat|type_search|clear_search|extract|done|fail","ref":"...","metin":"...","neden":"kısa"}
- read: görünür mesajları kaydet (her kaydırmadan sonra yap).
- scroll_up: sohbeti yukarı kaydır (eski mesajlar yüklenir). at_top=true ve okunan sayı artmıyorsa bitti.
- scroll_side_down: sol sohbet listesini aşağı kaydır. click_chat: sol listeden ref ile sohbet seç. type_search: arama kutusuna sohbet adını yaz (≤60 karakter), clear_search: temizle.
- extract: "read" hiç mesaj bulamıyorsa (okunan=0) ve text_tail doluysa, görünen metinden mesajları çıkar: {"eylem":"extract","mesajlar":[{"from":"me|them","text":"...","ts":"ISO8601 ya da boş"}]}.
- done: görev tamam. fail: yapılamıyor (neden yaz).
KURALLAR: Mesaj YAZAMAZ, GÖNDEREMEZSİN; başka eylem yok. Yalnızca verilen ref'leri kullan. Her yanıtta TEK JSON nesnesi, başka metin yok.`

// Run görevi yürütür.
func Run(ctx context.Context, c reply.Caller, d Driver, g Goal) (Result, error) {
	var res Result
	var hist []string
	opened := g.OpenName == ""
	readZero := 0
	var prevRead = -1
	stuck := 0
	for res.Steps = 0; res.Steps < maxSteps; res.Steps++ {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		snap, err := d.Snapshot(ctx, readZero >= 2)
		if err != nil {
			return res, err
		}
		goal := "Açık sohbetin geçmişini oku."
		if !opened {
			goal = fmt.Sprintf("Önce sol listeden %q sohbetini aç (listede yoksa arama kutusuna yaz), sonra geçmişini oku.", g.OpenName)
		}
		if !g.All && g.MaxMessages > 0 {
			goal += fmt.Sprintf(" En fazla ~%d mesaj oku; sayıya ulaşınca done de.", g.MaxMessages)
		}
		sb, _ := json.Marshal(snap)
		last := hist
		if len(last) > 6 {
			last = last[len(last)-6:]
		}
		user := fmt.Sprintf("GÖREV: %s\nDURUM: %s\nSON EYLEMLER: %s\nTek JSON eylemi ver.", goal, sb, strings.Join(last, " | "))
		raw, _, err := c.Complete(ctx, system, user, 900)
		res.Calls++
		if err != nil {
			return res, err
		}
		a, err := parse(raw)
		if err != nil {
			hist = append(hist, "geçersiz çıktı")
			if res.Calls > maxSteps+5 {
				return res, errors.New("model geçerli eylem üretemedi")
			}
			continue
		}
		if err := validate(a, snap); err != nil {
			hist = append(hist, "reddedildi: "+err.Error())
			d.Log("reddedilen eylem: " + err.Error())
			continue
		}
		d.Log(fmt.Sprintf("Adım %d: %s %s", res.Steps+1, a.Action, strings.TrimSpace(a.Why)))
		switch a.Action {
		case Done:
			return res, nil
		case Fail:
			return res, fmt.Errorf("ajan görevi yapamadı: %s", a.Why)
		case Extract:
			res.Extracted = append(res.Extracted, a.Messages...)
			hist = append(hist, fmt.Sprintf("extract(%d)", len(a.Messages)))
			continue
		}
		note, err := d.Act(ctx, a)
		if err != nil {
			hist = append(hist, a.Action+" hata: "+err.Error())
			d.Log("eylem hatası: " + err.Error())
			continue
		}
		hist = append(hist, a.Action+" → "+note)
		if a.Action == ClickChat {
			opened = true
		}
		if a.Action == Read {
			if snap.Read == prevRead && snap.AtTop {
				stuck++
			}
			prevRead = snap.Read
			if snap.Read == 0 {
				readZero++
			}
		}
		if stuck >= 2 { // başa ulaşıldı ve yeni mesaj yok: modeli beklemeden bitir
			return res, nil
		}
	}
	return res, errors.New("ajan adım sınırına ulaştı")
}

func parse(raw string) (Action, error) {
	var a Action
	i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if i < 0 || j <= i {
		return a, errors.New("JSON yok")
	}
	if err := json.Unmarshal([]byte(raw[i:j+1]), &a); err != nil {
		return a, err
	}
	return a, nil
}

func validate(a Action, s Snapshot) error {
	if !allowed[a.Action] {
		return fmt.Errorf("izin verilmeyen eylem %q", a.Action)
	}
	if a.Action == ClickChat {
		for _, r := range s.SideRows {
			if r.Ref == a.Ref {
				return nil
			}
		}
		return fmt.Errorf("bilinmeyen ref %q", a.Ref)
	}
	if a.Action == TypeSearch {
		t := strings.TrimSpace(a.Text)
		if t == "" || len([]rune(t)) > maxSearchLen || strings.ContainsAny(t, "\n\r") {
			return errors.New("arama metni geçersiz")
		}
	}
	if a.Action == Extract && len(a.Messages) > 400 {
		return errors.New("çok fazla mesaj")
	}
	return nil
}
