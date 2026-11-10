package watch

import (
	"fmt"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/reply"
)

func policyRig(p Policy) *rig {
	r := newRig(Config{Settle: time.Second})
	r.b.SetPolicy(func(Chat) Policy { return p })
	return r
}

func fire(r *rig, key, text string) Action {
	n := len(r.actions())
	r.b.OnIncoming(chat(key, text))
	r.clk.advance(2 * time.Second)
	a := r.actions()
	if len(a) != n+1 {
		return Action{Kind: "<yok>"}
	}
	return a[n]
}

func TestPolicyWatchOffIgnores(t *testing.T) {
	r := policyRig(Policy{Watch: false, Auto: true})
	if a := fire(r, "k1", "gelir misin"); a.Kind != "<yok>" || r.llmCalls() != 0 {
		t.Fatalf("izlenmeyen kişi işlendi: %+v", a)
	}
}

func TestPolicyAutoOffOnlySuggests(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: false})
	if a := fire(r, "k1", "gelir misin"); a.Kind != "suggest" || a.PendingApproval {
		t.Fatalf("%+v", a)
	}
}

func TestPolicyGuardsTurnAutoSendIntoPendingApproval(t *testing.T) {
	cases := map[string]Policy{
		"sessiz saat": {Watch: true, Auto: true, Quiet: true, MaxPerHour: 6, MaxPerDay: 30},
		"ilk-N onay":  {Watch: true, Auto: true, ApprovalsLeft: 2, MaxPerHour: 6, MaxPerDay: 30},
		"hassas konu": {Watch: true, Auto: true, StopSensitive: true, MaxPerHour: 6, MaxPerDay: 30},
	}
	for name, p := range cases {
		r := policyRig(p)
		r.dec = func(Chat) (Decision, error) {
			return Decision{Recommended: 0, Calls: 1, Cands: []Cand{{Tur: "tek", Confident: true, Sensitive: name == "hassas konu", Steps: []reply.Step{{Text: "x"}}}}}, nil
		}
		if a := fire(r, "k1", "gelir misin"); a.Kind != "suggest" || !a.PendingApproval || a.Reason == "" {
			t.Errorf("%s: %+v", name, a)
		}
	}
}

func TestPolicySensitiveAllowedWhenGuardOff(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: true, StopSensitive: false, MaxPerHour: 6, MaxPerDay: 30})
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Cands: []Cand{{Tur: "tek", Confident: true, Sensitive: true, Steps: []reply.Step{{Text: "x"}}}}}, nil
	}
	if a := fire(r, "k1", "gelir misin"); a.Kind != "send" {
		t.Fatalf("koruma kapalıyken gönderilmeli: %+v", a)
	}
}

func TestPolicyPerPersonHourlyAndDailyLimits(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: true, MaxPerHour: 1, MaxPerDay: 2, StopSensitive: true})
	if a := fire(r, "k1", "a b"); a.Kind != "send" {
		t.Fatalf("%+v", a)
	}
	if a := fire(r, "k2", "a b"); a.Kind != "suggest" {
		t.Fatalf("saatlik sınır: %+v", a)
	}
	r.clk.advance(61 * time.Minute)
	if a := fire(r, "k3", "a b"); a.Kind != "send" {
		t.Fatalf("saat sonra gitmeli: %+v", a)
	}
	r.clk.advance(61 * time.Minute)
	if a := fire(r, "k4", "a b"); a.Kind != "suggest" || a.Reason == "" {
		t.Fatalf("günlük sınır: %+v", a)
	}
}

func TestManualNeverAutoSendsAndCountsCalls(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: true, MaxPerHour: 6, MaxPerDay: 30})
	r.b.AddCalls(7)
	if s := r.b.Stats(); s.CallsDay != 7 {
		t.Fatalf("%+v", s)
	}
}

func TestPolicyFullAutoSendsFastAndCapsWait(t *testing.T) {
	// Settle 1s olan rig'de Full 1.5s bekler; fire() 2s ilerletir.
	r := policyRig(Policy{Watch: true, Auto: true, Full: true, StopSensitive: true, MaxPerHour: 6, MaxPerDay: 30})
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Calls: 1, Cands: []Cand{{Tur: "tek", Confident: false, Steps: []reply.Step{{Text: "x", WaitSec: 40}}}}}, nil
	}
	a := fire(r, "k1", "gelir misin")
	if a.Kind != "send" || len(a.Steps) != 1 || a.Steps[0].WaitSec > 1.5 {
		t.Fatalf("tam otonom hemen göndermeli: %+v", a)
	}
}

func TestPolicyFullAutoStillStopsOnSensitiveAndBlocked(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: true, Full: true, StopSensitive: true, MaxPerHour: 6, MaxPerDay: 30})
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Calls: 1, Cands: []Cand{{Tur: "tek", Sensitive: true, Steps: []reply.Step{{Text: "x"}}}}}, nil
	}
	if a := fire(r, "k1", "x"); a.Kind == "send" || !a.PendingApproval {
		t.Fatalf("hassas konuda durmalı: %+v", a)
	}
	r2 := policyRig(Policy{Watch: true, Auto: true, Full: true, StopSensitive: true, MaxPerHour: 6, MaxPerDay: 30})
	r2.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Calls: 1, Cands: []Cand{{Tur: "tek", Blocked: true, Steps: []reply.Step{{Text: "x"}}}}}, nil
	}
	if a := fire(r2, "k1", "gelir misin"); a.Kind == "send" {
		t.Fatalf("engelli aday gönderildi: %+v", a)
	}
}

func TestPolicyZeroLimitMeansUnlimited(t *testing.T) {
	r := policyRig(Policy{Watch: true, Auto: true, MaxPerHour: 0, MaxPerDay: 0})
	r.dec = func(Chat) (Decision, error) {
		return Decision{Recommended: 0, Calls: 1, Cands: []Cand{{Tur: "tek", Confident: true, Steps: []reply.Step{{Text: "x"}}}}}, nil
	}
	for i := 0; i < 20; i++ {
		if a := fire(r, fmt.Sprintf("k%d", i), "gelir misin"); a.Kind != "send" {
			t.Fatalf("%d. cevap gönderilmedi (sınırsız olmalı): %+v", i, a)
		}
	}
	r2 := policyRig(Policy{Watch: true, Auto: true, MaxPerHour: 2, MaxPerDay: 0})
	r2.dec = r.dec
	fire(r2, "a", "gelir misin")
	fire(r2, "b", "gelir misin")
	if a := fire(r2, "c", "gelir misin"); a.Kind == "send" {
		t.Fatalf("saatlik sınır 2 aşıldı: %+v", a)
	}
}
