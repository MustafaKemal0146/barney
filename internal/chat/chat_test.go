package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

type rec struct {
	system, user string
	out          string
}

func (r *rec) Complete(_ context.Context, system, user string, _ int) (string, llm.Usage, error) {
	r.system, r.user = system, user
	return r.out, llm.Usage{}, nil
}

func card(who string, n int) style.Card {
	var ms []model.Message
	t := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		ms = append(ms, model.Message{Sender: who, TS: t.Add(time.Duration(i) * time.Minute), Kind: model.KindText, Text: fmt.Sprintf("selam napıyon %d", i)})
	}
	return style.Extract(who, []model.Conversation{{Messages: ms}}, style.ByName(who))
}

func ctxFor() Ctx {
	t := time.Now()
	return Ctx{OwnerName: "Ali", PartnerName: "Ayşe", Owner: card("Ali", 50), Partner: card("Ayşe", 50),
		Real: []model.Message{{Sender: "Ayşe", TS: t, Kind: model.KindText, Text: "tel 0532 123 45 67 gel"}},
		Tone: reply.Tone{Preset: "barney", Flort: 7, Cesaret: 8, Mizah: 6, Samimiyet: 5}, Notes: "yeni tanışıyoruz", Facts: []string{"kahve sever"}}
}

func TestAdvisorPromptHasToneStyleContextAndMasksPhone(t *testing.T) {
	r := &rec{out: "şunu yaz: \"tamam\""}
	out, err := Advisor(context.Background(), r, ctxFor(), []Msg{NewMsg("user", "önceki soru", nil)}, "ona ne yazayım?")
	if err != nil || !strings.Contains(out, "tamam") {
		t.Fatalf("%q %v", out, err)
	}
	for _, want := range []string{"danışmanısın", "hazırcevap", "Flört seviyesi", "kahve sever", "yeni tanışıyoruz", "YAZMA BİÇİMİ", "önceki soru", "ona ne yazayım?"} {
		if !strings.Contains(r.system+r.user, want) {
			t.Errorf("istemde yok: %q", want)
		}
	}
	if strings.Contains(r.system+r.user, "0532 123 45 67") || strings.Contains(r.system+r.user, "05321234567") {
		t.Error("telefon numarası maskelenmedi")
	}
	if _, err := Advisor(context.Background(), r, ctxFor(), nil, "  "); err == nil {
		t.Error("boş mesaj kabul edildi")
	}
}

func TestRoleplayNeedsEnoughPartnerMessagesAndSwapsRoles(t *testing.T) {
	cx := ctxFor()
	cx.Partner = card("Ayşe", 5)
	if _, err := Roleplay(context.Background(), &rec{out: "x"}, cx, nil, "selam", reply.DefaultTiming); err == nil {
		t.Fatal("az mesajla rol provası başladı")
	}
	cx = ctxFor()
	r := &rec{out: `{"mesajlar":["iyidir sen"],"not":"x"}`}
	steps, err := Roleplay(context.Background(), r, cx, []Msg{NewMsg("user", "naber", nil), NewMsg("assistant", "iyi", []reply.Step{{Text: "iyi", WaitSec: 2}})}, "bu akşam müsait misin", reply.DefaultTiming)
	if err != nil || len(steps) == 0 {
		t.Fatalf("%v %v", steps, err)
	}
	// model kişi (Ayşe) olarak yazar: "Sen Ayşe adlı kişinin yerine..."
	if !strings.Contains(r.system, "Sen Ayşe adlı kişinin") || !strings.Contains(r.user, "Şimdi Ayşe olarak") {
		t.Errorf("roller ters çevrilmedi:\n%s\n---\n%s", r.system, r.user)
	}
	if !strings.Contains(r.user, "Ali: bu akşam müsait misin") {
		t.Errorf("kullanıcı mesajı yok: %s", r.user)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	if l, err := Load(dir, ModeAdvisor); err != nil || len(l) != 0 {
		t.Fatal(l, err)
	}
	a, b := NewMsg("user", "a", nil), NewMsg("assistant", "b", []reply.Step{{Text: "b"}})
	if err := Append(dir, ModeAdvisor, a, b); err != nil {
		t.Fatal(err)
	}
	if l, _ := Load(dir, ModeAdvisor); len(l) != 2 || l[1].Steps[0].Text != "b" || a.ID == b.ID {
		t.Fatalf("%+v", l)
	}
	if l, _ := Load(dir, ModeRoleplay); len(l) != 0 {
		t.Fatal("modlar karıştı")
	}
	if err := Clear(dir, ModeAdvisor); err != nil || Clear(dir, ModeAdvisor) != nil {
		t.Fatal("temizleme")
	}
	if ValidMode("x") || !ValidMode(ModeRoleplay) {
		t.Fatal("ValidMode")
	}
}
