package browserbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeExt bir sahte uzantı: gelen çağrılara yanıt fn ile üretilir.
func startBridge(t *testing.T, fn func(c Call) *Reply) (*Bridge, func()) {
	t.Helper()
	b := New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		b.Serve(r.Context(), c)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	c, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","version":"1","extVersion":"0.1.0","browser":"Chrome/test"}`))
	go func() {
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var call Call
			if json.Unmarshal(data, &call) != nil || call.Type != "call" {
				continue
			}
			if r := fn(call); r != nil {
				r.ID, r.Type = call.ID, "result"
				out, _ := json.Marshal(r)
				_ = c.Write(ctx, websocket.MessageText, out)
			}
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !b.Connected() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return b, func() { cancel(); srv.Close() }
}

func TestCallRoundTripAndTabTracking(t *testing.T) {
	b, stop := startBridge(t, func(c Call) *Reply {
		return &Reply{OK: true, Content: []Content{{Type: "text", Text: "ok " + c.Action}}, Tab: &TabInfo{ID: 7, URL: "https://ctf.example.com/x", Title: "T"}}
	})
	defer stop()
	r, err := b.Call(context.Background(), Call{Action: "read_page", TabID: 7})
	if err != nil || r.Content[0].Text != "ok read_page" {
		t.Fatalf("got %+v err=%v", r, err)
	}
	if got := b.TabURL(0); got != "https://ctf.example.com/x" {
		t.Fatalf("TabURL=%q", got)
	}
	if st := b.Status(); !st.Connected || st.ExtVersion != "0.1.0" || st.ActiveTab == nil {
		t.Fatalf("status=%+v", st)
	}
}

func TestCallNotConnected(t *testing.T) {
	_, err := New().Call(context.Background(), Call{Action: "tabs_context"})
	if !IsNotConnected(err) {
		t.Fatalf("beklenen not_connected, gelen %v", err)
	}
}

func TestCallErrorReply(t *testing.T) {
	b, stop := startBridge(t, func(c Call) *Reply {
		return &Reply{Error: &Error{Code: "host_changed", Message: "sayfa değişti"}}
	})
	defer stop()
	_, err := b.Call(context.Background(), Call{Action: "click", ExpectHost: "a.com"})
	if e, ok := err.(*Error); !ok || e.Code != "host_changed" {
		t.Fatalf("err=%v", err)
	}
}

func TestDisconnectFailsPending(t *testing.T) {
	b, stop := startBridge(t, func(c Call) *Reply { return nil }) // asla yanıt vermez
	done := make(chan error, 1)
	go func() { _, err := b.Call(context.Background(), Call{Action: "screenshot"}); done <- err }()
	time.Sleep(100 * time.Millisecond)
	stop()
	select {
	case err := <-done:
		if e, ok := err.(*Error); !ok || e.Code != ErrDisconnected {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bekleyen çağrı bağlantı kopunca bitmedi")
	}
}

func TestContextCancel(t *testing.T) {
	b, stop := startBridge(t, func(c Call) *Reply { return nil })
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.Call(ctx, Call{Action: "screenshot"}); err == nil {
		t.Fatal("iptalde hata bekleniyordu")
	}
}

func TestSameTabCallsSerialize(t *testing.T) {
	order := make(chan string, 4)
	b, stop := startBridge(t, func(c Call) *Reply {
		order <- "start"
		time.Sleep(60 * time.Millisecond)
		order <- "end"
		return &Reply{OK: true}
	})
	defer stop()
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() { _, _ = b.Call(context.Background(), Call{Action: "click", TabID: 3}); done <- struct{}{} }()
	}
	<-done
	<-done
	close(order)
	var seq []string
	for s := range order {
		seq = append(seq, s)
	}
	want := []string{"start", "end", "start", "end"}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("aynı sekme çağrıları sıralanmadı: %v", seq)
		}
	}
}
