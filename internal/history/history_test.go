package history

import "testing"

func TestAddListExamplesClear(t *testing.T) {
	dir := t.TempDir()
	if l, err := List(dir); err != nil || len(l) != 0 {
		t.Fatalf("boş liste: %v %v", l, err)
	}
	for _, e := range []Entry{
		{Kind: Suggested, Text: "a"}, {Kind: Edited, Text: "öner1", Final: "gönderilen1"}, {Kind: Sent, Text: "b"},
		{Kind: Rejected, Text: "öner2"}, {Kind: Edited, Text: "öner3", Final: "gönderilen3"},
	} {
		if err := Add(dir, e); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := List(dir)
	if len(l) != 5 || l[0].TS == 0 {
		t.Fatalf("%v", l)
	}
	ex := Examples(dir, 2) // en yeni 2 düzeltme/ret, yeniden eskiye
	if len(ex) != 2 || ex[0].Suggested != "öner3" || ex[0].Final != "gönderilen3" || ex[1].Suggested != "öner2" || ex[1].Final != "" {
		t.Fatalf("%+v", ex)
	}
	if err := Clear(dir); err != nil {
		t.Fatal(err)
	}
	if l, _ := List(dir); len(l) != 0 {
		t.Fatal("temizlenmedi")
	}
	if err := Clear(dir); err != nil {
		t.Fatalf("olmayan dosyayı silmek hata vermemeli: %v", err)
	}
}
