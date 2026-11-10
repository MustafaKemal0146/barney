package reply

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/style"
)

func ownerCard() style.Card {
	return style.Card{
		Who: "Ali", Messages: 500, Confidence: style.ConfMid,
		Burst:   style.BurstStats{Bursts: 100, P2to3: 1, MaxSize: 3, InBurstGapSecs: 4},
		Length:  style.LengthStats{WordsMedian: 3, WordsP90: 4, SentencesMed: 1},
		Case:    style.CaseStats{AllLowerRatio: 1, LowerStartRatio: 1},
		Punct:   style.PunctStats{EndNoneRatio: 1},
		Latency: style.LatencyStats{Samples: 10, MedianSec: 60, P90Sec: 200},
		Samples: []string{"lan çoook yorgunum", "evdeyim", "gel bi çay iç"},
	}
}

func history() []model.Message {
	t := time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)
	return []model.Message{
		{Sender: "Ali", TS: t, Kind: model.KindText, Text: "napıyon"},
		{Sender: "Ayşe", TS: t.Add(time.Minute), Kind: model.KindText, Text: "numaram 0532 123 45 67 kaydet, akşam gelir misin?"},
	}
}

// fake: sıradaki yanıtları verir; hedef sayıyı sistem isteminden okuyup "good" yer tutucusunu doldurur.
type fake struct {
	replies []string
	prompts []string
	systems []string
	calls   int
}

var targetRx = regexp.MustCompile(`(?:tam olarak|Cevap) (\d+) (?:ayrı|ile)`)

func (f *fake) Complete(_ context.Context, system, user string, _ int) (string, llm.Usage, error) {
	f.systems = append(f.systems, system)
	f.prompts = append(f.prompts, user)
	r := f.replies[min(f.calls, len(f.replies)-1)]
	f.calls++
	if r == "GOOD" {
		n, _ := strconv.Atoi(targetRx.FindStringSubmatch(system)[1])
		var ms []string
		for i := 0; i < n; i++ {
			ms = append(ms, fmt.Sprintf("%q", "tamam gelirim"))
		}
		r = `{"mesajlar":[` + strings.Join(ms, ",") + `],"not":"kabul"}`
	}
	return r, llm.Usage{PromptTokens: 100, CompletionTokens: 10}, nil
}

func req() Request {
	return Request{Owner: ownerCard(), History: history(), OwnerName: "Ali", PartnerName: "Ayşe", Seed: 7}
}

func TestGoodFirstTry(t *testing.T) {
	f := &fake{replies: []string{"GOOD"}}
	res, err := Generate(context.Background(), f, req())
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 1 || !res.Confident || len(res.Violations) != 0 {
		t.Fatalf("%+v", res)
	}
	if len(res.Steps) < 1 || len(res.Steps) > res.Target || res.Target > 8 {
		t.Fatalf("adım sayısı %d hedef %d", len(res.Steps), res.Target)
	}
	if res.Steps[0].WaitSec < 4 || res.Usage.Total() != 110 {
		t.Fatalf("%+v", res)
	}
}

func TestRetryWithFeedback(t *testing.T) {
	bad := `{"mesajlar":["Tamam, Gelirim!"],"not":"x"}` // büyük harf, ünlem, 1 mesaj
	f := &fake{replies: []string{bad, "GOOD"}}
	res, err := Generate(context.Background(), f, req())
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || !res.Confident {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(f.prompts[1], "büyük harf") || !strings.Contains(f.prompts[1], "mesaj sayısı") {
		t.Fatalf("ikinci istek ihlalleri içermiyor:\n%s", f.prompts[1])
	}
}

func TestStillBadIsNotConfident(t *testing.T) {
	f := &fake{replies: []string{`{"mesajlar":["Tamam!"],"not":""}`}}
	res, err := Generate(context.Background(), f, req())
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != MaxRetries+1 || res.Confident || len(res.Violations) == 0 || len(res.Steps) != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestInvalidJSONErrors(t *testing.T) {
	f := &fake{replies: []string{"cevap şu: merhaba", `{"yanlis":1}`, `{"mesajlar":[]}`}}
	if _, err := Generate(context.Background(), f, req()); err == nil {
		t.Fatal("hata bekleniyordu")
	}
	if f.calls != MaxRetries+1 {
		t.Fatalf("çağrı sayısı %d", f.calls)
	}
}

func TestMaskingBothWays(t *testing.T) {
	f := &fake{replies: []string{`{"mesajlar":["kaydettim [TEL-1]"],"not":""}`, "GOOD"}}
	r := req()
	r.Owner.Burst = style.BurstStats{Bursts: 10, P1: 1, MaxSize: 1}
	res, err := Generate(context.Background(), f, r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.prompts[0], "532") {
		t.Fatalf("telefon modele gitti:\n%s", f.prompts[0])
	}
	if !strings.Contains(f.prompts[0], "[TEL-1]") {
		t.Fatalf("yer tutucu yok:\n%s", f.prompts[0])
	}
	_ = res
}

func TestUnmaskOutput(t *testing.T) {
	f := &fake{replies: []string{`{"mesajlar":["kaydettim [TEL-1]"],"not":""}`}}
	r := req()
	r.Owner.Burst = style.BurstStats{Bursts: 10, P1: 1, MaxSize: 1}
	res, err := Generate(context.Background(), f, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Steps[0].Text, "05321234567") {
		t.Fatalf("yer tutucu geri çevrilmedi: %q", res.Steps[0].Text)
	}
}

func TestPreconditions(t *testing.T) {
	r := req()
	r.Owner = style.Card{}
	if _, err := Generate(context.Background(), &fake{replies: []string{"GOOD"}}, r); err == nil {
		t.Error("boş kart reddedilmeli")
	}
	r = req()
	r.History = r.History[:1] // son mesaj kullanıcının
	if _, err := Generate(context.Background(), &fake{replies: []string{"GOOD"}}, r); err == nil {
		t.Error("cevaplanacak mesaj yok")
	}
}

func TestPlanDeterministicAndClamped(t *testing.T) {
	c := ownerCard()
	texts := []string{"a", "bir iki üç dört beş altı yedi sekiz dokuz on"}
	a := Plan(c, texts, rand.New(rand.NewSource(1)), DefaultTiming)
	b := Plan(c, texts, rand.New(rand.NewSource(1)), DefaultTiming)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatal("tekrarlanabilir değil")
	}
	if a[0].WaitSec < 60*0.6 || a[0].WaitSec > 60*1.6+1 {
		t.Errorf("ilk bekleme dağılım dışında: %v", a[0].WaitSec)
	}
	if a[1].WaitSec < float64(len([]rune(texts[1])))/5-0.1 {
		t.Errorf("yazma süresinden kısa: %v", a[1].WaitSec)
	}
	c.Latency.MedianSec = 100000 // gece boyu cevap vermeyen kişi
	if w := Plan(c, texts, rand.New(rand.NewSource(1)), DefaultTiming)[0].WaitSec; w > 610 {
		t.Errorf("tavan aşıldı: %v", w)
	}
}
