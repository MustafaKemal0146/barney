package engine

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/style"
	"github.com/MustafaKemal0146/barney/internal/watch"
)

type fake struct{ n atomic.Int64 }

func (f *fake) Complete(_ context.Context, system, _ string, _ int) (string, llm.Usage, error) {
	f.n.Add(1)
	switch {
	case strings.Contains(system, `Sen "Vicdan"`):
		return `{"kararlar":[{"tur":"guvenli","uygun":true,"sebep":""},{"tur":"komik","uygun":true,"sebep":""},{"tur":"cesur","uygun":true,"sebep":""}]}`, llm.Usage{}, nil
	case strings.Contains(system, "ÜÇ aday"):
		return `{"adaylar":[{"tur":"guvenli","mesajlar":["tamam"]},{"tur":"komik","mesajlar":["tamam"]},{"tur":"cesur","mesajlar":["tamam"]}],"not":""}`, llm.Usage{}, nil
	case strings.Contains(system, `Sen "`):
		return `{"ozet":"a","oneri":"b","uyari":""}`, llm.Usage{}, nil
	}
	return `{"mesajlar":["tamam"],"not":"n"}`, llm.Usage{}, nil
}

func profile() Profile {
	c := style.Card{Who: "Ali", Messages: 500, Confidence: style.ConfMid, Burst: style.BurstStats{Bursts: 10, P1: 1, MaxSize: 1},
		Length: style.LengthStats{WordsP90: 4}, Case: style.CaseStats{AllLowerRatio: 1, LowerStartRatio: 1}, Punct: style.PunctStats{EndNoneRatio: 1}}
	return Profile{OwnerName: "Ali", Owner: c, Partners: map[string]style.Card{}}
}

func chat() watch.Chat {
	return watch.Chat{ID: "Ayşe", Partner: "Ayşe", History: []model.Message{
		{Sender: "Ali", Kind: model.KindText, Text: "napıyon", TS: time.Now()},
		{Sender: "Ayşe", Kind: model.KindText, Text: "gelir misin", TS: time.Now()},
	}}
}

func TestQuickAndCouncilDeciders(t *testing.T) {
	f := &fake{}
	d, err := NewDecider(profile(), Options{Synth: f})
	if err != nil {
		t.Fatal(err)
	}
	dec, err := d(context.Background(), chat())
	if err != nil || len(dec.Cands) != 1 || !dec.Cands[0].Confident || dec.Recommended != 0 || f.n.Load() != 1 {
		t.Fatalf("%+v %v calls=%d", dec, err, f.n.Load())
	}
	f2 := &fake{}
	d, _ = NewDecider(profile(), Options{Synth: f2, Council: true})
	dec, err = d(context.Background(), chat())
	if err != nil || len(dec.Cands) != 3 || dec.Recommended != 0 || dec.Calls != 5 {
		t.Fatalf("%+v %v", dec, err)
	}
}

func TestRequiresProfile(t *testing.T) {
	if _, err := NewDecider(Profile{}, Options{Synth: &fake{}}); err == nil {
		t.Fatal("boş profil reddedilmeli")
	}
}
