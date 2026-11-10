package watch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
)

// fakeClock zamanlayıcıları elle ilerletir.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	ts  []*fakeTimer
}
type fakeTimer struct {
	at      time.Time
	f       func()
	stopped bool
	fired   bool
}

func (t *fakeTimer) Stop() bool { was := !t.stopped && !t.fired; t.stopped = true; return was }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) AfterFunc(d time.Duration, f func()) Stopper {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), f: f}
	c.ts = append(c.ts, t)
	return t
}

// advance süreyi ilerletir, vadesi gelenleri (eşzamanlı) çalıştırır.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	for _, t := range c.ts {
		if !t.stopped && !t.fired && !t.at.After(c.now) {
			t.fired = true
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

type rig struct {
	b     *Brain
	clk   *fakeClock
	mu    sync.Mutex
	acts  []Action
	calls int
	dec   func(Chat) (Decision, error)
}

func newRig(cfg Config) *rig {
	r := &rig{clk: &fakeClock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}}
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Calls: 5, Cands: []Cand{{Tur: "guvenli", Confident: true, Steps: []reply.Step{{Text: "tamam", WaitSec: 3}}}}}, nil
	}
	r.b = newBrain(cfg, func(_ context.Context, c Chat) (Decision, error) {
		r.mu.Lock()
		r.calls++
		r.mu.Unlock()
		return r.dec(c)
	}, func(a Action) { r.mu.Lock(); r.acts = append(r.acts, a); r.mu.Unlock() }, r.clk)
	return r
}

func (r *rig) actions() []Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Action(nil), r.acts...)
}
func (r *rig) llmCalls() int { r.mu.Lock(); defer r.mu.Unlock(); return r.calls }

func chat(key, text string) Chat {
	return Chat{ID: "Ayşe", Partner: "Ayşe", IncomingKey: key, History: []model.Message{
		{Sender: "Ali", Kind: model.KindText, Text: "napıyon"},
		{Sender: "Ayşe", Kind: model.KindText, Text: text},
	}}
}

func TestSleepsWithoutIncoming(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}})
	r.clk.advance(24 * time.Hour)
	if r.llmCalls() != 0 || len(r.actions()) != 0 {
		t.Fatal("olay yokken hiçbir şey olmamalı")
	}
}

func TestSettleBatchesBurst(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: 10 * time.Second})
	r.b.OnIncoming(chat("k1", "selam"))
	r.clk.advance(6 * time.Second)
	r.b.OnIncoming(chat("k2", "orada mısın"))
	r.clk.advance(6 * time.Second) // ilk zamanlayıcı iptal edilmiş olmalı
	if r.llmCalls() != 0 {
		t.Fatal("sakinleşme dolmadan çağrı yapıldı")
	}
	r.clk.advance(5 * time.Second)
	if r.llmCalls() != 1 {
		t.Fatalf("art arda mesajlar tek çağrı olmalı: %d", r.llmCalls())
	}
	a := r.actions()
	if len(a) != 1 || a[0].IncomingKey != "k2" || a[0].Kind != "suggest" {
		t.Fatalf("%+v", a)
	}
}

func TestDedupeAndAllowlist(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second})
	r.b.OnIncoming(chat("k1", "selam"))
	r.clk.advance(2 * time.Second)
	r.b.OnIncoming(chat("k1", "selam")) // aynı mesaj tekrar bildirildi
	r.clk.advance(2 * time.Second)
	other := chat("x", "selam")
	other.ID, other.Partner = "Mehmet", "Mehmet"
	r.b.OnIncoming(other)
	r.clk.advance(2 * time.Second)
	if r.llmCalls() != 1 || len(r.actions()) != 1 {
		t.Fatalf("çağrı=%d eylem=%d", r.llmCalls(), len(r.actions()))
	}
}

func TestEmptyAllowlistWatchesNothing(t *testing.T) {
	r := newRig(Config{})
	r.b.OnIncoming(chat("k1", "selam"))
	r.clk.advance(time.Minute)
	if r.llmCalls() != 0 {
		t.Fatal("izin listesi boşken izleme olmamalı")
	}
}

func TestTrivialSkipsWithoutLLM(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second})
	for i, txt := range []string{"ok", "👍", "Tamam!"} {
		r.b.OnIncoming(chat("k"+string(rune('a'+i)), txt))
		r.clk.advance(2 * time.Second)
	}
	if r.llmCalls() != 0 {
		t.Fatalf("kapanış mesajlarına LLM çağrılmamalı: %d", r.llmCalls())
	}
	for _, a := range r.actions() {
		if a.Kind != "skip" {
			t.Fatalf("%+v", a)
		}
	}
	if IsTrivial("akşam gelir misin") || IsTrivial("ok ama neden") {
		t.Fatal("gerçek mesajlar kapanış sayılmamalı")
	}
}

func TestAutoSendRequiresEverything(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second, AutoSend: true, MaxPerHour: 2})
	for i := 0; i < 3; i++ {
		r.b.OnIncoming(chat("k"+string(rune('a'+i)), "gelir misin"))
		r.clk.advance(2 * time.Second)
	}
	a := r.actions()
	if a[0].Kind != "send" || len(a[0].Steps) != 1 || a[1].Kind != "send" {
		t.Fatalf("%+v", a)
	}
	if a[2].Kind != "suggest" || a[2].Reason == "" {
		t.Fatalf("saatlik sınır aşılınca öneriye düşmeli: %+v", a[2])
	}
	r.clk.advance(61 * time.Minute)
	r.b.OnIncoming(chat("kz", "gelir misin"))
	r.clk.advance(2 * time.Second)
	if r.actions()[3].Kind != "send" {
		t.Fatal("bir saat sonra sınır sıfırlanmalı")
	}
}

func TestNotConfidentNeverSends(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second, AutoSend: true})
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Cands: []Cand{{Tur: "guvenli", Confident: false, Steps: []reply.Step{{Text: "x"}}}}}, nil
	}
	r.b.OnIncoming(chat("k1", "gelir misin"))
	r.clk.advance(2 * time.Second)
	if r.actions()[0].Kind != "suggest" {
		t.Fatalf("%+v", r.actions()[0])
	}
}

func TestPauseKillSwitch(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: 5 * time.Second, AutoSend: true})
	r.b.OnIncoming(chat("k1", "gelir misin"))
	r.b.Pause()
	r.clk.advance(time.Minute)
	r.b.OnIncoming(chat("k2", "orada mısın"))
	r.clk.advance(time.Minute)
	if r.llmCalls() != 0 || len(r.actions()) != 0 || !r.b.Paused() {
		t.Fatal("duraklatılmışken hiçbir şey çalışmamalı")
	}
	r.b.Resume()
	r.b.OnIncoming(chat("k3", "orada mısın"))
	r.clk.advance(6 * time.Second)
	if r.llmCalls() != 1 {
		t.Fatal("sürdürünce çalışmalı")
	}
}

func TestDailyCap(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second, MaxCallsPerDay: 5})
	r.b.OnIncoming(chat("k1", "gelir misin"))
	r.clk.advance(2 * time.Second) // 5 çağrı harcar
	r.b.OnIncoming(chat("k2", "orada mısın"))
	r.clk.advance(2 * time.Second)
	a := r.actions()
	if r.llmCalls() != 1 || a[1].Kind != "skip" {
		t.Fatalf("günlük sınır: çağrı=%d %+v", r.llmCalls(), a)
	}
	r.clk.advance(25 * time.Hour)
	r.b.OnIncoming(chat("k3", "orada mısın"))
	r.clk.advance(2 * time.Second)
	if r.llmCalls() != 2 {
		t.Fatal("ertesi gün sınır sıfırlanmalı")
	}
}

func TestErrorIsReportedNotSwallowed(t *testing.T) {
	r := newRig(Config{Allow: []string{"Ayşe"}, Settle: time.Second, AutoSend: true})
	r.dec = func(Chat) (Decision, error) { return Decision{}, errors.New("sağlayıcı hatası") }
	r.b.OnIncoming(chat("k1", "gelir misin"))
	r.clk.advance(2 * time.Second)
	if a := r.actions()[0]; a.Kind != "error" || a.Reason == "" {
		t.Fatalf("%+v", a)
	}
}

func TestManualBypassesAllowlistAndNeverSends(t *testing.T) {
	r := newRig(Config{AutoSend: true})
	r.b.Manual(chat("k1", "ok"))
	deadline := time.Now().Add(2 * time.Second)
	for len(r.actions()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	a := r.actions()
	if len(a) != 1 || a[0].Kind != "suggest" || r.llmCalls() != 1 {
		t.Fatalf("%+v", a)
	}
}
