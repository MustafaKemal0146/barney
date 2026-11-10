package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
)

type fake struct {
	out string
	err error
	got string
}

func (f *fake) Complete(_ context.Context, _, user string, _ int) (string, llm.Usage, error) {
	f.got = user
	return f.out, llm.Usage{}, f.err
}

func msgs() []model.Message {
	d := time.Now()
	return []model.Message{
		{Sender: "Ali", TS: d, Kind: model.KindText, Text: "tel 0532 123 45 67 ara"},
		{Sender: "Ayşe", TS: d, Kind: model.KindText, Text: "kahve severim"},
	}
}

func TestRefreshAddsDedupesAndRespectsDismissed(t *testing.T) {
	dir := t.TempDir()
	f := &fake{out: "işte: {\"gercekler\":[\"kahve sever\",\"kahve sever\",\"\",\"hafta sonu çalışıyor\"],\"asama\":\"yeni tanışıyorlar\"} bitti"}
	d, err := Refresh(context.Background(), f, dir, "Ali", "Ayşe", msgs())
	if err != nil || len(d.Facts) != 2 || d.Stage != "yeni tanışıyorlar" {
		t.Fatalf("%+v %v", d, err)
	}
	if want := "0532 123 45 67"; contains(f.got, want) {
		t.Errorf("telefon modele maskesiz gitti: %s", f.got)
	}
	if err := Delete(dir, d.Facts[0].ID); err != nil {
		t.Fatal(err)
	}
	d, _ = Refresh(context.Background(), f, dir, "Ali", "Ayşe", msgs())
	if len(d.Facts) != 1 {
		t.Fatalf("silinen olgu geri geldi: %+v", d.Facts)
	}
	if err := SetStage(dir, "yakınlaşıyorlar"); err != nil {
		t.Fatal(err)
	}
	if Load(dir).Stage != "yakınlaşıyorlar" {
		t.Fatal("aşama kaydedilmedi")
	}
	if got := Texts(Load(dir)); len(got) != 1 {
		t.Fatalf("%v", got)
	}
}

func TestRefreshErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Refresh(context.Background(), &fake{out: "json yok"}, dir, "A", "B", msgs()); err == nil {
		t.Error("geçersiz çıktı kabul edildi")
	}
	if _, err := Refresh(context.Background(), &fake{err: errors.New("ağ")}, dir, "A", "B", msgs()); err == nil {
		t.Error("model hatası yutuldu")
	}
	if _, err := Refresh(context.Background(), &fake{out: "{}"}, dir, "A", "B", nil); err == nil {
		t.Error("boş sohbet kabul edildi")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
