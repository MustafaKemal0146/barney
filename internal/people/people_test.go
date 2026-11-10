package people

import (
	"testing"
	"time"
)

func TestQuietHoursWrapAroundMidnight(t *testing.T) {
	g := Guards{QuietOn: true, QuietStart: "23:00", QuietEnd: "07:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 10, h, m, 0, 0, time.UTC) }
	for _, c := range []struct {
		h, m int
		want bool
	}{{22, 59, false}, {23, 0, true}, {2, 30, true}, {6, 59, true}, {7, 0, false}, {12, 0, false}} {
		if got := g.Quiet(at(c.h, c.m)); got != c.want {
			t.Errorf("%02d:%02d = %v, beklenen %v", c.h, c.m, got, c.want)
		}
	}
	if (Guards{QuietOn: false, QuietStart: "01:00", QuietEnd: "08:00"}).Quiet(at(3, 0)) {
		t.Error("kapalıyken sessiz saat olmamalı")
	}
	if (Guards{QuietOn: true, QuietStart: "bozuk", QuietEnd: "08:00"}).Quiet(at(3, 0)) {
		t.Error("bozuk saat sessiz sayılmamalı")
	}
}

func TestPersistenceAndFind(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/Ayşe.txt"
	var b []byte
	for i := 0; i < 40; i++ {
		who := "Ali"
		if i%2 == 1 {
			who = "Ayşe"
		}
		b = append(b, []byte("01.09.2026 20:"+two(i%60)+" - "+who+": mesaj\n")...)
	}
	writeFile(t, path, b)
	v, err := s.Import(path, "Ali")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Ayşe" || v.Owner != "Ali" {
		t.Fatalf("%+v", v)
	}
	g := Guards{FirstNApprovals: 2, MaxPerHour: 3, MaxPerDay: 9, QuietStart: "00:00", QuietEnd: "01:00"}
	watch := true
	if _, err := s.Update(v.ID, Patch{Watch: &watch, Guards: &g}); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir) // yeniden aç: kalıcı mı
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(v.ID)
	if err != nil || !got.Watch || got.Guards.MaxPerHour != 3 {
		t.Fatalf("kalıcılık: %+v %v", got, err)
	}
	for _, title := range []string{"Ayşe", "ayse", "Ayşe Yılmaz (iş)"} {
		if f, ok := s2.Find(title); !ok || f.ID != v.ID {
			t.Errorf("Find(%q) bulamadı", title)
		}
	}
	if _, ok := s2.Find("Mehmet"); ok {
		t.Error("alakasız ad eşleşti")
	}
}
