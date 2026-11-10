package style

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/model"
)

var t0 = time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)

func msg(sender string, sec int, text string) model.Message {
	return model.Message{Sender: sender, TS: t0.Add(time.Duration(sec) * time.Second), Kind: model.KindText, Text: text}
}

// kısa, küçük harfli, emojili, art arda mesaj atan "Ali" ile paragraf yazan "Ayşe".
func fixture() model.Conversation {
	var ms []model.Message
	sec := 0
	for i := 0; i < 20; i++ {
		ms = append(ms, msg("Ayşe", sec, "Bugün işten çıktıktan sonra eve geldim ve biraz dinlendim. Sen ne yapıyorsun, akşam için planın var mı?"))
		sec += 120 // Ali 2 dk sonra cevap verir
		ms = append(ms, msg("Ali", sec, "lan çoook yorgunum 😂"))
		ms = append(ms, msg("Ali", sec+5, "evdeyim"))
		ms = append(ms, msg("Ali", sec+9, "gel bi çay iç"))
		sec += 3600
	}
	ms = append(ms, model.Message{Sender: "Ali", TS: t0, Kind: model.KindSystem, Text: "sistem"})
	return model.Conversation{Title: "Ayşe", Participants: []string{"Ali", "Ayşe"}, Messages: ms}
}

func TestOwnerCard(t *testing.T) {
	imp := &model.Import{OwnerName: "Ali", Conversations: []model.Conversation{fixture()}}
	owner, partners := FromImport(imp)

	if owner.Messages != 60 {
		t.Fatalf("mesaj sayısı = %d, beklenen 60", owner.Messages)
	}
	if owner.Confidence != ConfLow {
		t.Errorf("güven = %s", owner.Confidence)
	}
	if owner.Burst.P2to3 != 1 || owner.Burst.P1 != 0 || owner.Burst.MaxSize != 3 {
		t.Errorf("patlama = %+v, hep 3'lü patlama bekleniyordu", owner.Burst)
	}
	if owner.Burst.InBurstGapSecs != 4 {
		t.Errorf("patlama içi aralık = %v", owner.Burst.InBurstGapSecs)
	}
	if owner.Case.AllLowerRatio != 1 || owner.Case.LowerStartRatio != 1 {
		t.Errorf("küçük harf = %+v", owner.Case)
	}
	if owner.Emoji.Top[0] != "😂" || owner.Emoji.MessageRatio < 0.33 || owner.Emoji.MessageRatio > 0.34 {
		t.Errorf("emoji = %+v", owner.Emoji)
	}
	if owner.Elongation < 0.33 || owner.Elongation > 0.34 {
		t.Errorf("uzatma = %v", owner.Elongation)
	}
	if owner.Latency.MedianSec != 120 || owner.Latency.Samples != 20 {
		t.Errorf("gecikme = %+v", owner.Latency)
	}
	if owner.Hours[20] == 0 {
		t.Errorf("saat dağılımı boş")
	}
	if owner.Length.WordsMedian > 4 {
		t.Errorf("ortanca sözcük = %v", owner.Length.WordsMedian)
	}
	hasLan := false
	for _, w := range owner.Signature {
		if w.Word == "lan" {
			hasLan = true
		}
	}
	if !hasLan {
		t.Errorf("imza sözcüklerinde 'lan' yok: %v", owner.Signature)
	}
	if len(owner.Samples) != 3 { // tekrarsız 3 farklı metin
		t.Errorf("örnek sayısı = %d", len(owner.Samples))
	}

	if len(partners) != 1 || partners[0].Who != "Ayşe" {
		t.Fatalf("karşı taraf kartları = %+v", partners)
	}
	p := partners[0]
	if p.Burst.P1 != 1 || p.Length.WordsMedian < 15 || p.Case.AllLowerRatio != 0 {
		t.Errorf("Ayşe kartı = %+v", p)
	}
	if p.Punct.QuestionRatio != 1 {
		t.Errorf("soru oranı = %v", p.Punct.QuestionRatio)
	}
}

func TestDeterministic(t *testing.T) {
	imp := &model.Import{OwnerName: "Ali", Conversations: []model.Conversation{fixture()}}
	a, _ := FromImport(imp)
	b, _ := FromImport(imp)
	if fmt.Sprintf("%+v", a) != fmt.Sprintf("%+v", b) {
		t.Fatal("aynı veriden farklı kart çıktı")
	}
}

func TestGroupsExcludedAndEmpty(t *testing.T) {
	g := fixture()
	g.IsGroup = true
	owner, partners := FromImport(&model.Import{OwnerName: "Ali", Conversations: []model.Conversation{g}})
	if owner.Messages != 0 || len(partners) != 0 || owner.Confidence != ConfLow {
		t.Fatalf("grup dışlanmalı: %+v", owner)
	}
}

func TestHelpers(t *testing.T) {
	if !hasElongation("çooook") || hasElongation("kolay") {
		t.Error("uzatma tespiti")
	}
	if got := emojis("a😂b❤️🔥"); len(got) != 3 {
		t.Errorf("emojis = %q", got)
	}
	if sentences("Selam. Nasılsın? iyi") != 3 {
		t.Error("cümle sayısı")
	}
	if !isURLOnly("https://x.com/a") || isURLOnly("bak https://x.com") {
		t.Error("url")
	}
}

func TestPickBurstFollowsDistribution(t *testing.T) {
	c := Card{Burst: BurstStats{Bursts: 100, P1: 0.5, P2to3: 0.5, MaxSize: 3}}
	rng := rand.New(rand.NewSource(3))
	counts := map[int]int{}
	for i := 0; i < 2000; i++ {
		counts[PickBurst(c, rng)]++
	}
	if counts[4] != 0 || counts[1] < 800 || counts[1] > 1200 || counts[2]+counts[3] < 800 {
		t.Fatalf("dağılım: %v", counts)
	}
	if PickBurst(Card{}, rng) != 1 {
		t.Error("boş kart 1 vermeli")
	}
}

func TestCheck(t *testing.T) {
	c := Card{Messages: 500, Length: LengthStats{WordsP90: 4}, Case: CaseStats{AllLowerRatio: 1, LowerStartRatio: 1}}
	if v := Check(c, []string{"tamam gelirim", "tamam"}, 2); len(v) != 0 {
		t.Fatalf("uygun mesaj reddedildi: %v", v)
	}
	v := Check(c, []string{"Tamam gelirim 😂 bu çok uzun bir mesaj oldu şimdi.", "ok!"}, 3)
	want := []string{"hedef 3", "büyük harf", "çok uzun", "emoji", "nokta", "ünlem"}
	all := strings.Join(v, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("%q ihlali yok:\n%s", w, all)
		}
	}
	if Check(Card{}, []string{"HER ŞEY SERBEST!!!"}, 5) != nil {
		t.Error("boş kartta denetim olmamalı")
	}
}
