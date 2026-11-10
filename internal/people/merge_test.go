package people

import (
	"fmt"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/model"
)

func mk(sender, text string, at time.Time) model.Message {
	return model.Message{Sender: sender, TS: at, Kind: model.KindText, Text: text}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateMergeDedupesAcrossSecondsAndMinutes(t *testing.T) {
	s := newStore(t)
	d := time.Date(2026, 11, 1, 10, 0, 30, 0, time.UTC)
	var first []model.Message
	for i := 0; i < 30; i++ {
		who := "Ali"
		if i%2 == 1 {
			who = "Ayşe"
		}
		first = append(first, mk(who, fmt.Sprintf("mesaj %d", i), d.Add(time.Duration(i)*5*time.Minute)))
	}
	v, err := s.CreateFromMessages("Ayşe", "Ali", "whatsapp", first)
	if err != nil || v.MessagesTotal != 30 {
		t.Fatalf("%+v %v", v, err)
	}
	// aynı mesajlar dakika duyarlı (saniyesiz) gelir + 5 yeni
	var again []model.Message
	for _, m := range first {
		m.TS = m.TS.Truncate(time.Minute)
		again = append(again, m)
	}
	for i := 30; i < 35; i++ {
		again = append(again, mk("Ali", fmt.Sprintf("yeni %d", i), d.Add(time.Duration(i)*5*time.Minute)))
	}
	n, err := s.Merge(v.ID, again)
	if err != nil || n != 5 {
		t.Fatalf("eklenen = %d (%v), beklenen 5", n, err)
	}
	if n, _ := s.Merge(v.ID, again); n != 0 {
		t.Fatalf("ikinci birleştirme %d ekledi", n)
	}
	got, _ := s.Get(v.ID)
	if got.MessagesTotal != 35 {
		t.Fatalf("toplam %d", got.MessagesTotal)
	}
	msgs, _ := s.Messages(v.ID)
	for i := 1; i < len(msgs); i++ {
		if msgs[i].TS.Before(msgs[i-1].TS) {
			t.Fatal("sıra bozuk")
		}
	}
}

func TestCardCacheInvalidatesOnMerge(t *testing.T) {
	s := newStore(t)
	d := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	var ms []model.Message
	for i := 0; i < 20; i++ {
		who := "Ali"
		if i%2 == 1 {
			who = "Ayşe"
		}
		ms = append(ms, mk(who, fmt.Sprintf("m %d", i), d.Add(time.Duration(i)*time.Hour)))
	}
	v, _ := s.CreateFromMessages("Ayşe", "Ali", "whatsapp", ms)
	if l := s.List(); l[0].MessagesTotal != 20 {
		t.Fatalf("%d", l[0].MessagesTotal)
	}
	_, _ = s.Merge(v.ID, []model.Message{mk("Ali", "bir daha", d.Add(100*time.Hour))})
	if l := s.List(); l[0].MessagesTotal != 21 || l[0].MessagesOwner != 11 {
		t.Fatalf("önbellek yenilenmedi: %+v", l[0].MessagesTotal)
	}
	if s.Count() != 1 || s.OwnerName() != "Ali" {
		t.Fatal("Count/OwnerName")
	}
}

func TestCreateRejectsEmpty(t *testing.T) {
	s := newStore(t)
	if _, err := s.CreateFromMessages("", "Ali", "", nil); err == nil {
		t.Error("boş ad kabul edildi")
	}
	if _, err := s.CreateFromMessages("Ayşe", "Ali", "", nil); err == nil {
		t.Error("mesajsız kişi kabul edildi")
	}
}

func TestFindIsConservative(t *testing.T) {
	s := newStore(t)
	d := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	add := func(name string) string {
		v, err := s.CreateFromMessages(name, "Ali", "whatsapp", []model.Message{mk("Ali", "x", d), mk(name, "y", d.Add(time.Minute))})
		if err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	ayse, aysegul := add("Ayşe"), add("Ayşegül")
	for title, want := range map[string]string{"Ayşe": ayse, "ayse": ayse, "Ayşegül": aysegul, "Ayşe Yılmaz": "", "Mehmet": ""} {
		v, ok := s.Find(title)
		switch {
		case want == "" && ok:
			// "Ayşe Yılmaz" iki kişiyle de örtüşmez: Ayşe'nin kelime kümesi altkümedir → tek aday olmalı
			if title != "Ayşe Yılmaz" || v.ID != ayse {
				t.Errorf("Find(%q) yanlış eşleşti: %s", title, v.Name)
			}
		case want != "" && (!ok || v.ID != want):
			t.Errorf("Find(%q) = %v %v, beklenen %s", title, v, ok, want)
		}
	}
	// "Ayşe" ile "Ayşegül" birbirine karışmamalı
	if v, _ := s.Find("Ayşegül"); v == nil || v.Name != "Ayşegül" {
		t.Error("Ayşegül yanlış kişiye gitti")
	}
}

func TestDeleteRemovesFilesAndCache(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	d := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	v, _ := s.CreateFromMessages("Ayşe", "Ali", "", []model.Message{mk("Ali", "a", d), mk("Ayşe", "b", d.Add(time.Minute))})
	if err := s.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if s2, _ := Open(dir); s2.Count() != 0 {
		t.Fatal("silinen kişi geri geldi")
	}
	if err := s.Delete(v.ID); err != ErrNotFound {
		t.Fatalf("%v", err)
	}
}
