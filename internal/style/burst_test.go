package style

import (
	"math/rand"
	"testing"
)

func multiCard() Card {
	var c Card
	c.Messages = 500
	c.Burst.Bursts, c.Burst.P1, c.Burst.P2to3, c.Burst.P4plus, c.Burst.MaxSize = 100, 0.3, 0.5, 0.2, 6
	c.Length.WordsP90 = 6
	c.Punct.EndNoneRatio = 0.9
	return c
}

func TestPickRangeMultiAtLeastTwo(t *testing.T) {
	c := multiCard()
	for s := int64(0); s < 200; s++ {
		lo, hi := PickRange(c, rand.New(rand.NewSource(s)))
		if lo < 2 || hi < lo || hi > 8 {
			t.Fatalf("seed %d: %d-%d", s, lo, hi)
		}
	}
	var single Card
	single.Burst.Bursts, single.Burst.P1 = 100, 0.95
	if lo, _ := PickRange(single, rand.New(rand.NewSource(1))); lo != 1 {
		t.Fatalf("tek mesajcı kişide alt sınır 1 olmalı: %d", lo)
	}
}

func TestSplitLongIntoShortMessages(t *testing.T) {
	c := multiCard()
	got := Split(c, []string{"bugün çok yoruldum gerçekten ya. sınav vardı bi de çok zordu. akşam konuşuruz"}, 5)
	if len(got) != 3 || got[0] != "bugün çok yoruldum gerçekten ya" || got[2] != "akşam konuşuruz" {
		t.Fatalf("%q", got)
	}
	// hi sınırı: fazlası sona eklenir
	if got := Split(c, []string{"bir iki üç dört beş altı yedi. a. b. c"}, 2); len(got) != 2 {
		t.Fatalf("%q", got)
	}
	// kısa mesaj ve tek mesajcı kişi bölünmez
	if got := Split(c, []string{"tamam."}, 4); len(got) != 1 {
		t.Fatalf("%q", got)
	}
	var single Card
	single.Burst.Bursts, single.Burst.P1 = 100, 0.95
	long := []string{"bugün çok yoruldum gerçekten ya. sınav vardı bi de çok zordu"}
	if got := Split(single, long, 4); len(got) != 1 {
		t.Fatalf("%q", got)
	}
}
