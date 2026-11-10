// Package reply, kullanıcının stil kartıyla karşı tarafın son mesajına cevap üretir.
//
// Akış: hedef mesaj sayısı kişinin patlama dağılımından kodla seçilir → tek LLM çağrısı →
// çıktı şemadan ve kart denetiminden (style.Check) geçer → uymazsa ihlaller söylenerek en çok
// MaxRetries kez yeniden denenir → bekleme süreleri LLM'siz hesaplanır. Hâlâ uymuyorsa sonuç
// döner ama Confident=false olur (Gözcü modu bunu otomatik göndermez).
package reply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/mask"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/style"
)

// Caller bir LLM çağrısı yapar. Testlerde sahtesi kullanılır.
type Caller interface {
	Complete(ctx context.Context, system, user string, maxTokens int) (string, llm.Usage, error)
}

// LLMCaller bir llm.Transport'u Caller olarak sarar.
type LLMCaller struct {
	Transport   llm.Transport
	Model       string
	Temperature float64
}

// Complete tek istek-yanıt çağrısı yapar.
func (c LLMCaller) Complete(ctx context.Context, system, user string, maxTokens int) (string, llm.Usage, error) {
	temp := c.Temperature
	return llm.Complete(ctx, c.Transport, llm.Request{
		Model: c.Model, MaxTokens: maxTokens, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: system}, {Role: llm.RoleUser, Content: user}},
	})
}

// Sınırlar.
const (
	MaxRetries = 2
	MaxOut     = 700 // cevap kısadır; fazlası boşa token
	ContextMax = 30  // modele giden son mesaj sayısı
)

// ErrInvalid şemaya uymayan model çıktısını bildirir.
var ErrInvalid = errors.New("model çıktısı şemaya uymuyor")

// Request bir cevap üretme isteğidir.
type Request struct {
	Owner                  style.Card      // kullanıcının kartı (ses)
	Partner                style.Card      // karşı tarafın kartı (boşsa yok sayılır)
	History                []model.Message // sohbetin son mesajları, eskiden yeniye; sonuncusu karşı taraftan olmalı
	OwnerName, PartnerName string
	// Goal isteğe bağlı kısa yönlendirme ("akşam buluşmayı öner" gibi).
	Goal     string
	Opener   bool      // sohbeti yeniden başlatan mesaj: son mesaj sahibin olabilir
	Tone     Tone      // içerik tonu (boş preset = yok)
	Facts    []string  // kişi hakkında bilinenler (hafıza)
	Stage    string    // ilişki aşaması
	Examples []Example // kullanıcının geçmiş düzeltmeleri
	Seed     int64
	Timing   Timing
}

// Result üretilen cevaptır.
type Result struct {
	Steps      []Step    `json:"steps"`
	Note       string    `json:"note,omitempty"`       // modelin kısa gerekçesi
	Violations []string  `json:"violations,omitempty"` // son denemede kalan ihlaller
	Sensitive  bool      `json:"sensitive,omitempty"`  // model hassas konu bildirdi
	Confident  bool      `json:"confident"`            // ihlal yok + kart yeterince güvenilir
	Attempts   int       `json:"attempts"`
	Usage      llm.Usage `json:"usage"`
	Target     int       `json:"target"`
}

type out struct {
	Mesajlar []string `json:"mesajlar"`
	Not      string   `json:"not"`
	Hassas   bool     `json:"hassas,omitempty"`
}

func parse(raw string) (out, error) {
	var o out
	a, b := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if a < 0 || b <= a {
		return o, fmt.Errorf("%w: JSON nesnesi bulunamadı", ErrInvalid)
	}
	dec := json.NewDecoder(strings.NewReader(raw[a : b+1]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return o, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(o.Mesajlar) == 0 {
		return o, fmt.Errorf("%w: mesaj yok", ErrInvalid)
	}
	return o, nil
}

func build(r Request) (system, user string, lo, hi int, m *mask.Masker, rng *rand.Rand) {
	rng = rand.New(rand.NewSource(r.Seed))
	lo, hi = style.PickRange(r.Owner, rng)
	m = mask.New()
	return systemPrompt(r, lo, hi, m), conversationBlock(r, m), lo, hi, m, rng
}

// Prompts modele GİDECEK (maskelenmiş) istemleri çağrı yapmadan verir; "buluta ne gidiyor" önizlemesi içindir.
func Prompts(r Request) (system, user string) {
	system, user, _, _, _, _ = build(r)
	return
}

// Generate cevabı üretir.
func Generate(ctx context.Context, c Caller, r Request) (Result, error) {
	if r.Owner.Messages == 0 {
		return Result{}, errors.New("stil kartı boş: önce sohbet içe aktar")
	}
	if len(r.History) == 0 || (!r.Opener && r.History[len(r.History)-1].Sender == r.OwnerName) {
		return Result{}, errors.New("cevaplanacak bir karşı taraf mesajı yok")
	}
	system, user, lo, hi, m, rng := build(r)

	res := Result{Target: hi}
	feedback := ""
	var lastTexts []string
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		res.Attempts++
		prompt := user
		if feedback != "" {
			prompt += "\n\nÖnceki denemen şu kurallara uymadı, düzelt:\n" + feedback
		}
		raw, usage, err := c.Complete(ctx, system, prompt, MaxOut)
		res.Usage = res.Usage.Add(usage)
		if err != nil {
			return res, err
		}
		o, err := parse(raw)
		if err != nil {
			feedback = "- Çıktı yalnızca {\"mesajlar\":[...],\"not\":\"...\"} biçiminde geçerli bir JSON nesnesi olmalı."
			continue
		}
		texts := make([]string, len(o.Mesajlar))
		for i, t := range o.Mesajlar {
			texts[i] = strings.TrimSpace(m.Unmask(t))
		}
		texts = style.Split(r.Owner, texts, hi)
		lastTexts, res.Note, res.Sensitive = texts, o.Not, o.Hassas
		res.Violations = style.CheckRange(r.Owner, texts, lo, hi)
		if len(res.Violations) == 0 {
			break
		}
		feedback = "- " + strings.Join(res.Violations, "\n- ")
	}
	if lastTexts == nil {
		return res, fmt.Errorf("%w: %d denemede geçerli çıktı alınamadı", ErrInvalid, res.Attempts)
	}
	res.Steps = Plan(r.Owner, lastTexts, rng, r.Timing)
	res.Confident = len(res.Violations) == 0 && r.Owner.Confidence != style.ConfLow
	return res, nil
}
