// Package watch, Barney'nin "Gözcü" beynidir: tarayıcı uzantısından gelen "yeni mesaj" olaylarını
// ekonomik biçimde işler. İzleme LLM'siz ve olay tabanlıdır; yapay zekâ yalnızca şu koşullar
// birlikte sağlandığında çağrılır:
//   - sohbet izin listesindedir (açıkça verilmiş),
//   - gelen mesaj daha önce işlenmemiştir (içerik anahtarıyla tekrar engellenir),
//   - karşı taraf yazmayı bıraktıktan sonra "sakinleşme" süresi dolmuştur (art arda mesajlar tek çağrı),
//   - mesaj "ok/👍" gibi kapanış değildir,
//   - günlük çağrı sınırı aşılmamıştır.
//
// Otomatik gönderme ayrıca ister: AutoSend açık, aday "güvenilir" (stil+Vicdan), saatlik sınır dolmamış.
// Aksi halde yalnızca öneri üretilir ve gönderme kullanıcıya bırakılır.
package watch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
)

// Chat uzantının bildirdiği açık sohbettir. History eskiden yeniye sıralıdır.
type Chat struct {
	ID          string          `json:"id"`
	Partner     string          `json:"partner"`
	History     []model.Message `json:"history"`
	IncomingKey string          `json:"incoming_key"` // son gelen mesajın içerik anahtarı
}

// Cand bir aday cevaptır.
type Cand struct {
	Tur        string       `json:"tur"`
	Steps      []reply.Step `json:"steps"`
	Confident  bool         `json:"confident"`
	Blocked    bool         `json:"blocked,omitempty"`
	Sensitive  bool         `json:"sensitive,omitempty"` // hassas konu: otomatik gönderilmez
	Violations []string     `json:"violations,omitempty"`
}

// Decision Decider'ın sonucudur.
type Decision struct {
	Cands       []Cand
	Recommended int // -1: otomatik önerilecek aday yok
	Calls       int // yapılan LLM çağrısı sayısı
	Note        string
}

// Decider cevabı üretir (tek çağrı ya da konsey).
type Decider func(ctx context.Context, c Chat) (Decision, error)

// Action Gözcü'nün uzantıya söylediği şeydir.
type Action struct {
	Kind        string       `json:"kind"` // suggest | send | skip | error
	ChatID      string       `json:"chat_id"`
	IncomingKey string       `json:"incoming_key,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Cands       []Cand       `json:"cands,omitempty"`
	Recommended int          `json:"recommended"`
	Steps       []reply.Step `json:"steps,omitempty"`
	Calls       int          `json:"calls,omitempty"`
	// PendingApproval: cevap otomatik gönderilebilirdi ama bir koruma (hassas konu, sessiz saat,
	// ilk-N onay) yüzünden kullanıcı onayı bekliyor.
	PendingApproval bool `json:"pending_approval,omitempty"`
}

// FullSettle tam otonom modda son mesajdan sonra beklenen kısa süre (art arda mesajları toplamak için).
const FullSettle = 1500 * time.Millisecond

// Policy sohbet başına (kişi başına) çalışma anı kuralıdır. Brain'e SetPolicy ile verilirse Config'deki
// Allow/AutoSend/MaxPerHour yerine bu kullanılır.
type Policy struct {
	Watch         bool // izlenen sohbet mi
	Auto          bool // otomatik gönderim açık mı
	Quiet         bool // şu an sessiz saat
	ApprovalsLeft int  // >0: sıradaki cevaplar kullanıcı onayı ister
	MaxPerHour    int  // 0 = sınırsız
	MaxPerDay     int  // 0 = sınırsız
	StopSensitive bool
	Full          bool // tam otonom: kısa sakinleşme, "güvenilir" şartı aranmaz (engelli aday yine gönderilmez)
}

// Config Gözcü ayarları.
type Config struct {
	Allow          []string      // izinli sohbet/kişi adları; boşsa hiçbir sohbet izlenmez
	AutoSend       bool          // varsayılan KAPALI
	Settle         time.Duration // son gelen mesajdan sonra beklenecek süre (varsayılan 8 sn)
	MaxPerHour     int           // saatte en çok otomatik gönderim (varsayılan 6)
	MaxCallsPerDay int           // günde en çok LLM çağrısı (varsayılan 200)
	DecideTimeout  time.Duration // varsayılan 90 sn
}

func (c *Config) defaults() {
	if c.Settle <= 0 {
		c.Settle = 8 * time.Second
	}
	if c.MaxPerHour <= 0 {
		c.MaxPerHour = 6
	}
	if c.MaxCallsPerDay <= 0 {
		c.MaxCallsPerDay = 200
	}
	if c.DecideTimeout <= 0 {
		c.DecideTimeout = 90 * time.Second
	}
}

// Stopper bir zamanlayıcıyı durdurur.
type Stopper interface{ Stop() bool }

// Clock zamanı soyutlar (testlerde sahte).
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Stopper
}

type realClock struct{}

func (realClock) Now() time.Time                              { return time.Now() }
func (realClock) AfterFunc(d time.Duration, f func()) Stopper { return time.AfterFunc(d, f) }

// Brain Gözcü durum makinesidir.
type Brain struct {
	mu      sync.Mutex
	cfg     Config
	allow   map[string]bool
	decide  Decider
	clock   Clock
	emit    func(Action)
	paused  bool
	pending map[string]Chat        // sohbet → bekleyen (en yeni) durum
	timers  map[string]Stopper     // sohbet → sakinleşme zamanlayıcısı
	handled map[string]string      // sohbet → işlenmiş son IncomingKey
	sent    []time.Time            // otomatik gönderim zamanları
	sentBy  map[string][]time.Time // sohbet → otomatik gönderim zamanları (kişi bazlı sınır)
	policy  func(Chat) Policy
	day     string
	calls   int
}

// New bir Brain üretir. emit eşzamanlı çağrılabilir.
func New(cfg Config, d Decider, emit func(Action)) *Brain { return newBrain(cfg, d, emit, realClock{}) }

func newBrain(cfg Config, d Decider, emit func(Action), clk Clock) *Brain {
	cfg.defaults()
	b := &Brain{cfg: cfg, decide: d, clock: clk, emit: emit, allow: map[string]bool{},
		pending: map[string]Chat{}, timers: map[string]Stopper{}, handled: map[string]string{}, sentBy: map[string][]time.Time{}}
	for _, n := range cfg.Allow {
		if k := model.Norm(n); k != "" {
			b.allow[k] = true
		}
	}
	return b
}

// Allowed sohbetin izinli olup olmadığını söyler.
func (b *Brain) Allowed(c Chat) bool {
	if b.policy != nil {
		return b.policy(c).Watch
	}
	return b.allow[model.Norm(c.Partner)] || b.allow[model.Norm(c.ID)]
}

// Pause tüm izlemeyi durdurur (kill switch) ve bekleyen zamanlayıcıları iptal eder.
func (b *Brain) Pause() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paused = true
	for id, t := range b.timers {
		t.Stop()
		delete(b.timers, id)
	}
	b.pending = map[string]Chat{}
}

// Resume izlemeyi sürdürür.
func (b *Brain) Resume() { b.mu.Lock(); b.paused = false; b.mu.Unlock() }

// Paused durdurulmuş mu.
func (b *Brain) Paused() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.paused }

// SetPolicy sohbet başına kuralı belirleyen işlevi verir (nil = Config'deki sabit kurallar).
func (b *Brain) SetPolicy(f func(Chat) Policy) { b.mu.Lock(); b.policy = f; b.mu.Unlock() }

// SetAutoSend otomatik göndermeyi açar/kapar.
func (b *Brain) SetAutoSend(on bool) { b.mu.Lock(); b.cfg.AutoSend = on; b.mu.Unlock() }

// OnIncoming açık sohbette yeni bir karşı taraf mesajı bildirir. Sıfır maliyetlidir: yalnızca
// zamanlayıcıyı (yeniden) kurar.
func (b *Brain) OnIncoming(c Chat) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.paused || !b.Allowed(c) || c.IncomingKey == "" || b.handled[c.ID] == c.IncomingKey {
		return
	}
	b.pending[c.ID] = c
	if t := b.timers[c.ID]; t != nil {
		t.Stop()
	}
	id := c.ID
	settle := b.cfg.Settle
	if b.policy != nil {
		if pol := b.policy(c); pol.Full && pol.Auto {
			settle = FullSettle
		}
	}
	b.timers[id] = b.clock.AfterFunc(settle, func() { b.fire(id) })
}

// Manual kullanıcının "şimdi öner" isteğidir: izin listesini ve sakinleşmeyi atlar, hiçbir zaman
// otomatik göndermez.
func (b *Brain) Manual(c Chat) {
	go func() {
		a := b.run(c, false)
		if a.Kind == "send" { // asla
			a.Kind = "suggest"
		}
		b.emit(a)
	}()
}

func (b *Brain) fire(id string) {
	b.mu.Lock()
	c, ok := b.pending[id]
	delete(b.timers, id)
	if !ok || b.paused || b.handled[id] == c.IncomingKey {
		b.mu.Unlock()
		return
	}
	b.handled[id] = c.IncomingKey
	b.mu.Unlock()
	b.emit(b.run(c, true))
}

func (b *Brain) run(c Chat, allowAuto bool) Action {
	act := Action{ChatID: c.ID, IncomingKey: c.IncomingKey, Recommended: -1}
	last := lastText(c)
	if allowAuto && IsTrivial(last) {
		act.Kind, act.Reason = "skip", "kapanış mesajı, yapay zekâ çağrılmadı"
		return act
	}
	b.mu.Lock()
	if today := b.clock.Now().Format("2006-01-02"); today != b.day {
		b.day, b.calls = today, 0
	}
	if b.calls >= b.cfg.MaxCallsPerDay {
		b.mu.Unlock()
		act.Kind, act.Reason = "skip", "günlük yapay zekâ çağrı sınırı doldu"
		return act
	}
	timeout := b.cfg.DecideTimeout
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	d, err := b.decide(ctx, c)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls += max(d.Calls, 1)
	act.Calls = d.Calls
	if err != nil {
		act.Kind, act.Reason = "error", err.Error()
		return act
	}
	// Düşünürken daha yeni bir mesaj geldiyse bu cevap bayat; yenisi kendi turunu başlatır.
	if allowAuto {
		if p, ok := b.pending[c.ID]; ok && p.IncomingKey != c.IncomingKey {
			act.Kind, act.Reason = "skip", "düşünürken daha yeni mesaj geldi"
			return act
		}
	}
	act.Cands, act.Recommended, act.Reason = d.Cands, d.Recommended, d.Note
	act.Kind = "suggest"
	pol := Policy{Watch: true, Auto: b.cfg.AutoSend, MaxPerHour: b.cfg.MaxPerHour, MaxPerDay: 1 << 30, StopSensitive: true}
	if b.policy != nil {
		pol = b.policy(c)
	}
	if allowAuto && pol.Auto && !b.paused && d.Recommended >= 0 && d.Recommended < len(d.Cands) && (d.Cands[d.Recommended].Confident || (pol.Full && !d.Cands[d.Recommended].Blocked)) {
		cand := d.Cands[d.Recommended]
		now := b.clock.Now()
		keep := b.sentBy[c.ID][:0]
		today := 0
		for _, t := range b.sentBy[c.ID] {
			if now.Sub(t) < 24*time.Hour {
				keep = append(keep, t)
				today++
			}
		}
		b.sentBy[c.ID] = keep
		hour := 0
		for _, t := range keep {
			if now.Sub(t) < time.Hour {
				hour++
			}
		}
		switch {
		case cand.Sensitive && pol.StopSensitive:
			act.PendingApproval, act.Reason = true, "hassas konu: göndermeden önce onayın gerekiyor"
		case pol.Quiet:
			act.PendingApproval, act.Reason = true, "sessiz saat: otomatik gönderilmedi, onayını bekliyor"
		case pol.ApprovalsLeft > 0:
			act.PendingApproval, act.Reason = true, fmt.Sprintf("ilk cevaplar onaylı (%d kaldı): onayını bekliyor", pol.ApprovalsLeft)
		case pol.MaxPerHour > 0 && hour >= pol.MaxPerHour:
			act.Reason = fmt.Sprintf("saatlik otomatik gönderim sınırı (%d) doldu; öneri bırakıldı", pol.MaxPerHour)
		case pol.MaxPerDay > 0 && today >= pol.MaxPerDay:
			act.Reason = fmt.Sprintf("günlük otomatik gönderim sınırı (%d) doldu; öneri bırakıldı", pol.MaxPerDay)
		default:
			b.sentBy[c.ID] = append(b.sentBy[c.ID], now)
			b.sent = append(b.sent, now)
			act.Kind, act.Steps = "send", cand.Steps
			if pol.Full { // tam otonom: bekleme en az (yine de insan gibi küçük aralık)
				st := make([]reply.Step, len(cand.Steps))
				for i, s := range cand.Steps {
					st[i] = reply.Step{Text: s.Text, WaitSec: min(s.WaitSec, 1.5)}
				}
				act.Steps = st
			}
		}
	}
	return act
}

// AddCalls dışarıda (arayüzden) yapılan yapay zekâ çağrılarını günlük sayaca ekler.
func (b *Brain) AddCalls(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if today := b.clock.Now().Format("2006-01-02"); today != b.day {
		b.day, b.calls = today, 0
	}
	b.calls += n
}

// Stats anlık durumdur.
type Stats struct {
	Paused   bool `json:"paused"`
	AutoSend bool `json:"auto_send"`
	CallsDay int  `json:"calls_today"`
	SentHour int  `json:"sent_last_hour"`
}

// Stats durumu verir.
func (b *Brain) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, t := range b.sent {
		if b.clock.Now().Sub(t) < time.Hour {
			n++
		}
	}
	return Stats{Paused: b.paused, AutoSend: b.cfg.AutoSend, CallsDay: b.calls, SentHour: n}
}

func lastText(c Chat) string {
	for i := len(c.History) - 1; i >= 0; i-- {
		if c.History[i].Sender != "" && model.Norm(c.History[i].Sender) == model.Norm(c.Partner) {
			return c.History[i].Text
		}
	}
	return ""
}

var trivial = map[string]bool{
	"ok": true, "okey": true, "tamam": true, "tmm": true, "hmm": true, "hı hı": true, "aynen": true,
	"tşk": true, "sağol": true, "sagol": true, "eyvallah": true, "süper": true,
}

// IsTrivial cevap gerektirmeyen kapanış/onay mesajlarını tanır: "ok", "tamam", yalnız emoji vb.
func IsTrivial(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return true
	}
	if trivial[strings.Trim(t, ".!")] {
		return true
	}
	for _, r := range t {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true // yalnız emoji/noktalama
}
