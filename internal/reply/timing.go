package reply

import (
	"math"
	"math/rand"
	"unicode/utf8"

	"github.com/MustafaKemal0146/barney/internal/style"
)

// Step gönderilecek tek bir mesajdır; WaitSec, bir önceki adımdan (ilkinde: karşı tarafın
// mesajından) sonra yazmaya başlamadan önceki toplam bekleme süresidir (yazma süresi dahil).
type Step struct {
	Text    string  `json:"text"`
	WaitSec float64 `json:"wait_sec"`
}

// Timing bekleme planının sınırlarıdır.
type Timing struct {
	MinFirst, MaxFirst float64 // ilk cevap için en az / en çok bekleme (sn)
	CharsPerSec        float64 // yazma hızı
}

// DefaultTiming insan benzeri ama bezdirmeyen varsayılanlar.
var DefaultTiming = Timing{MinFirst: 4, MaxFirst: 600, CharsPerSec: 5}

// Plan, mesajlara kişinin kendi gecikme dağılımından türetilmiş bekleme süreleri atar.
// LLM'siz ve rng ile tekrarlanabilirdir. Gecikme örneği yoksa ilk bekleme ~20 sn kabul edilir.
func Plan(c style.Card, texts []string, rng *rand.Rand, t Timing) []Step {
	if t.CharsPerSec <= 0 {
		t.CharsPerSec = DefaultTiming.CharsPerSec
	}
	if t.MaxFirst <= 0 {
		t = Timing{MinFirst: DefaultTiming.MinFirst, MaxFirst: DefaultTiming.MaxFirst, CharsPerSec: t.CharsPerSec}
	}
	base := 20.0
	if c.Latency.Samples >= 5 && c.Latency.MedianSec > 0 {
		base = c.Latency.MedianSec
	}
	out := make([]Step, len(texts))
	for i, tx := range texts {
		typing := float64(utf8.RuneCountInString(tx)) / t.CharsPerSec
		var wait float64
		if i == 0 {
			wait = clamp(base*(0.6+rng.Float64()), t.MinFirst, t.MaxFirst) + typing
		} else {
			gap := c.Burst.InBurstGapSecs
			if gap <= 0 {
				gap = 3
			}
			wait = math.Max(gap*(0.7+0.6*rng.Float64()), typing)
		}
		out[i] = Step{Text: tx, WaitSec: math.Round(wait*10) / 10}
	}
	return out
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
