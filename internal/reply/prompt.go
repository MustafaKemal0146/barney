package reply

import (
	"fmt"
	"strings"

	"github.com/MustafaKemal0146/barney/internal/mask"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/style"
)

func pc(x float64) string { return fmt.Sprintf("%%%.0f", x*100) }

// Describe bir kartı modele okunur Türkçe kurallara çevirir.
func Describe(c style.Card) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Mesaj uzunluğu: genelde %.0f sözcük (en uzun ~%.0f). %.0f cümle.\n", c.Length.WordsMedian, c.Length.WordsP90, c.Length.SentencesMed)
	switch {
	case c.Case.AllLowerRatio >= 0.9:
		b.WriteString("- Hep küçük harfle yazar, hiç büyük harf kullanmaz.\n")
	case c.Case.LowerStartRatio >= 0.9:
		b.WriteString("- Mesajlara küçük harfle başlar.\n")
	default:
		b.WriteString("- Normal büyük/küçük harf kullanır.\n")
	}
	switch {
	case c.Punct.EndNoneRatio >= 0.7:
		b.WriteString("- Mesajları noktalama koymadan bitirir.\n")
	case c.Punct.EndPeriodRatio >= 0.5:
		b.WriteString("- Mesajları noktayla bitirir.\n")
	}
	if c.Punct.ExclaimRatio >= 0.15 {
		fmt.Fprintf(&b, "- Mesajların %s'inde ünlem kullanır.\n", pc(c.Punct.ExclaimRatio))
	}
	if c.Emoji.MessageRatio < 0.03 {
		b.WriteString("- Emoji kullanmaz.\n")
	} else {
		fmt.Fprintf(&b, "- Mesajların %s'inde emoji vardır (sevdikleri: %s).\n", pc(c.Emoji.MessageRatio), strings.Join(c.Emoji.Top, " "))
	}
	if c.Elongation >= 0.05 {
		fmt.Fprintf(&b, "- Sözcükleri uzatır (\"çooook\" gibi), mesajların %s'inde.\n", pc(c.Elongation))
	}
	if len(c.Signature) > 0 {
		ws := make([]string, len(c.Signature))
		for i, w := range c.Signature {
			ws[i] = w.Word
		}
		fmt.Fprintf(&b, "- Kendine özgü sözcükleri: %s.\n", strings.Join(ws, ", "))
	}
	return b.String()
}

// SharedRules tüm istemlerde (tek çağrı ve konsey) geçen güvenlik ve dürüstlük kurallarıdır.
const SharedRules = `- Kişi hakkında bilmediğin olguyu (nerede olduğu, ne yaptığı, plan, yaş, iş) uydurma. Bilmiyorsan genel kal ya da soru sor.
- Yalan, baskı, kıskandırma, suçluluk yaratma ya da manipülasyon yok. Saygılı ve doğal ol; karşı taraf istemiyorsa üstelemezsin.
- Yapay zekâ olduğunu belli etme, açıklama ekleme. Sohbetin diliyle (Türkçe) yaz.
`

// StyleBlock kullanıcının yazma biçimini, örnek mesajlarını, karşı tarafın biçimini ve isteği anlatır.
// Maskelenecek her şey m'den geçer.
func StyleBlock(r Request, m *mask.Masker) string {
	var b strings.Builder
	fmt.Fprintf(&b, "YAZMA BİÇİMİ (%s)\n%s\n", r.OwnerName, m.Apply(Describe(r.Owner)))
	b.WriteString("ÖRNEK MESAJLAR (gerçek, birebir üslup)\n")
	for _, s := range r.Owner.Samples {
		fmt.Fprintf(&b, "- %s\n", oneLine(m.Apply(s)))
	}
	if r.Partner.Messages > 0 {
		fmt.Fprintf(&b, "\nKARŞI TARAFIN YAZMA BİÇİMİ (%s)\n%s", r.PartnerName, m.Apply(Describe(r.Partner)))
		b.WriteString("Cevabın onun tonuna ve enerjisine uyumlu olsun ama sen kendi sesinle yaz.\n")
	}
	if g := strings.TrimSpace(r.Goal); g != "" {
		fmt.Fprintf(&b, "\nKULLANICININ İSTEĞİ (bunu doğal biçimde gözet): %s\n", m.Apply(g))
	}
	if t := ToneBlock(r.Tone); t != "" {
		b.WriteString("\n" + t)
	}
	b.WriteString(contextBlock(r, m.Apply))
	return b.String()
}

// BurstRule kaç mesaj yazılacağını ve anlatımın kısa mesajlara nasıl bölüneceğini anlatır.
func BurstRule(c style.Card, lo, hi int) string {
	if lo == hi {
		return fmt.Sprintf("- Cevap tam olarak %d ayrı mesajdan oluşur.\n", lo)
	}
	s := fmt.Sprintf("- Cevap %d ile %d arası ayrı mesajdan oluşur; sayıyı söyleyeceğin şeye göre sen seç (kısa bir onay az, bir şey anlatıyorsan çok).\n", lo, hi)
	if c.Multi() {
		s += "- Bu kişi tek uzun mesaj atmaz, art arda kısa mesajlar atar. Anlatacağını parçalara böl: her mesaj tek bir düşünce ya da yarım cümle olsun (ör. \"ya\" / \"bugün çok yoruldum\" / \"sınav vardı bi de\"). Bir mesaja birden fazla cümle sığdırma.\n"
	}
	return s
}

// ConversationBlock maskelenmiş son sohbeti verir.
func ConversationBlock(r Request, m *mask.Masker) string { return conversationBlock(r, m) }

func systemPrompt(r Request, lo, hi int, m *mask.Masker) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sen %s adlı kişinin yerine mesaj yazan bir yazım yardımcısısın. Görevin: karşı tarafın son mesajına, bu kişinin kendi sesiyle, onun yazacağı gibi cevap yazmak.\n\n", r.OwnerName)
	b.WriteString("KURALLAR\n")
	b.WriteString(BurstRule(r.Owner, lo, hi))
	b.WriteString("- Aşağıdaki yazma biçimine ve örnek mesajlara sadık kal. Örneklerdeki cümleleri kopyalama, o üslupta yeni yaz.\n")
	b.WriteString(SharedRules)
	b.WriteString("- Çıktı YALNIZCA şu JSON: {\"mesajlar\":[\"...\"],\"not\":\"kısa gerekçe\",\"hassas\":false} (hassas: cevap para, sağlık, buluşma yeri/saati, kavga ya da ağır duygu gibi insan onayı gerektiren bir konuya girerse true)\n\n")
	b.WriteString(StyleBlock(r, m))
	return b.String()
}

func conversationBlock(r Request, m *mask.Masker) string {
	h := r.History
	if len(h) > ContextMax {
		h = h[len(h)-ContextMax:]
	}
	var b strings.Builder
	b.WriteString("SOHBET (eskiden yeniye; en alttaki cevaplanacak):\n")
	for _, msg := range h {
		if msg.Kind != model.KindText {
			fmt.Fprintf(&b, "%s: [%s]\n", msg.Sender, msg.Kind)
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", msg.Sender, oneLine(m.Apply(msg.Text)))
	}
	if r.Opener {
		fmt.Fprintf(&b, "\nSohbet bir süredir sessiz. Şimdi %s olarak sohbeti doğal biçimde yeniden başlatan bir mesaj yaz.", r.OwnerName)
		return b.String()
	}
	fmt.Fprintf(&b, "\nŞimdi %s olarak cevap yaz.", r.OwnerName)
	return b.String()
}

func oneLine(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ⏎ ") }
