package reply

import (
	"fmt"
	"strings"
)

// Tone cevabın içeriksel tonunu ayarlar. Yazma BİÇİMİ (uzunluk, imla, patlama) her zaman kullanıcının
// stil kartından gelir ve style.Check ile kodla denetlenir; ton yalnızca ne söylendiğini etkiler.
type Tone struct {
	Preset    string `json:"preset"` // benim | barney | sade | samimi | flort
	Flort     int    `json:"flort"`
	Cesaret   int    `json:"cesaret"`
	Mizah     int    `json:"mizah"`
	Samimiyet int    `json:"samimiyet"`
}

// DefaultTone yeni kişilerin başlangıç tonudur.
var DefaultTone = Tone{Preset: "benim", Flort: 3, Cesaret: 4, Mizah: 5, Samimiyet: 6}

// Clamp kaydırıcıları 0..10 aralığına çeker.
func (t Tone) Clamp() Tone {
	c := func(v int) int { return max(0, min(10, v)) }
	t.Flort, t.Cesaret, t.Mizah, t.Samimiyet = c(t.Flort), c(t.Cesaret), c(t.Mizah), c(t.Samimiyet)
	return t
}

func level(v int) string {
	switch {
	case v <= 1:
		return "hiç yok"
	case v <= 3:
		return "az"
	case v <= 5:
		return "orta"
	case v <= 7:
		return "yüksek"
	case v <= 9:
		return "çok yüksek"
	}
	return "en üst düzey"
}

var presetText = map[string]string{
	"benim":  "Tamamen kullanıcının kendi doğal tonu; abartma, kendi alışkanlığını koru.",
	"barney": "Kendinden emin, hazırcevap, hafif iddialı ve esprili ama saygılı; küçük hamleler yapar, kaba ya da aşağılayıcı olmaz.",
	"sade":   "Net, kısa, süssüz; doğrudan konuya girer.",
	"samimi": "Sıcak, içten, yakın bir arkadaş gibi; ilgi gösterir, soru sorar.",
	"flort":  "Oyunbaz, hafif iltifatlı, merak uyandıran; flört eder ama ısrarcı ve baskıcı olmaz.",
}

// ToneBlock tonu istem metnine çevirir; ayar yoksa boş döner.
func ToneBlock(t Tone) string {
	if t.Preset == "" {
		return ""
	}
	t = t.Clamp()
	var b strings.Builder
	b.WriteString("TON (içeriğin havası; yazma biçimini bozma)\n")
	if p, ok := presetText[t.Preset]; ok {
		fmt.Fprintf(&b, "- Mod: %s\n", p)
	}
	fmt.Fprintf(&b, "- Flört seviyesi: %s (%d/10)\n- Cesaret/iddia: %s (%d/10)\n- Mizah: %s (%d/10)\n- Samimiyet: %s (%d/10)\n",
		level(t.Flort), t.Flort, level(t.Cesaret), t.Cesaret, level(t.Mizah), t.Mizah, level(t.Samimiyet), t.Samimiyet)
	return b.String()
}

// Example kullanıcının düzelttiği bir öneridir (geri bildirimden öğrenme).
type Example struct {
	Suggested string `json:"suggested"`
	Final     string `json:"final"` // boşsa reddedildi
}

func contextBlock(r Request, mask func(string) string) string {
	var b strings.Builder
	if r.Stage != "" {
		fmt.Fprintf(&b, "\nİLİŞKİ AŞAMASI: %s\n", mask(r.Stage))
	}
	if len(r.Facts) > 0 {
		b.WriteString("\nKİŞİ HAKKINDA BİLİNENLER (yalnız bunlara dayan, gerisini uydurma)\n")
		for _, f := range r.Facts {
			fmt.Fprintf(&b, "- %s\n", oneLine(mask(f)))
		}
	}
	if len(r.Examples) > 0 {
		b.WriteString("\nKULLANICININ GEÇMİŞ DÜZELTMELERİ (neyi beğenmediğini ve nasıl yazdığını gösterir)\n")
		for _, e := range r.Examples {
			if e.Final == "" {
				fmt.Fprintf(&b, "- Reddetti: %s\n", oneLine(mask(e.Suggested)))
			} else {
				fmt.Fprintf(&b, "- Önerilen: %s → Gönderdiği: %s\n", oneLine(mask(e.Suggested)), oneLine(mask(e.Final)))
			}
		}
	}
	return b.String()
}
