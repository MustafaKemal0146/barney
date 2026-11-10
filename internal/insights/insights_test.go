package insights

import (
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/model"
)

func msg(who string, at time.Time) model.Message {
	return model.Message{Sender: who, TS: at, Kind: model.KindText, Text: "x"}
}

func TestAnalyze(t *testing.T) {
	d := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC) // pazartesi
	ms := []model.Message{
		msg("Ali", d),
		msg("Ayşe", d.Add(2*time.Minute)),              // Ayşe 120 sn'de cevapladı
		msg("Ali", d.Add(3*time.Minute)),               // Ali 60 sn
		msg("Ayşe", d.Add(24*time.Hour)),               // yeni konuşmayı Ayşe başlattı
		msg("Ali", d.Add(24*time.Hour+10*time.Minute)), // Ali 600 sn
	}
	r := Analyze(ms, "Ali", d.Add(72*time.Hour))
	if r.Total != 5 || r.OwnerSharePct != 60 || r.PartnerSharePct != 40 {
		t.Fatalf("%+v", r)
	}
	if r.InitiatorOwnerPct != 50 {
		t.Errorf("başlatan: %v", r.InitiatorOwnerPct)
	}
	if r.LatencyPartnerMedSec != 120 || r.LatencyOwnerMedianSec != 600 {
		t.Errorf("gecikme: %v %v", r.LatencyPartnerMedSec, r.LatencyOwnerMedianSec)
	}
	if r.ByHour[20] != 5 || r.ByWeekday[0] != 3 || r.ByWeekday[1] != 2 {
		t.Errorf("dağılım: %v %v", r.ByHour, r.ByWeekday)
	}
	if r.SilenceDays != 1 || len(r.Weekly) != 1 {
		t.Errorf("sessizlik/haftalık: %d %v", r.SilenceDays, r.Weekly)
	}
}
