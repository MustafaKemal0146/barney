package reply

import (
	"strings"
	"testing"
)

func TestToneBlockAndClamp(t *testing.T) {
	if ToneBlock(Tone{}) != "" {
		t.Error("ton yokken blok olmamalı")
	}
	b := ToneBlock(Tone{Preset: "flort", Flort: 99, Cesaret: -5, Mizah: 5, Samimiyet: 0})
	for _, w := range []string{"flört eder", "10/10", "0/10", "5/10"} {
		if !strings.Contains(b, w) {
			t.Errorf("%q yok: %s", w, b)
		}
	}
	c := Tone{Flort: 11, Cesaret: -1}.Clamp()
	if c.Flort != 10 || c.Cesaret != 0 {
		t.Errorf("%+v", c)
	}
}

func TestContextBlock(t *testing.T) {
	id := func(s string) string { return s }
	b := contextBlock(Request{Stage: "yakınlaşıyorlar", Facts: []string{"kahve sever"}, Examples: []Example{{Suggested: "a", Final: "b"}, {Suggested: "c"}}}, id)
	for _, w := range []string{"İLİŞKİ AŞAMASI: yakınlaşıyorlar", "kahve sever", "Önerilen: a → Gönderdiği: b", "Reddetti: c"} {
		if !strings.Contains(b, w) {
			t.Errorf("%q yok:\n%s", w, b)
		}
	}
	if contextBlock(Request{}, id) != "" {
		t.Error("boş bağlam blok üretmemeli")
	}
}
