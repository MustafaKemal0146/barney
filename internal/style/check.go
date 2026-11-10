package style

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PickBurst, kişinin patlama dağılımına göre bu sefer kaç mesaj atacağını seçer.
// Kart boşsa 1 döner. Aynı rng tohumu aynı sonucu verir.
func PickBurst(c Card, rng *rand.Rand) int {
	if c.Burst.Bursts == 0 {
		return 1
	}
	x := rng.Float64()
	switch {
	case x < c.Burst.P1:
		return 1
	case x < c.Burst.P1+c.Burst.P2to3:
		return 2 + rng.Intn(2) // 2 ya da 3
	default:
		hi := c.Burst.MaxSize
		if hi < 4 {
			hi = 4
		}
		if hi > 8 {
			hi = 8
		}
		return 4 + rng.Intn(hi-3)
	}
}

// Check, üretilen mesajların karta uyup uymadığını deterministik olarak denetler.
// Boş dönerse uygundur. target <= 0 ise mesaj sayısı denetlenmez.
// Kart güvenilir değilse (hiç mesaj yok) denetim yapılmaz.
func Check(c Card, texts []string, target int) []string {
	if c.Messages == 0 {
		return nil
	}
	var v []string
	if target > 0 && len(texts) != target {
		v = append(v, fmt.Sprintf("mesaj sayısı %d, hedef %d", len(texts), target))
	}
	maxWords := math.Max(c.Length.WordsP90*1.6, 4)
	for i, t := range texts {
		n := i + 1
		t = strings.TrimSpace(t)
		if t == "" {
			v = append(v, fmt.Sprintf("mesaj %d boş", n))
			continue
		}
		if w := float64(len(strings.Fields(t))); w > maxWords {
			v = append(v, fmt.Sprintf("mesaj %d çok uzun: %.0f sözcük, sınır %.0f", n, w, maxWords))
		}
		if c.Case.AllLowerRatio >= 0.9 && strings.IndexFunc(t, unicode.IsUpper) >= 0 {
			v = append(v, fmt.Sprintf("mesaj %d büyük harf içeriyor, kişi hep küçük harf yazar", n))
		} else if c.Case.LowerStartRatio >= 0.9 {
			if r, _ := utf8.DecodeRuneInString(t); unicode.IsUpper(r) {
				v = append(v, fmt.Sprintf("mesaj %d büyük harfle başlıyor, kişi küçük harfle başlar", n))
			}
		}
		ems := emojis(t)
		if c.Emoji.MessageRatio < 0.03 && len(ems) > 0 {
			v = append(v, fmt.Sprintf("mesaj %d emoji içeriyor, kişi neredeyse hiç emoji kullanmaz", n))
		} else if lim := int(math.Max(2, math.Ceil(c.Emoji.PerMessage*3))); len(ems) > lim {
			v = append(v, fmt.Sprintf("mesaj %d çok fazla emoji içeriyor (%d, sınır %d)", n, len(ems), lim))
		}
		body, _ := trimEmoji(t)
		switch {
		case strings.HasSuffix(body, "...") || strings.HasSuffix(body, "…"):
			if c.Punct.EllipsisRatio < 0.02 {
				v = append(v, fmt.Sprintf("mesaj %d üç noktayla bitiyor, kişi böyle yazmaz", n))
			}
		case strings.HasSuffix(body, "."):
			if c.Punct.EndPeriodRatio < 0.05 {
				v = append(v, fmt.Sprintf("mesaj %d nokta ile bitiyor, kişi mesajı noktayla bitirmez", n))
			}
		}
		if c.Punct.ExclaimRatio < 0.03 && strings.Contains(t, "!") {
			v = append(v, fmt.Sprintf("mesaj %d ünlem içeriyor, kişi ünlem kullanmaz", n))
		}
		if c.Length.MultiLineRatio < 0.05 && strings.Contains(t, "\n") {
			v = append(v, fmt.Sprintf("mesaj %d çok satırlı, kişi tek satır yazar", n))
		}
	}
	return v
}

// Multi kişinin art arda birden çok mesaj atma eğiliminde olup olmadığını söyler.
func (c Card) Multi() bool { return c.Burst.Bursts > 0 && c.Burst.P2to3+c.Burst.P4plus >= 0.3 }

// PickRange modelin kaç mesaj yazabileceği aralığı verir. Model içeriğe göre bu aralıkta seçer:
// anlatacak çok şey varsa çok, tek kelimelik cevapsa az. Art arda yazan kişide en az 2'dir.
func PickRange(c Card, rng *rand.Rand) (lo, hi int) {
	if c.Burst.Bursts == 0 {
		return 1, 3
	}
	t := PickBurst(c, rng)
	lo, hi = max(1, t-1), t+1
	if c.Multi() {
		lo, hi = max(2, lo), max(hi, 3)
	}
	hi = min(8, max(hi, lo))
	return lo, hi
}

// CheckRange Check gibidir ama mesaj sayısını [lo, hi] aralığına göre denetler.
func CheckRange(c Card, texts []string, lo, hi int) []string {
	if c.Messages == 0 {
		return nil
	}
	var v []string
	if len(texts) < lo || len(texts) > hi {
		v = append(v, fmt.Sprintf("mesaj sayısı %d, %d ile %d arasında olmalı (anlatacağını kısa, ayrı mesajlara böl)", len(texts), lo, hi))
	}
	return append(v, Check(c, texts, 0)...)
}

// Split art arda yazan kişide uzun mesajları cümle sınırlarından kısa mesajlara böler (yapay zekâsız).
// Toplam en çok hi mesaj olur; fazlası son parçaya eklenir. Kişi noktasız bitiriyorsa sondaki nokta atılır.
func Split(c Card, texts []string, hi int) []string {
	if !c.Multi() || hi <= 1 {
		return texts
	}
	limit := math.Max(c.Length.WordsP90, 6)
	var out []string
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if float64(len(strings.Fields(t))) <= limit {
			out = append(out, t)
			continue
		}
		out = append(out, splitSentences(t)...)
	}
	for len(out) > hi && len(out) > 1 { // fazlaları birleştir (sondan)
		n := len(out)
		out = append(out[:n-2], out[n-2]+" "+out[n-1])
	}
	if c.Punct.EndNoneRatio >= 0.7 {
		for i, t := range out {
			if strings.HasSuffix(t, ".") && !strings.HasSuffix(t, "..") {
				out[i] = strings.TrimSpace(strings.TrimSuffix(t, "."))
			}
		}
	}
	return out
}

// splitSentences metni . ! ? … ve satır sonlarından parçalar; işaretler parçada kalır.
func splitSentences(t string) []string {
	var out []string
	var cur strings.Builder
	rs := []rune(t)
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for i, r := range rs {
		if r == '\n' {
			flush()
			continue
		}
		cur.WriteRune(r)
		if strings.ContainsRune(".!?…", r) {
			next := rune(0)
			if i+1 < len(rs) {
				next = rs[i+1]
			}
			if next == ' ' || next == '\n' || next == 0 {
				flush()
			}
		}
	}
	flush()
	if len(out) == 0 {
		return []string{strings.TrimSpace(t)}
	}
	return out
}
