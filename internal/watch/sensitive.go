package watch

import (
	"regexp"
	"strings"
)

// Hassas konular: insan onayı gerektirir; otomatik gönderim bu konularda asla yapılmaz.
// Go'nun \b'si yalnız ASCII'dir (ş, ö, ç ile başlayan sözcükleri kaçırır), bu yüzden sınırlar \pL ile kurulur.
var (
	// başı sözcük başı olan (ek alabilen) kökler
	sensitivePrefix = []string{"iban", "borç", "ödünç", "havale", "kredi", "hastane", "doktor", "ilaç", "ameliyat", "teşhis", "tedavi",
		"intihar", "buluşal", "buluşma", "görüşelim", "adres", "konum", "kavga", "ayrıl", "terk", "hamile", "gebe", "polis", "avukat", "şifre"}
	// tam sözcük olanlar
	sensitiveWhole = []string{"para", "eft", "hasta", "dava", "ölmek", "nerede buluşuyoruz", "saat kaçta"}
	sensitiveRe    = buildSensitive()
	ibanRe         = regexp.MustCompile(`(?i)\btr\d{2}\s?\d{4}`)
)

func buildSensitive() *regexp.Regexp {
	start := `(?:^|[^\pL\pN])`
	end := `(?:$|[^\pL\pN])`
	var parts []string
	for _, w := range sensitivePrefix {
		parts = append(parts, start+regexp.QuoteMeta(w))
	}
	for _, w := range sensitiveWhole {
		parts = append(parts, start+regexp.QuoteMeta(w)+end)
	}
	return regexp.MustCompile(`(?i)` + strings.Join(parts, "|"))
}

// IsSensitive para, sağlık, buluşma yeri/saati, ayrılık/kavga gibi insan onayı gerektiren konuları sezer.
func IsSensitive(text string) bool {
	t := strings.TrimSpace(text)
	return t != "" && (sensitiveRe.MatchString(t) || ibanRe.MatchString(t))
}
