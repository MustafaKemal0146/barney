package browseragent

import (
	"context"
	"strings"
	"testing"

	"github.com/MustafaKemal0146/barney/internal/llm"
)

type scripted struct {
	out []string
	i   int
}

func (s *scripted) Complete(_ context.Context, _, _ string, _ int) (string, llm.Usage, error) {
	o := s.out[min(s.i, len(s.out)-1)]
	s.i++
	return o, llm.Usage{}, nil
}

type fakeDriver struct {
	acts  []Action
	snaps []Snapshot
	n     int
	logs  []string
}

func (f *fakeDriver) Snapshot(context.Context, bool) (Snapshot, error) {
	s := f.snaps[min(f.n, len(f.snaps)-1)]
	f.n++
	return s, nil
}
func (f *fakeDriver) Act(_ context.Context, a Action) (string, error) {
	f.acts = append(f.acts, a)
	return "ok", nil
}
func (f *fakeDriver) Log(l string) { f.logs = append(f.logs, l) }

func TestRunOpensChatReadsAndFinishes(t *testing.T) {
	d := &fakeDriver{snaps: []Snapshot{{SideRows: []Row{{Ref: "r1", Name: "Ayşe"}}}, {}, {Read: 10}, {Read: 10}}}
	m := &scripted{out: []string{
		`{"eylem":"click_chat","ref":"r1","neden":"aç"}`, `{"eylem":"read"}`, `{"eylem":"scroll_up"}`, `{"eylem":"done"}`}}
	res, err := Run(context.Background(), m, d, Goal{OpenName: "Ayşe", MaxMessages: 100})
	if err != nil || res.Calls != 4 {
		t.Fatalf("%+v %v", res, err)
	}
	if len(d.acts) != 3 || d.acts[0].Action != ClickChat {
		t.Fatalf("%+v", d.acts)
	}
}

func TestRunRejectsForbiddenAndUnknownRefs(t *testing.T) {
	d := &fakeDriver{snaps: []Snapshot{{SideRows: []Row{{Ref: "r1", Name: "Ayşe"}}}}}
	m := &scripted{out: []string{
		`{"eylem":"send_message","metin":"merhaba"}`,                        // yasak
		`{"eylem":"click_chat","ref":"composer"}`,                           // bilinmeyen ref
		`{"eylem":"type_search","metin":"` + strings.Repeat("a", 90) + `"}`, // çok uzun
		`{"eylem":"done"}`}}
	if _, err := Run(context.Background(), m, d, Goal{}); err != nil {
		t.Fatal(err)
	}
	if len(d.acts) != 0 {
		t.Fatalf("yasak eylem yürütüldü: %+v", d.acts)
	}
}

func TestRunStopsWhenAtTopAndNoNewMessages(t *testing.T) {
	d := &fakeDriver{snaps: []Snapshot{{Read: 50, AtTop: true}}}
	m := &scripted{out: []string{`{"eylem":"read"}`}}
	res, err := Run(context.Background(), m, d, Goal{All: true})
	if err != nil || res.Steps > 5 {
		t.Fatalf("takılmamalı: %+v %v", res, err)
	}
}

func TestRunExtractFallbackAndFail(t *testing.T) {
	d := &fakeDriver{snaps: []Snapshot{{TextTail: "selam"}}}
	m := &scripted{out: []string{`{"eylem":"extract","mesajlar":[{"from":"them","text":"selam"}]}`, `{"eylem":"done"}`}}
	res, err := Run(context.Background(), m, d, Goal{})
	if err != nil || len(res.Extracted) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	m = &scripted{out: []string{`{"eylem":"fail","neden":"liste yok"}`}}
	if _, err := Run(context.Background(), m, &fakeDriver{snaps: []Snapshot{{}}}, Goal{}); err == nil {
		t.Fatal("fail hata dönmeli")
	}
}
