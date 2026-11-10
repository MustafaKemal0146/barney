package watch

import "testing"

func TestIsSensitive(t *testing.T) {
	for _, s := range []string{"bana iban atar mısın", "borç lazım", "hastaneye gidiyorum", "yarın buluşalım mı", "nerede buluşuyoruz", "adresini at", "ayrılalım", "şifre neydi", "kavga ettik", "TR12 3456 7890"} {
		if !IsSensitive(s) {
			t.Errorf("hassas sayılmalıydı: %q", s)
		}
	}
	for _, s := range []string{"naber", "bugün ders çok yoğundu", "bölüm toplantısı vardı", "ders bitti", "akşam film izleyelim mi", "😂😂", "", "kod yazıyorum", "sevgilim değil sadece arkadaş"} {
		if IsSensitive(s) {
			t.Errorf("hassas sayılmamalıydı: %q", s)
		}
	}
}
