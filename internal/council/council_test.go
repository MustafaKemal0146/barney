package council

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

func ownerCard() style.Card {
	return style.Card{
		Who: "Ali", Messages: 500, Confidence: style.ConfMid,
		Burst:   style.BurstStats{Bursts: 100, P2to3: 1, MaxSize: 3, InBurstGapSecs: 4},
		Length:  style.LengthStats{WordsMedian: 3, WordsP90: 4, SentencesMed: 1},
		Case:    style.CaseStats{AllLowerRatio: 1, LowerStartRatio: 1},
		Punct:   style.PunctStats{EndNoneRatio: 1},
		Latency: style.LatencyStats{Samples: 10, MedianSec: 60},
		Samples: []string{"lan çoook yorgunum", "evdeyim"},
	}
}

func request() reply.Request {
	t := time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)
	return reply.Request{
		Owner: ownerCard(), OwnerName: "Ali", PartnerName: "Ayşe", Seed: 3,
		History: []model.Message{
			{Sender: "Ali", TS: t, Kind: model.KindText, Text: "napıyon"},
			{Sender: "Ayşe", TS: t.Add(time.Minute), Kind: model.KindText, Text: "akşam gelir misin? 0532 123 45 67"},
		},
	}
}

var tgtRx = regexp.MustCompile(`"(guvenli|komik|cesur)": .*?(\d+)-\d+ ayrı`)

// fake sistem isteminin türüne göre yanıt verir; eşzamanlı çağrılara dayanıklıdır.
type fake struct {
	mu         sync.Mutex
	calls      map[string]int
	failKind   map[string]bool // "okuyucu" | "synth" | "vicdan"
	synthFirst string          // ilk sentez yanıtı ("" = iyi)
	block      string          // Vicdan'ın reddedeceği tür
	prompts    []string
}

func kind(system string) string {
	switch {
	case strings.Contains(system, `Sen "Okuyucu"`):
		return "okuyucu"
	case strings.Contains(system, `Sen "Stratejist"`):
		return "stratejist"
	case strings.Contains(system, `Sen "Esprili"`):
		return "esprili"
	case strings.Contains(system, `Sen "Vicdan"`):
		return "vicdan"
	}
	return "synth"
}

func (f *fake) Complete(_ context.Context, system, user string, _ int) (string, llm.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := kind(system)
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[k]++
	f.prompts = append(f.prompts, k+"\n"+system+"\n"+user)
	u := llm.Usage{PromptTokens: 50, CompletionTokens: 10}
	if f.failKind[k] {
		return "", u, errors.New("sağlayıcı hatası")
	}
	switch k {
	case "vicdan":
		var ks []string
		for _, t := range Turler {
			ks = append(ks, fmt.Sprintf(`{"tur":%q,"uygun":%v,"sebep":"x"}`, t, t != f.block))
		}
		return `{"kararlar":[` + strings.Join(ks, ",") + `]}`, u, nil
	case "synth":
		if f.calls[k] == 1 && f.synthFirst != "" {
			return f.synthFirst, u, nil
		}
		var cs []string
		for _, m := range tgtRx.FindAllStringSubmatch(system, -1) {
			n, _ := strconv.Atoi(m[2])
			var ms []string
			for i := 0; i < n; i++ {
				ms = append(ms, fmt.Sprintf("%q", m[1]+" tamam"))
			}
			cs = append(cs, fmt.Sprintf(`{"tur":%q,"mesajlar":[%s]}`, m[1], strings.Join(ms, ",")))
		}
		return `{"adaylar":[` + strings.Join(cs, ",") + `],"not":"n"}`, u, nil
	}
	return `{"ozet":"ilgili","oneri":"soru sor","uyari":""}`, u, nil
}

func run(t *testing.T, f *fake) (Result, error) {
	t.Helper()
	return Run(context.Background(), Options{Fast: f}, request())
}

func TestHappyPath(t *testing.T) {
	f := &fake{}
	res, err := run(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if res.Calls != 5 || len(res.Opinions) != 3 || len(res.Candidates) != 3 {
		t.Fatalf("calls=%d op=%d cand=%d warn=%v", res.Calls, len(res.Opinions), len(res.Candidates), res.Warnings)
	}
	if res.Recommended != 0 || res.Candidates[0].Tur != TurGuvenli || !res.Candidates[0].Confident {
		t.Fatalf("%+v", res)
	}
	if res.Usage.Total() != 5*60 {
		t.Errorf("usage = %d", res.Usage.Total())
	}
	for _, c := range res.Candidates {
		if len(c.Steps) == 0 || c.Steps[0].WaitSec <= 0 {
			t.Errorf("bekleme planı yok: %+v", c)
		}
	}
	for _, p := range f.prompts {
		if strings.Contains(p, "532 123") {
			t.Fatalf("telefon modele gitti:\n%s", p)
		}
	}
}

func TestVicdanBlocks(t *testing.T) {
	res, err := run(t, &fake{block: TurGuvenli})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Candidates[0].Blocked || res.Candidates[0].Confident {
		t.Fatalf("engellenmeliydi: %+v", res.Candidates[0])
	}
	if res.Recommended < 0 || res.Candidates[res.Recommended].Tur != TurKomik {
		t.Fatalf("öneri komik olmalı: %d", res.Recommended)
	}
}

func TestOnePersonaFails(t *testing.T) {
	res, err := run(t, &fake{failKind: map[string]bool{"esprili": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Opinions) != 2 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "Esprili") {
		t.Fatalf("%+v", res)
	}
}

func TestAllPersonasFail(t *testing.T) {
	_, err := run(t, &fake{failKind: map[string]bool{"okuyucu": true, "stratejist": true, "esprili": true}})
	if err == nil {
		t.Fatal("hata bekleniyordu")
	}
}

func TestSynthRetryFixesViolations(t *testing.T) {
	bad := `{"adaylar":[{"tur":"guvenli","mesajlar":["Tamam!"]},{"tur":"komik","mesajlar":["Tamam!"]},{"tur":"cesur","mesajlar":["Tamam!"]}],"not":""}`
	f := &fake{synthFirst: bad}
	res, err := run(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls["synth"] != 2 || res.Calls != 6 {
		t.Fatalf("sentez=%d toplam=%d", f.calls["synth"], res.Calls)
	}
	if res.Recommended != 0 {
		t.Fatalf("%+v", res)
	}
	last := f.prompts[len(f.prompts)-2] // yeniden denenen sentez, Vicdan'dan önce
	if !strings.Contains(last, "guvenli: mesaj") {
		t.Fatalf("ihlal geri bildirimi yok:\n%s", last)
	}
}

func TestVicdanFailureLeavesNothingConfident(t *testing.T) {
	res, err := run(t, &fake{failKind: map[string]bool{"vicdan": true}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Recommended != -1 || len(res.Candidates) != 3 {
		t.Fatalf("%+v", res)
	}
	for _, c := range res.Candidates {
		if c.Confident {
			t.Fatal("Vicdan olmadan güvenilir aday olmamalı")
		}
	}
	if len(res.Warnings) == 0 {
		t.Fatal("uyarı bekleniyordu")
	}
}

func TestPreconditions(t *testing.T) {
	r := request()
	r.History = r.History[:1]
	if _, err := Run(context.Background(), Options{Fast: &fake{}}, r); err == nil {
		t.Error("son mesaj kullanıcınınsa hata")
	}
	if _, err := Run(context.Background(), Options{}, request()); err == nil {
		t.Error("model yoksa hata")
	}
}
