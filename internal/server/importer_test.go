package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/MustafaKemal0146/barney/internal/browseragent"
)

func TestWireToModel(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Unix()
	in := []wireMsg{
		{From: "me", Text: " selam ", TS: t0},
		{From: "them", Text: "", TS: t0 + 60},       // boş metin atılır
		{From: "them", Kind: "media", TS: t0 + 120}, // medya kalır (metinsiz)
		{From: "them", Text: "zamansız", TS: 0},     // zamanı öncekinden taşır
	}
	out := wireToModel(in, "Ali", "Ayşe")
	if len(out) != 3 || out[0].Sender != "Ali" || out[0].Text != "selam" || out[1].Kind != "media" || out[2].Sender != "Ayşe" || out[2].TS.Unix() != t0+120 {
		t.Fatalf("%+v", out)
	}
	if got := wireToModel([]wireMsg{{From: "me", Text: "x"}}, "A", "B"); len(got) != 0 {
		t.Fatalf("zamansız ilk mesaj atılmalı: %+v", got)
	}
}

func TestNormOptsClampsAndDefaults(t *testing.T) {
	s := &Server{}
	s.Info.History = HistoryCfg{MaxMessages: 3000, MaxSeconds: 300}
	o := s.normOpts(ImportOpts{})
	if o.MaxMessages != 3000 || o.MaxSeconds != 300 || o.Mode != "reader" || o.All {
		t.Fatalf("%+v", o)
	}
	o = s.normOpts(ImportOpts{MaxMessages: 5, MaxSeconds: 1, Mode: "weird"})
	if o.MaxMessages != 50 || o.MaxSeconds != 15 || o.Mode != "reader" {
		t.Fatalf("alt sınır: %+v", o)
	}
	o = s.normOpts(ImportOpts{MaxMessages: 9999999, MaxSeconds: 999999, Mode: "ai"})
	if o.MaxMessages != 100000 || o.MaxSeconds != 3600 || o.Mode != "ai" {
		t.Fatalf("üst sınır: %+v", o)
	}
}

func TestExtractedToWireTimestamps(t *testing.T) {
	out := extractedToWire([]browseragent.Msg{{From: "me", Text: "a", TS: "2026-11-01T10:00:00Z"}, {From: "x", Text: "b"}})
	if out[0].TS != time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Unix() || out[0].From != "me" || out[1].From != "them" || out[1].TS == 0 {
		t.Fatalf("%+v", out)
	}
}

func TestGarbageAndUnexpectedFramesAreHarmless(t *testing.T) {
	a := newApp(t)
	c := connect(t, a)
	for _, raw := range []string{
		`not json`, `{}`, `{"type":"history_chunk","job_id":"nobody","messages":[{"from":"me","text":"x","ts":1}]}`,
		`{"type":"command_result","job_id":"nobody","ok":true}`, `{"type":"chats","chats":null}`, `{"type":"unknown"}`,
	} {
		_ = c.Write(context.Background(), websocket.MessageText, []byte(raw))
	}
	time.Sleep(150 * time.Millisecond)
	a.srv.imp.mu.Lock()
	n := len(a.srv.imp.chunks)
	a.srv.imp.mu.Unlock()
	if n != 0 {
		t.Fatalf("beklenmeyen parça biriktirildi: %d", n)
	}
	if code := a.do(t, "GET", "/api/state", nil, nil); code != 200 {
		t.Fatal("sunucu bozuldu")
	}
	b, _ := json.Marshal(map[string]any{"type": "incoming", "chat": map[string]any{"id": "x", "partner": "x", "incoming_key": "k", "history": []any{}}})
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatalf("bağlantı kapandı: %v", err)
	}
}
