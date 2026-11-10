// Package style, bir kişinin yazma biçimini (StyleCard) mesajlarından deterministik olarak çıkarır.
// LLM çağrısı yoktur: aynı veri her zaman aynı kartı üretir. Kart hem kullanıcı ("ses") hem karşı
// taraf için aynı yöntemle çıkarılır.
//
// Kartın Samples alanı ham mesaj metni içerir; LLM'e gitmeden önce mask paketinden geçirilmelidir.
package style

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MustafaKemal0146/barney/internal/model"
)

// Ayarlanabilir eşikler.
const (
	// BurstGap aynı kişinin art arda mesajlarının tek "patlama" sayılacağı en büyük boşluktur.
	BurstGap = 90 * time.Second
	// MaxLatency bundan uzun bir boşluk "cevap gecikmesi" sayılmaz (gece/ertesi gün).
	MaxLatency = 6 * time.Hour
	// SampleCount karta konan örnek mesaj sayısı (few-shot için).
	SampleCount = 40
	// recentWindow örnekler yalnızca son bu kadar metin mesajından seçilir (üslup zamanla değişir).
	recentWindow = 3000
)

// Güven düzeyleri.
const (
	ConfLow  = "düşük"
	ConfMid  = "orta"
	ConfHigh = "yüksek"
)

// WordCount bir sözcük ve sayısı.
type WordCount struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

// BurstStats art arda mesaj atma düzenini anlatır.
type BurstStats struct {
	Bursts         int     `json:"bursts"`
	P1             float64 `json:"p1"`      // tek mesajlık patlama oranı
	P2to3          float64 `json:"p2_3"`    // 2-3 mesajlık
	P4plus         float64 `json:"p4_plus"` // 4+ mesajlık
	MedianSize     float64 `json:"median_size"`
	MaxSize        int     `json:"max_size"`
	InBurstGapSecs float64 `json:"in_burst_gap_sec_median"` // patlama içi mesajlar arası ortanca saniye
}

// LengthStats mesaj uzunluğu.
type LengthStats struct {
	WordsMedian    float64 `json:"words_median"`
	WordsP90       float64 `json:"words_p90"`
	WordsMean      float64 `json:"words_mean"`
	CharsMedian    float64 `json:"chars_median"`
	SentencesMed   float64 `json:"sentences_median"`
	MultiLineRatio float64 `json:"multiline_ratio"` // birden çok satırlı mesaj oranı
}

// CaseStats büyük/küçük harf alışkanlığı.
type CaseStats struct {
	LowerStartRatio float64 `json:"lower_start_ratio"` // küçük harfle başlayan
	AllLowerRatio   float64 `json:"all_lower_ratio"`   // hiç büyük harf içermeyen
}

// PunctStats noktalama.
type PunctStats struct {
	EndPeriodRatio float64 `json:"end_period_ratio"` // tek nokta ile biten
	EndNoneRatio   float64 `json:"end_none_ratio"`   // noktalamasız biten
	EllipsisRatio  float64 `json:"ellipsis_ratio"`   // "..." ile biten
	QuestionRatio  float64 `json:"question_ratio"`   // soru işareti içeren
	ExclaimRatio   float64 `json:"exclaim_ratio"`    // ünlem içeren
	RepeatRatio    float64 `json:"repeat_ratio"`     // "??", "!!" gibi tekrar
}

// EmojiStats emoji kullanımı.
type EmojiStats struct {
	MessageRatio float64  `json:"message_ratio"` // en az bir emoji içeren mesaj oranı
	PerMessage   float64  `json:"per_message"`
	Top          []string `json:"top"`
}

// LatencyStats karşı tarafın son mesajından sonra ilk cevaba kadar geçen süre.
type LatencyStats struct {
	Samples   int     `json:"samples"`
	MedianSec float64 `json:"median_sec"`
	P90Sec    float64 `json:"p90_sec"`
}

// Card bir kişinin yazma biçimi.
type Card struct {
	Who        string       `json:"who"`
	Messages   int          `json:"messages"` // metin mesajı sayısı
	Confidence string       `json:"confidence"`
	Burst      BurstStats   `json:"burst"`
	Length     LengthStats  `json:"length"`
	Case       CaseStats    `json:"case"`
	Punct      PunctStats   `json:"punct"`
	Emoji      EmojiStats   `json:"emoji"`
	Elongation float64      `json:"elongation_ratio"` // "çooook" gibi uzatma içeren mesaj oranı
	TopWords   []WordCount  `json:"top_words"`
	Signature  []WordCount  `json:"signature_words"` // karşı tarafa göre belirgin sözcükler
	Latency    LatencyStats `json:"latency"`
	Hours      [24]float64  `json:"hours"` // saat dağılımı (toplamı 1)
	Samples    []string     `json:"samples"`
}

// IsAuthor bir mesajın incelenen kişiye ait olup olmadığını söyler.
type IsAuthor func(sender string) bool

// ByName gönderen adı Norm'a göre eşleşen kişiyi seçer.
func ByName(name string) IsAuthor {
	n := model.Norm(name)
	return func(s string) bool { return n != "" && model.Norm(s) == n }
}

// Extract verilen sohbetlerden author'a ait kartı çıkarır. Sohbetler zaman sırasında olmalıdır;
// sıralı değilse burada sıralanır. Grup sohbetlerini çağıran eler.
func Extract(who string, convs []model.Conversation, author IsAuthor) Card {
	a := newAcc()
	for _, c := range convs {
		a.addConversation(c, author)
	}
	return a.card(who)
}

// FromImport içe aktarmadan kullanıcının kartını ve her bire-bir sohbetteki karşı tarafın kartını çıkarır.
// Grup sohbetleri üslup farklı olduğu için dışarıda bırakılır.
func FromImport(imp *model.Import) (owner Card, partners []Card) {
	var direct []model.Conversation
	for _, c := range imp.Conversations {
		if !c.IsGroup {
			direct = append(direct, c)
		}
	}
	ownerIs := ByName(imp.OwnerName)
	owner = Extract(imp.OwnerName, direct, ownerIs)
	for _, c := range direct {
		other := partnerName(c, ownerIs)
		if other == "" {
			continue
		}
		partners = append(partners, Extract(other, []model.Conversation{c}, ByName(other)))
	}
	return owner, partners
}

func partnerName(c model.Conversation, ownerIs IsAuthor) string {
	for _, p := range c.Participants {
		if !ownerIs(p) {
			return p
		}
	}
	for _, m := range c.Messages {
		if m.Sender != "" && !ownerIs(m.Sender) {
			return m.Sender
		}
	}
	return ""
}

// acc ham örnekleri biriktirir.
type acc struct {
	texts                                  []string
	wordsPer, charsPer, sentPer            []float64
	multiLine, lowerStart, allLower        int
	endPeriod, endNone, ellipsis, quest    int
	exclaim, repeat, withEmoji, emojiTotal int
	elong                                  int
	emojiCount                             map[string]int
	ownTok, otherTok                       map[string]int
	ownTokTotal, otherTokTotal             int
	burstSizes                             []float64
	inBurstGaps, latencies                 []float64
	hours                                  [24]int
}

func newAcc() *acc {
	return &acc{emojiCount: map[string]int{}, ownTok: map[string]int{}, otherTok: map[string]int{}}
}

func (a *acc) addConversation(c model.Conversation, author IsAuthor) {
	msgs := append([]model.Message(nil), c.Messages...)
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].TS.Before(msgs[j].TS) })
	var prev *model.Message
	burst := 0
	for i := range msgs {
		m := &msgs[i]
		if m.Kind == model.KindSystem || m.Kind == model.KindDeleted || m.Kind == model.KindUnsent {
			continue
		}
		mine := author(m.Sender)
		if !mine {
			if m.Kind == model.KindText {
				for _, t := range tokens(m.Text) {
					a.otherTok[t]++
					a.otherTokTotal++
				}
			}
			a.closeBurst(&burst)
			prev = m
			continue
		}
		// Patlama ve gecikme: yalnızca kendi mesajlarımız için.
		sameRun := prev != nil && author(prev.Sender) && m.TS.Sub(prev.TS) <= BurstGap
		if sameRun {
			burst++
			a.inBurstGaps = append(a.inBurstGaps, m.TS.Sub(prev.TS).Seconds())
		} else {
			a.closeBurst(&burst)
			burst = 1
			if prev != nil && !author(prev.Sender) {
				if d := m.TS.Sub(prev.TS); d >= 0 && d <= MaxLatency {
					a.latencies = append(a.latencies, d.Seconds())
				}
			}
		}
		prev = m
		if m.Kind != model.KindText {
			continue
		}
		a.addText(m)
	}
	a.closeBurst(&burst)
}

func (a *acc) closeBurst(n *int) {
	if *n > 0 {
		a.burstSizes = append(a.burstSizes, float64(*n))
		*n = 0
	}
}

func (a *acc) addText(m *model.Message) {
	txt := strings.TrimSpace(m.Text)
	if txt == "" || isURLOnly(txt) {
		return
	}
	a.texts = append(a.texts, txt)
	a.hours[m.TS.Hour()]++
	toks := tokens(txt)
	a.wordsPer = append(a.wordsPer, float64(len(strings.Fields(txt))))
	a.charsPer = append(a.charsPer, float64(utf8.RuneCountInString(txt)))
	a.sentPer = append(a.sentPer, float64(sentences(txt)))
	for _, t := range toks {
		a.ownTok[t]++
		a.ownTokTotal++
	}
	if strings.Contains(txt, "\n") {
		a.multiLine++
	}
	first, _ := utf8.DecodeRuneInString(txt)
	if unicode.IsLower(first) {
		a.lowerStart++
	}
	hasUpper := false
	for _, r := range txt {
		if unicode.IsUpper(r) {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		a.allLower++
	}
	body, _ := trimEmoji(txt)
	switch {
	case strings.HasSuffix(body, "...") || strings.HasSuffix(body, "…"):
		a.ellipsis++
	case strings.HasSuffix(body, "."):
		a.endPeriod++
	case !endsWithPunct(body):
		a.endNone++
	}
	if strings.ContainsAny(txt, "?") {
		a.quest++
	}
	if strings.ContainsAny(txt, "!") {
		a.exclaim++
	}
	if strings.Contains(txt, "??") || strings.Contains(txt, "!!") || strings.Contains(txt, "?!") || strings.Contains(txt, "!?") {
		a.repeat++
	}
	ems := emojis(txt)
	if len(ems) > 0 {
		a.withEmoji++
		a.emojiTotal += len(ems)
		for _, e := range ems {
			a.emojiCount[e]++
		}
	}
	if hasElongation(txt) {
		a.elong++
	}
}

func (a *acc) card(who string) Card {
	n := len(a.texts)
	c := Card{Who: who, Messages: n}
	switch {
	case n >= 1000:
		c.Confidence = ConfHigh
	case n >= 300:
		c.Confidence = ConfMid
	default:
		c.Confidence = ConfLow
	}
	if n == 0 {
		return c
	}
	fn := float64(n)
	r := func(x int) float64 { return round3(float64(x) / fn) }

	// Patlama
	c.Burst.Bursts = len(a.burstSizes)
	if nb := float64(len(a.burstSizes)); nb > 0 {
		var p1, p23, p4 int
		for _, s := range a.burstSizes {
			switch {
			case s == 1:
				p1++
			case s <= 3:
				p23++
			default:
				p4++
			}
			if int(s) > c.Burst.MaxSize {
				c.Burst.MaxSize = int(s)
			}
		}
		c.Burst.P1, c.Burst.P2to3, c.Burst.P4plus = round3(float64(p1)/nb), round3(float64(p23)/nb), round3(float64(p4)/nb)
		c.Burst.MedianSize = pct(a.burstSizes, 50)
	}
	c.Burst.InBurstGapSecs = round3(pct(a.inBurstGaps, 50))

	c.Length = LengthStats{
		WordsMedian: pct(a.wordsPer, 50), WordsP90: pct(a.wordsPer, 90), WordsMean: round3(mean(a.wordsPer)),
		CharsMedian: pct(a.charsPer, 50), SentencesMed: pct(a.sentPer, 50), MultiLineRatio: r(a.multiLine),
	}
	c.Case = CaseStats{LowerStartRatio: r(a.lowerStart), AllLowerRatio: r(a.allLower)}
	c.Punct = PunctStats{
		EndPeriodRatio: r(a.endPeriod), EndNoneRatio: r(a.endNone), EllipsisRatio: r(a.ellipsis),
		QuestionRatio: r(a.quest), ExclaimRatio: r(a.exclaim), RepeatRatio: r(a.repeat),
	}
	c.Emoji = EmojiStats{MessageRatio: r(a.withEmoji), PerMessage: round3(float64(a.emojiTotal) / fn), Top: topKeys(a.emojiCount, 8)}
	c.Elongation = r(a.elong)
	c.TopWords = topWords(a.ownTok, 15, 2)
	c.Signature = a.signature(15)
	c.Latency = LatencyStats{Samples: len(a.latencies), MedianSec: pct(a.latencies, 50), P90Sec: pct(a.latencies, 90)}
	var hs int
	for _, h := range a.hours {
		hs += h
	}
	if hs > 0 {
		for i, h := range a.hours {
			c.Hours[i] = round3(float64(h) / float64(hs))
		}
	}
	c.Samples = pickSamples(a.texts, SampleCount)
	return c
}

// signature, kişinin karşı tarafa göre belirgin olarak daha çok kullandığı sözcükleri verir.
func (a *acc) signature(k int) []WordCount {
	if a.ownTokTotal == 0 {
		return nil
	}
	type sc struct {
		w     string
		c     int
		score float64
	}
	var all []sc
	for w, c := range a.ownTok {
		if c < 3 || utf8.RuneCountInString(w) < 2 {
			continue
		}
		own := float64(c) / float64(a.ownTokTotal)
		other := (float64(a.otherTok[w]) + 1) / (float64(a.otherTokTotal) + 1)
		if s := own / other; s >= 2 {
			all = append(all, sc{w, c, s})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].w < all[j].w
	})
	if len(all) > k {
		all = all[:k]
	}
	out := make([]WordCount, len(all))
	for i, s := range all {
		out[i] = WordCount{s.w, s.c}
	}
	return out
}

// pickSamples, metinler arasından eşit aralıklı n örnek seçer (son recentWindow mesaj içinden, tekrarsız).
func pickSamples(texts []string, n int) []string {
	if len(texts) > recentWindow {
		texts = texts[len(texts)-recentWindow:]
	}
	seen := map[string]bool{}
	var uniq []string
	for _, t := range texts {
		if !seen[t] {
			seen[t] = true
			uniq = append(uniq, t)
		}
	}
	if len(uniq) <= n {
		return uniq
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, uniq[i*len(uniq)/n])
	}
	return out
}

// ---- yardımcılar

var trLower = strings.NewReplacer("İ", "i", "I", "ı")

func tokens(s string) []string {
	s = strings.ToLower(trLower.Replace(s))
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, strings.Trim(b.String(), "'"))
			b.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	res := out[:0]
	for _, t := range out {
		if t != "" {
			res = append(res, t)
		}
	}
	return res
}

func topWords(m map[string]int, k, minLen int) []WordCount {
	var all []WordCount
	for w, c := range m {
		if utf8.RuneCountInString(w) >= minLen {
			all = append(all, WordCount{w, c})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Word < all[j].Word
	})
	if len(all) > k {
		all = all[:k]
	}
	return all
}

func topKeys(m map[string]int, k int) []string {
	var all []WordCount
	for w, c := range m {
		all = append(all, WordCount{w, c})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Word < all[j].Word
	})
	var out []string
	for i := 0; i < len(all) && i < k; i++ {
		out = append(out, all[i].Word)
	}
	return out
}

func isURLOnly(s string) bool {
	f := strings.Fields(s)
	return len(f) == 1 && (strings.HasPrefix(f[0], "http://") || strings.HasPrefix(f[0], "https://") || strings.HasPrefix(f[0], "www."))
}

func sentences(s string) int {
	n := 0
	inEnd := false
	for _, r := range s {
		if r == '.' || r == '!' || r == '?' || r == '…' || r == '\n' {
			if !inEnd {
				n++
				inEnd = true
			}
		} else if !unicode.IsSpace(r) {
			inEnd = false
		}
	}
	if !inEnd {
		n++
	}
	return n
}

func endsWithPunct(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r == '.' || r == '!' || r == '?' || r == '…' || r == ',' || r == ';' || r == ':' || r == ')'
}

// hasElongation aynı harfin üç ya da daha fazla kez art arda gelip gelmediğine bakar.
func hasElongation(s string) bool {
	var last rune
	run := 0
	for _, r := range strings.ToLower(s) {
		if r == last && unicode.IsLetter(r) {
			run++
			if run >= 3 {
				return true
			}
		} else {
			last, run = r, 1
		}
	}
	return false
}

func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F3FB && r <= 0x1F3FF: // ten tonu
		return false
	case r >= 0x1F300 && r <= 0x1FAFF, r >= 0x1F000 && r <= 0x1F2FF, r >= 0x2600 && r <= 0x27BF:
		return true
	case r == 0x2B50 || r == 0x2B55 || r == 0x2764 || r == 0x203C || r == 0x2049:
		return true
	}
	return false
}

// emojis metindeki emojileri sırayla verir (ZWJ / varyasyon seçicileri tek emojiye katılır).
func emojis(s string) []string {
	var out []string
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if !isEmojiRune(rs[i]) {
			continue
		}
		j := i + 1
		for j < len(rs) && (rs[j] == 0xFE0F || rs[j] == 0x200D || (rs[j] >= 0x1F3FB && rs[j] <= 0x1F3FF) || (rs[j-1] == 0x200D && isEmojiRune(rs[j]))) {
			j++
		}
		out = append(out, string(rs[i:j]))
		i = j - 1
	}
	return out
}

// trimEmoji sondaki emoji ve boşlukları atar; noktalama analizi emojiden etkilenmesin.
func trimEmoji(s string) (string, int) {
	rs := []rune(strings.TrimSpace(s))
	n := 0
	for len(rs) > 0 {
		r := rs[len(rs)-1]
		if isEmojiRune(r) || r == 0xFE0F || r == 0x200D || unicode.IsSpace(r) || (r >= 0x1F3FB && r <= 0x1F3FF) {
			rs = rs[:len(rs)-1]
			n++
			continue
		}
		break
	}
	return string(rs), n
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

// pct en yakın sıra yöntemiyle yüzdelik değeri verir.
func pct(x []float64, p float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	idx := int(math.Ceil(p/100*float64(len(s)))) - 1
	if idx < 0 {
		idx = 0
	}
	return round3(s[idx])
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
