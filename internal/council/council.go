// Package council, birden çok bakış açısının (persona) paralel görüş verdiği, tek sentezle üç aday
// cevaba dönüştüğü ve bir "Vicdan" denetiminden geçtiği cevap motorudur.
//
// Maliyet: 3 paralel kısa persona çağrısı + 1 sentez + 1 Vicdan denetimi (+ stil ihlali varsa en çok
// MaxRetries sentez tekrarı). "Ses" üyesi LLM'siz, stil kartıyla (style.Check) çalışır. Persona ve
// Vicdan çağrıları ucuz modelle, sentez güçlü modelle yapılabilir.
package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/mask"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

// Sınırlar.
const (
	MaxRetries    = 2
	OpinionMaxOut = 400
	SynthMaxOut   = 1200
	VicdanMaxOut  = 500
)

// ErrInvalid şemaya uymayan model çıktısını bildirir.
var ErrInvalid = errors.New("model çıktısı şemaya uymuyor")

// Options konsey ayarları. Synth boşsa Fast kullanılır.
type Options struct {
	Fast  reply.Caller // persona ve Vicdan çağrıları
	Synth reply.Caller // sentez
}

// Opinion bir personanın görüşüdür.
type Opinion struct {
	Persona string `json:"persona"`
	Ozet    string `json:"ozet"`
	Oneri   string `json:"oneri"`
	Uyari   string `json:"uyari"`
}

// Candidate bir aday cevaptır.
type Candidate struct {
	Tur         string       `json:"tur"`
	Steps       []reply.Step `json:"steps"`
	Violations  []string     `json:"violations,omitempty"`
	Blocked     bool         `json:"blocked"`
	BlockReason string       `json:"block_reason,omitempty"`
	Confident   bool         `json:"confident"` // ihlal yok, Vicdan onayladı, kart güvenilir
}

// Result konsey çıktısı.
type Result struct {
	Candidates  []Candidate `json:"candidates"`
	Recommended int         `json:"recommended"` // -1: otomatik önerilecek aday yok
	Opinions    []Opinion   `json:"opinions"`
	Note        string      `json:"note,omitempty"`
	Warnings    []string    `json:"warnings,omitempty"` // atlanan/başarısız adımlar
	Calls       int         `json:"calls"`
	Usage       llm.Usage   `json:"usage"`
}

func decode(raw string, v any) error {
	a, b := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if a < 0 || b <= a {
		return fmt.Errorf("%w: JSON nesnesi bulunamadı", ErrInvalid)
	}
	dec := json.NewDecoder(strings.NewReader(raw[a : b+1]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

type tally struct {
	mu    sync.Mutex
	calls int
	use   llm.Usage
}

func (t *tally) add(u llm.Usage) {
	t.mu.Lock()
	t.calls++
	t.use = t.use.Add(u)
	t.mu.Unlock()
}

// Run konseyi çalıştırır.
func Run(ctx context.Context, o Options, r reply.Request) (Result, error) {
	if o.Fast == nil {
		return Result{}, errors.New("konsey için model yok")
	}
	if o.Synth == nil {
		o.Synth = o.Fast
	}
	if r.Owner.Messages == 0 {
		return Result{}, errors.New("stil kartı boş: önce sohbet içe aktar")
	}
	if n := len(r.History); n == 0 || r.History[n-1].Sender == r.OwnerName {
		return Result{}, errors.New("cevaplanacak bir karşı taraf mesajı yok")
	}
	m := mask.New()
	var tl tally
	res := Result{Recommended: -1}

	// 1) Persona görüşleri paralel; biri başarısız olsa konsey sürer.
	ops := make([]*Opinion, len(Personas))
	warns := make([]string, len(Personas))
	convo := reply.ConversationBlock(r, m)
	// Masker eşzamanlı kullanılamaz: istemler goroutine'lerden önce, sırayla hazırlanır.
	sysPrompts := make([]string, len(Personas))
	for i, p := range Personas {
		sysPrompts[i] = p.Prompt + "\n\n" + personaContext(r, m)
	}
	var wg sync.WaitGroup
	for i, p := range Personas {
		wg.Add(1)
		go func(i int, p Persona) {
			defer wg.Done()
			sys := sysPrompts[i]
			raw, u, err := o.Fast.Complete(ctx, sys, convo, OpinionMaxOut)
			tl.add(u)
			if err != nil {
				warns[i] = p.Name + ": " + err.Error()
				return
			}
			var op Opinion
			if err := decode(raw, &op); err != nil {
				warns[i] = p.Name + ": " + err.Error()
				return
			}
			op.Persona = p.Name
			ops[i] = &op
		}(i, p)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return res, err
	}
	for i, op := range ops {
		if op != nil {
			res.Opinions = append(res.Opinions, *op)
		} else if warns[i] != "" {
			res.Warnings = append(res.Warnings, warns[i])
		}
	}
	if len(res.Opinions) == 0 {
		fillUsage(&res, &tl)
		return res, fmt.Errorf("hiçbir persona görüş veremedi: %s", strings.Join(res.Warnings, "; "))
	}

	// 2) Sentez: üç aday, her biri kendi hedef mesaj sayısıyla; ihlalli adaylar geri bildirimle yenilenir.
	rng := rand.New(rand.NewSource(r.Seed))
	targets := map[string][2]int{}
	for _, t := range Turler {
		lo, hi := style.PickRange(r.Owner, rng)
		targets[t] = [2]int{lo, hi}
	}
	best := map[string]*Candidate{}
	var texts = map[string][]string{}
	feedback := ""
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		sys := synthPrompt(r, m, res.Opinions, targets)
		user := convo
		if feedback != "" {
			user += "\n\nÖnceki denemende şu kurallara uyulmadı; tüm adayları düzelterek yeniden yaz:\n" + feedback
		}
		raw, u, err := o.Synth.Complete(ctx, sys, user, SynthMaxOut)
		tl.add(u)
		if err != nil {
			if len(best) == 0 {
				fillUsage(&res, &tl)
				return res, err
			}
			res.Warnings = append(res.Warnings, "sentez yenilenemedi: "+err.Error())
			break
		}
		var out synthOut
		if err := decode(raw, &out); err != nil || len(out.Adaylar) == 0 {
			feedback = `- Çıktı YALNIZCA {"adaylar":[{"tur":"guvenli","mesajlar":["..."]},...],"not":"..."} biçiminde geçerli JSON olmalı.`
			continue
		}
		res.Note = out.Not
		var fb []string
		got := map[string]bool{}
		for _, a := range out.Adaylar {
			tgt, ok := targets[a.Tur]
			if !ok || got[a.Tur] {
				continue
			}
			got[a.Tur] = true
			tx := make([]string, len(a.Mesajlar))
			for i, t := range a.Mesajlar {
				tx[i] = strings.TrimSpace(m.Unmask(t))
			}
			tx = style.Split(r.Owner, tx, tgt[1])
			v := style.CheckRange(r.Owner, tx, tgt[0], tgt[1])
			// Önceki denemeden daha az ihlalli olan korunur.
			if prev := best[a.Tur]; prev == nil || len(v) < len(prev.Violations) {
				best[a.Tur] = &Candidate{Tur: a.Tur, Violations: v}
				texts[a.Tur] = tx
			}
			for _, x := range v {
				fb = append(fb, fmt.Sprintf("%s: %s", a.Tur, x))
			}
		}
		for _, t := range Turler {
			if !got[t] && best[t] == nil {
				fb = append(fb, t+": aday eksik")
			}
		}
		if len(fb) == 0 {
			feedback = ""
			break
		}
		feedback = "- " + strings.Join(fb, "\n- ")
	}
	if len(best) == 0 {
		fillUsage(&res, &tl)
		return res, fmt.Errorf("%w: sentezden geçerli aday alınamadı", ErrInvalid)
	}

	// Bekleme süreleri kodla; aday başına ayrı tohum akışı.
	for _, t := range Turler {
		c := best[t]
		if c == nil {
			res.Warnings = append(res.Warnings, t+" adayı üretilemedi")
			continue
		}
		c.Steps = reply.Plan(r.Owner, texts[t], rng, r.Timing)
		res.Candidates = append(res.Candidates, *c)
	}

	// 3) Vicdan denetimi: uygun değilse aday engellenir. Denetim yapılamazsa hiçbir aday "güvenilir" olmaz.
	vicdanOK := true
	verdicts, err := vicdan(ctx, o.Fast, r, m, convo, res.Candidates, &tl)
	if err != nil {
		vicdanOK = false
		res.Warnings = append(res.Warnings, "Vicdan denetimi yapılamadı: "+err.Error())
	}
	for i := range res.Candidates {
		c := &res.Candidates[i]
		if v, ok := verdicts[c.Tur]; ok && !v.Uygun {
			c.Blocked, c.BlockReason = true, v.Sebep
		} else if !ok && vicdanOK {
			c.Blocked, c.BlockReason = true, "Vicdan bu aday için karar vermedi"
		}
		c.Confident = vicdanOK && !c.Blocked && len(c.Violations) == 0 && r.Owner.Confidence != style.ConfLow
	}
	for _, t := range Turler { // öneri sırası: güvenli, komik, cesur
		for i, c := range res.Candidates {
			if c.Tur == t && c.Confident && res.Recommended < 0 {
				res.Recommended = i
			}
		}
	}
	fillUsage(&res, &tl)
	return res, nil
}

func fillUsage(r *Result, t *tally) {
	t.mu.Lock()
	r.Calls, r.Usage = t.calls, t.use
	t.mu.Unlock()
}

func personaContext(r reply.Request, m *mask.Masker) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Kullanıcı: %s. Karşı taraf: %s.\n", r.OwnerName, r.PartnerName)
	if r.Partner.Messages > 0 {
		fmt.Fprintf(&b, "KARŞI TARAFIN YAZMA BİÇİMİ\n%s", m.Apply(reply.Describe(r.Partner)))
	}
	fmt.Fprintf(&b, "KULLANICININ YAZMA BİÇİMİ\n%s", m.Apply(reply.Describe(r.Owner)))
	if g := strings.TrimSpace(r.Goal); g != "" {
		fmt.Fprintf(&b, "KULLANICININ İSTEĞİ: %s\n", m.Apply(g))
	}
	return b.String()
}

type synthOut struct {
	Adaylar []struct {
		Tur      string   `json:"tur"`
		Mesajlar []string `json:"mesajlar"`
	} `json:"adaylar"`
	Not string `json:"not"`
}

func synthPrompt(r reply.Request, m *mask.Masker, ops []Opinion, targets map[string][2]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sen %s adlı kişinin yerine mesaj yazan bir yazım yardımcısısın. Aşağıdaki konsey görüşlerini birleştirip karşı tarafın son mesajına bu kişinin kendi sesiyle ÜÇ aday cevap yaz.\n\nKURALLAR\n", r.OwnerName)
	rng := func(t string) string { return fmt.Sprintf("%d-%d ayrı mesaj", targets[t][0], targets[t][1]) }
	fmt.Fprintf(&b, "- \"guvenli\": sıcak, risksiz, doğal. %s.\n", rng(TurGuvenli))
	fmt.Fprintf(&b, "- \"komik\": kişinin üslubuna uyan ince mizah (espri uygun değilse yine hafif ve sıcak). %s.\n", rng(TurKomik))
	fmt.Fprintf(&b, "- \"cesur\": daha net bir hamle (teklif, soru, ilgi belirtme) ama saygılı ve baskısız. %s.\n", rng(TurCesur))
	if r.Owner.Multi() {
		b.WriteString("- Bu kişi tek uzun mesaj atmaz, art arda kısa mesajlar atar: her mesaj tek bir düşünce ya da yarım cümle olsun; bir mesaja birden fazla cümle sığdırma.\n")
	}
	b.WriteString("- Her aday yazma biçimine ve örneklere sadık kalsın; örnek cümleleri kopyalama. Adaylar birbirinden gerçekten farklı olsun.\n")
	b.WriteString("- Konsey Stratejist geri çekilmeyi önerdiyse cesur aday da yumuşak kalsın.\n")
	b.WriteString(reply.SharedRules)
	b.WriteString(`- Çıktı YALNIZCA şu JSON: {"adaylar":[{"tur":"guvenli","mesajlar":["..."]},{"tur":"komik","mesajlar":["..."]},{"tur":"cesur","mesajlar":["..."]}],"not":"kısa gerekçe"}` + "\n\nKONSEY GÖRÜŞLERİ\n")
	for _, o := range ops {
		fmt.Fprintf(&b, "- %s: %s | öneri: %s | uyarı: %s\n", o.Persona, m.Apply(o.Ozet), m.Apply(o.Oneri), m.Apply(o.Uyari))
	}
	b.WriteString("\n" + reply.StyleBlock(r, m))
	return b.String()
}

type verdict struct {
	Tur   string `json:"tur"`
	Uygun bool   `json:"uygun"`
	Sebep string `json:"sebep"`
}

func vicdan(ctx context.Context, c reply.Caller, r reply.Request, m *mask.Masker, convo string, cands []Candidate, tl *tally) (map[string]verdict, error) {
	var b strings.Builder
	b.WriteString(convo + "\n\nADAY CEVAPLAR\n")
	for _, cd := range cands {
		for i, s := range cd.Steps {
			fmt.Fprintf(&b, "%s #%d: %s\n", cd.Tur, i+1, strings.ReplaceAll(m.Apply(s.Text), "\n", " "))
		}
	}
	raw, u, err := c.Complete(ctx, vicdanPrompt, b.String(), VicdanMaxOut)
	tl.add(u)
	if err != nil {
		return nil, err
	}
	var out struct {
		Kararlar []verdict `json:"kararlar"`
	}
	if err := decode(raw, &out); err != nil {
		return nil, err
	}
	res := map[string]verdict{}
	for _, v := range out.Kararlar {
		res[v.Tur] = v
	}
	return res, nil
}

// PersonaPromptPreview bir personaya gidecek (maskelenmiş) istemi çağrı yapmadan verir.
func PersonaPromptPreview(r reply.Request, p Persona) string {
	m := mask.New()
	return p.Prompt + "\n\n" + personaContext(r, m) + "\n" + reply.ConversationBlock(r, m)
}
