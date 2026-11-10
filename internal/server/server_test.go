package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/people"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/watch"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeCaller model yerine geçer: danışman düz metin, üretim JSON döner.
type fakeCaller struct {
	mu    sync.Mutex
	calls int
	err   error
	reply string
}

func (f *fakeCaller) Complete(_ context.Context, system, user string, _ int) (string, llm.Usage, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return "", llm.Usage{}, f.err
	}
	if strings.Contains(system, "danışmanısın") {
		return "şunu yaz: \"tamam gelirim\"", llm.Usage{}, nil
	}
	if f.reply != "" {
		return f.reply, llm.Usage{}, nil
	}
	return `{"mesajlar":["tamam gelirim"],"not":"deneme"}`, llm.Usage{}, nil
}

// export WhatsApp (gg.aa.yyyy) biçiminde sentetik bir sohbet üretir; sahip Ali, karşı taraf Ayşe.
func export(n int) string {
	var b strings.Builder
	t := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		who, txt := "Ali", fmt.Sprintf("selam napıyon %d", i)
		if i%2 == 1 {
			who, txt = "Ayşe", fmt.Sprintf("iyidir sen nasılsın %d", i)
		}
		fmt.Fprintf(&b, "%s - %s: %s\n", t.Format("02.01.2006 15:04"), who, txt)
		t = t.Add(7 * time.Minute)
	}
	return b.String()
}

type app struct {
	srv *Server
	ts  *httptest.Server
	fc  *fakeCaller
	log string
}

func newApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	store, err := people.Open(filepath.Join(dir, "people"))
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeCaller{}
	log := filepath.Join(dir, "log.jsonl")
	s := &Server{Secret: testKey, OwnerName: "Ali", LogPath: log,
		Svc: Services{People: store, Caller: func() (reply.Caller, error) { return fc, nil }, Timing: reply.DefaultTiming}}
	s.Info = Info{Provider: "deepseek", Providers: []string{"deepseek"}, KeySet: true, LearnFeedback: true}
	b := watch.New(watch.Config{Settle: 20 * time.Millisecond}, s.Decider(), s.Push)
	b.SetPolicy(s.Policy)
	s.SetBrain(b)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &app{srv: s, ts: ts, fc: fc, log: log}
}

// do kimlikli JSON isteği yapar; gövde out'a çözülür.
func (a *app) do(t *testing.T, method, path string, body, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		bb, _ := json.Marshal(body)
		rd = bytes.NewReader(bb)
	}
	req, _ := http.NewRequest(method, a.ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+testKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
	if resp.StatusCode >= 300 {
		t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp.StatusCode
}

// upload sentetik dışa aktarımı yükler.
func (a *app) upload(t *testing.T, owner string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "Ayşe.txt")
	_, _ = fw.Write([]byte(export(800)))
	if owner != "" {
		_ = mw.WriteField("owner", owner)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.ts.URL+"/api/people", &buf)
	req.Header.Set("Authorization", "Bearer "+testKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	data, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(data, &out)
	if resp.StatusCode >= 300 {
		t.Logf("upload -> %d %s", resp.StatusCode, data)
	}
	return resp.StatusCode, out
}

func (a *app) addPerson(t *testing.T) string {
	t.Helper()
	code, p := a.upload(t, "Ali")
	if code != 200 {
		t.Fatalf("yükleme %d %v", code, p)
	}
	return p["id"].(string)
}

func dial(t *testing.T, ts *httptest.Server, key, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	return websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws",
		&websocket.DialOptions{Subprotocols: []string{Subprotocol, key}, HTTPHeader: h})
}

func read(t *testing.T, c *websocket.Conn) outFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var f outFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func incoming(key string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "incoming", "chat": map[string]any{
		"id": "Ayşe", "partner": "Ayşe", "incoming_key": key,
		"history": []map[string]any{{"from": "me", "text": "napıyon"}, {"from": "them", "text": "gelir misin"}}}})
	return b
}

func TestAuth(t *testing.T) {
	ts := newApp(t).ts
	if _, _, err := dial(t, ts, "yanlis-anahtar", "chrome-extension://abc"); err == nil {
		t.Fatal("yanlış anahtar kabul edildi")
	}
	if _, _, err := dial(t, ts, testKey, "https://evil.example"); err == nil {
		t.Fatal("web sayfası kaynağı kabul edildi")
	}
	c, _, err := dial(t, ts, testKey, "chrome-extension://abc")
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if f := read(t, c); f.Type != "hello_ack" {
		t.Fatalf("%+v", f)
	}
}

func TestHostGuardAndBearer(t *testing.T) {
	a := newApp(t)
	req, _ := http.NewRequest("GET", a.ts.URL+"/api/state", nil)
	req.Host = "evil.example.com"
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("rebinding: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("GET", a.ts.URL+"/api/state", nil)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anahtarsız: %d", resp.StatusCode)
	}
	if code := a.do(t, "GET", "/api/state", nil, nil); code != 200 {
		t.Errorf("anahtarlı: %d", code)
	}
	for _, p := range []string{"/api/people", "/api/live", "/api/events"} {
		req, _ := http.NewRequest("GET", a.ts.URL+p, nil)
		if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 401 {
			t.Errorf("%s anahtarsız = %d", p, resp.StatusCode)
		}
	}
}

func TestImportListPatchDelete(t *testing.T) {
	a := newApp(t)
	// sahip verilmeden: "Ali" tek aday olmayabilir → 200 ya da 202; owner verilerek kesin çalışır
	id := a.addPerson(t)
	var list []map[string]any
	a.do(t, "GET", "/api/people", nil, &list)
	if len(list) != 1 || list[0]["name"] != "Ayşe" || list[0]["owner"] != "Ali" {
		t.Fatalf("%v", list)
	}
	if list[0]["messages_total"].(float64) != 800 {
		t.Fatalf("mesaj sayısı: %v", list[0]["messages_total"])
	}
	var p map[string]any
	if code := a.do(t, "PATCH", "/api/people/"+id, map[string]any{"goal": "flort", "notes": "yeni tanışıyoruz",
		"tone": map[string]any{"preset": "barney", "flort": 7, "cesaret": 12, "mizah": 5, "samimiyet": 6}, "watch": true}, &p); code != 200 {
		t.Fatalf("patch %d", code)
	}
	tone := p["tone"].(map[string]any)
	if p["goal"] != "flort" || tone["preset"] != "barney" || tone["cesaret"].(float64) != 10 || p["watch"] != true {
		t.Fatalf("patch sonucu: %v", p)
	}
	if code := a.do(t, "PATCH", "/api/people/"+id, map[string]any{"goal": "yanlis"}, nil); code != 400 {
		t.Errorf("geçersiz hedef = %d", code)
	}
	if code := a.do(t, "DELETE", "/api/people/"+id, nil, nil); code != 204 {
		t.Errorf("silme = %d", code)
	}
	if code := a.do(t, "GET", "/api/people/"+id, nil, nil); code != 404 {
		t.Errorf("silinen kişi = %d", code)
	}
}

func TestImportRejectsBadFile(t *testing.T) {
	a := newApp(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "x.exe")
	_, _ = fw.Write([]byte("MZ"))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.ts.URL+"/api/people", &buf)
	req.Header.Set("Authorization", "Bearer "+testKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 400 {
		t.Fatalf("exe kabul edildi: %d", resp.StatusCode)
	}
}

func TestAdvisorAndRoleplayChat(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	var out struct{ Messages []map[string]any }
	if code := a.do(t, "POST", "/api/people/"+id+"/chat", map[string]any{"mode": "advisor", "text": "ona ne yazayım?"}, &out); code != 200 {
		t.Fatalf("advisor %d", code)
	}
	if len(out.Messages) != 2 || !strings.Contains(out.Messages[1]["text"].(string), "tamam gelirim") {
		t.Fatalf("%v", out.Messages)
	}
	if code := a.do(t, "POST", "/api/people/"+id+"/chat", map[string]any{"mode": "roleplay", "text": "bu akşam müsait misin"}, &out); code != 200 {
		t.Fatalf("roleplay %d", code)
	}
	steps, _ := out.Messages[1]["steps"].([]any)
	if len(steps) == 0 {
		t.Fatalf("rol provasında adım yok: %v", out.Messages[1])
	}
	var hist struct{ Messages []map[string]any }
	a.do(t, "GET", "/api/people/"+id+"/chat?mode=advisor", nil, &hist)
	if len(hist.Messages) != 2 {
		t.Fatalf("kalıcılık: %v", hist)
	}
	a.do(t, "GET", "/api/people/"+id+"/chat?mode=roleplay", nil, &hist)
	if len(hist.Messages) != 2 {
		t.Fatalf("modlar karışmamalı: %v", hist)
	}
	if code := a.do(t, "DELETE", "/api/people/"+id+"/chat?mode=advisor", nil, nil); code != 204 {
		t.Fatal(code)
	}
	if code := a.do(t, "POST", "/api/people/"+id+"/chat", map[string]any{"mode": "x", "text": "a"}, nil); code != 400 {
		t.Fatalf("geçersiz mod = %d", code)
	}
	a.fc.err = errors.New("kota doldu")
	if code := a.do(t, "POST", "/api/people/"+id+"/chat", map[string]any{"mode": "advisor", "text": "tekrar"}, nil); code != http.StatusBadGateway {
		t.Fatalf("model hatası = %d", code)
	}
}

func TestSuggestInsightsHistoryMemory(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	var sug struct {
		Cands []map[string]any
	}
	// son mesaj sahibin (i=119 tek) olabilir: sahip Ali çift indekste; 120. mesaj Ayşe (i=119) → cevaplanabilir
	if code := a.do(t, "POST", "/api/people/"+id+"/suggest", map[string]any{}, &sug); code != 200 || len(sug.Cands) != 1 {
		t.Fatalf("suggest %d %v", code, sug)
	}
	var h []map[string]any
	a.do(t, "GET", "/api/people/"+id+"/history", nil, &h)
	if len(h) == 0 || h[0]["kind"] != "suggested" {
		t.Fatalf("geçmiş: %v", h)
	}
	var ins map[string]any
	a.do(t, "GET", "/api/people/"+id+"/insights", nil, &ins)
	if ins["total"].(float64) != 800 || ins["owner_share_pct"].(float64) != 50 {
		t.Fatalf("analiz: %v", ins)
	}
	a.fc.reply = `{"gercekler":["kahveyi sever","hafta sonu çalışıyor"],"asama":"yeni tanışıyorlar"}`
	var mem struct {
		Facts []map[string]any
		Stage string
	}
	if code := a.do(t, "POST", "/api/people/"+id+"/memory/refresh", nil, &mem); code != 200 || len(mem.Facts) != 2 || mem.Stage != "yeni tanışıyorlar" {
		t.Fatalf("hafıza %d %+v", code, mem)
	}
	fid := mem.Facts[0]["id"].(string)
	if code := a.do(t, "DELETE", "/api/people/"+id+"/memory/"+fid, nil, nil); code != 204 {
		t.Fatal(code)
	}
	a.do(t, "POST", "/api/people/"+id+"/memory/refresh", nil, &mem) // silinen tekrar eklenmemeli
	if len(mem.Facts) != 1 {
		t.Fatalf("silinen olgu geri geldi: %+v", mem.Facts)
	}
}

func TestSettingsEndpoint(t *testing.T) {
	a := newApp(t)
	var got SettingsIn
	a.srv.OnSettings = func(in SettingsIn) error {
		if in.Provider == "kotu" {
			return fmt.Errorf("bilinmeyen sağlayıcı")
		}
		got = in
		a.srv.UpdateInfo(func(i *Info) { i.KeySet = in.Key != nil && *in.Key != "" })
		return nil
	}
	key := "  abcdefgh12  "
	var st map[string]any
	body, _ := json.Marshal(map[string]any{"provider": "deepseek", "key": key})
	req, _ := http.NewRequest("POST", a.ts.URL+"/api/settings", bytes.NewReader(body))
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 401 {
		t.Fatalf("kimliksiz = %d", resp.StatusCode)
	}
	if code := a.do(t, "POST", "/api/settings", map[string]any{"provider": "deepseek", "key": key}, &st); code != 200 || *got.Key != "abcdefgh12" {
		t.Fatalf("kaydet = %d %v", code, got)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "abcdefgh12") || st["key_set"] != true {
		t.Fatalf("yanıt anahtarı sızdırıyor ya da key_set yok: %s", raw)
	}
	if code := a.do(t, "POST", "/api/settings", map[string]any{"provider": "kotu"}, nil); code != 400 {
		t.Fatalf("hata = %d", code)
	}
}

// ws bağlı uzantıyı taklit eder.
func connect(t *testing.T, a *app) *websocket.Conn {
	t.Helper()
	c, _, err := dial(t, a.ts, testKey, "chrome-extension://abc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if f := read(t, c); f.Type != "hello_ack" {
		t.Fatalf("%+v", f)
	}
	return c
}

func TestLiveSendNeedsExtensionAndValidates(t *testing.T) {
	a := newApp(t)
	steps := []map[string]any{{"text": "tamam gelirim", "wait_sec": 1}}
	body := map[string]any{"chat_id": "Ayşe", "incoming_key": "k1", "steps": steps}
	if code := a.do(t, "POST", "/api/live/send", body, nil); code != http.StatusConflict {
		t.Fatalf("uzantısız = %d", code)
	}
	c := connect(t, a)
	if code := a.do(t, "POST", "/api/live/send", map[string]any{"chat_id": "Ayşe", "steps": []map[string]any{{"text": "  "}}}, nil); code != 400 {
		t.Fatalf("boş mesaj = %d", code)
	}
	if code := a.do(t, "POST", "/api/live/send", body, nil); code != http.StatusAccepted {
		t.Fatalf("gönder = %d", code)
	}
	f := read(t, c)
	if f.Type != "action" || f.Action.Kind != "send" || f.Action.IncomingKey != "k1" || f.Action.Steps[0].Text != "tamam gelirim" {
		t.Fatalf("%+v", f)
	}
	a.do(t, "POST", "/api/pause", nil, nil)
	if code := a.do(t, "POST", "/api/live/send", body, nil); code != http.StatusConflict {
		t.Fatalf("duraklatılmışken gönderilmemeli: %d", code)
	}
}

func TestAutoSendGuards(t *testing.T) {
	// FirstN=1 → ilk cevap onay bekler; onaydan sonra otomatik gider.
	a := newApp(t)
	id := a.addPerson(t)
	g := map[string]any{"first_n_approvals": 1, "quiet_on": false, "quiet_start": "01:00", "quiet_end": "08:00", "max_per_hour": 6, "max_per_day": 30, "stop_sensitive": true}
	a.do(t, "PATCH", "/api/people/"+id, map[string]any{"watch": true, "auto_send": true, "guards": g}, nil)
	c := connect(t, a)
	_ = c.Write(context.Background(), websocket.MessageText, incoming("k1"))
	f := read(t, c)
	if f.Action == nil || f.Action.Kind != "suggest" || !f.Action.PendingApproval {
		t.Fatalf("ilk cevap onay beklemeli: %+v", f.Action)
	}
	if code := a.do(t, "POST", "/api/live/approve", map[string]any{"chat_id": "Ayşe", "incoming_key": "k1", "steps": []map[string]any{{"text": "tamam gelirim"}}, "cand_tur": "tek"}, nil); code != 202 {
		t.Fatalf("onay = %d", code)
	}
	if f := read(t, c); f.Action.Kind != "send" {
		t.Fatalf("%+v", f)
	}
	_ = c.Write(context.Background(), websocket.MessageText, incoming("k2"))
	if f := read(t, c); f.Action.Kind != "send" {
		t.Fatalf("onaydan sonra otomatik gitmeli: %+v", f.Action)
	}
	data, _ := os.ReadFile(a.log)
	if !strings.Contains(string(data), "send_planned") {
		t.Fatalf("günlük: %s", data)
	}
}

func TestAutoSendStopsOnSensitiveTopic(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	g := map[string]any{"first_n_approvals": 0, "quiet_on": false, "quiet_start": "01:00", "quiet_end": "08:00", "max_per_hour": 6, "max_per_day": 30, "stop_sensitive": true}
	a.do(t, "PATCH", "/api/people/"+id, map[string]any{"watch": true, "auto_send": true, "guards": g}, nil)
	c := connect(t, a)
	b, _ := json.Marshal(map[string]any{"type": "incoming", "chat": map[string]any{"id": "Ayşe", "partner": "Ayşe", "incoming_key": "s1",
		"history": []map[string]any{{"from": "me", "text": "napıyon"}, {"from": "them", "text": "bana iban atar mısın borç lazım"}}}})
	_ = c.Write(context.Background(), websocket.MessageText, b)
	f := read(t, c)
	if f.Action.Kind != "suggest" || !f.Action.PendingApproval {
		t.Fatalf("hassas konuda otomatik gönderilmemeli: %+v", f.Action)
	}
}

func TestUnknownChatIsNotWatched(t *testing.T) {
	a := newApp(t)
	c := connect(t, a)
	b, _ := json.Marshal(map[string]any{"type": "incoming", "chat": map[string]any{"id": "Yabancı", "partner": "Yabancı", "incoming_key": "x",
		"history": []map[string]any{{"from": "them", "text": "selam"}}}})
	_ = c.Write(context.Background(), websocket.MessageText, b)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("kişi olarak eklenmemiş sohbet izlenmemeli")
	}
	if a.fc.calls != 0 {
		t.Fatalf("model çağrıldı: %d", a.fc.calls)
	}
}

func TestPauseFrameStopsWatching(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	a.do(t, "PATCH", "/api/people/"+id, map[string]any{"watch": true}, nil)
	c := connect(t, a)
	_ = c.Write(context.Background(), websocket.MessageText, []byte(`{"type":"pause"}`))
	if f := read(t, c); f.Type != "state" || !f.Stats.Paused {
		t.Fatalf("%+v", f)
	}
	_ = c.Write(context.Background(), websocket.MessageText, incoming("k1"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("duraklatılmışken eylem gelmemeli")
	}
}

func TestLiveStateAndSSE(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	c := connect(t, a)
	_ = c.Write(context.Background(), websocket.MessageText, incoming("k1"))
	time.Sleep(100 * time.Millisecond)
	var live struct {
		Connected bool
		Platform  string
		Chat      map[string]any
	}
	a.do(t, "GET", "/api/live", nil, &live)
	if !live.Connected || live.Platform != "whatsapp" || live.Chat["person_id"] != id {
		t.Fatalf("%+v", live)
	}
	req, _ := http.NewRequest("GET", a.ts.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("içerik türü: %s", resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), `"type":"state"`) {
		t.Fatalf("ilk olay durum olmalı: %s", buf[:n])
	}
}

func TestSPAFallbackAndCSP(t *testing.T) {
	a := newApp(t)
	a.srv.Web = fstest.MapFS{"index.html": {Data: []byte("<html>barney</html>")}, "assets/a.js": {Data: []byte("x")}}
	for _, p := range []string{"/", "/p/abc/sohbet", "/assets/a.js"} {
		resp, err := http.Get(a.ts.URL + p)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", p, resp, err)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: CSP yok", p)
		}
	}
}

func TestLoadSecretPersists(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadSecret(dir)
	if err != nil || len(a) != 64 {
		t.Fatal(a, err)
	}
	b, _ := LoadSecret(dir)
	if a != b {
		t.Fatal("anahtar kalıcı değil")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "pairing.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("izin: %v", fi.Mode())
	}
}

// fakeExt uzantıyı taklit eder: komutlara yanıt verir. chats: sohbet adı → mesaj sayısı.
func fakeExt(t *testing.T, c *websocket.Conn, chats map[string]int, owner string) (opened *[]string) {
	t.Helper()
	var mu sync.Mutex
	var log []string
	cur := ""
	go func() {
		for {
			_, data, err := c.Read(context.Background())
			if err != nil {
				return
			}
			var f struct {
				Type, JobID, Cmd string
				Args             map[string]any
			}
			f.Type, f.JobID, f.Cmd = "", "", ""
			var raw map[string]any
			_ = json.Unmarshal(data, &raw)
			f.Type, _ = raw["type"].(string)
			f.JobID, _ = raw["job_id"].(string)
			f.Cmd, _ = raw["cmd"].(string)
			f.Args, _ = raw["args"].(map[string]any)
			if f.Type != "command" {
				continue
			}
			send := func(v map[string]any) {
				b, _ := json.Marshal(v)
				_ = c.Write(context.Background(), websocket.MessageText, b)
			}
			switch f.Cmd {
			case "open_chat":
				name, _ := f.Args["name"].(string)
				mu.Lock()
				log = append(log, name)
				cur = name
				mu.Unlock()
				if _, ok := chats[name]; !ok {
					send(map[string]any{"type": "command_error", "job_id": f.JobID, "error": "sohbet bulunamadı"})
					continue
				}
				send(map[string]any{"type": "command_result", "job_id": f.JobID, "ok": true})
			case "ai_snapshot":
				send(map[string]any{"type": "command_result", "job_id": f.JobID, "ok": true, "data": map[string]any{"title": "Ayşe", "side_rows": []any{}, "read_total": 0}})
			case "ai_act":
				mu.Lock()
				log = append(log, "ai:"+fmt.Sprint(f.Args["action"]))
				mu.Unlock()
				send(map[string]any{"type": "command_result", "job_id": f.JobID, "ok": true, "note": "ok"})
			case "list_chats":
				var rows []map[string]any
				for n := range chats {
					rows = append(rows, map[string]any{"name": n, "last": "x", "unread": 0})
				}
				send(map[string]any{"type": "chats", "job_id": f.JobID, "chats": rows})
				send(map[string]any{"type": "command_result", "job_id": f.JobID, "ok": true})
			case "scrape_current", "ai_collect":
				mu.Lock()
				name := cur
				mu.Unlock()
				if name == "" {
					for n := range chats {
						name = n
					}
				}
				n := chats[name]
				var msgs []map[string]any
				t0 := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
				for i := 0; i < n; i++ {
					from, txt := "me", fmt.Sprintf("selam napıyon %d", i)
					if i%2 == 1 {
						from, txt = "them", fmt.Sprintf("iyidir sen nasılsın %d", i)
					}
					msgs = append(msgs, map[string]any{"from": from, "text": txt, "ts": t0.Add(time.Duration(i) * 7 * time.Minute).Unix()})
				}
				send(map[string]any{"type": "history_progress", "job_id": f.JobID, "loaded": n / 2, "note": "yarısı", "chat_name": name})
				send(map[string]any{"type": "history_chunk", "job_id": f.JobID, "seq": 0, "partner": name, "owner": owner, "messages": msgs[:n/2]})
				send(map[string]any{"type": "history_chunk", "job_id": f.JobID, "seq": 1, "partner": name, "owner": owner, "messages": msgs[n/2:], "done": true})
				send(map[string]any{"type": "command_result", "job_id": f.JobID, "ok": true, "loaded": n})
			}
		}
	}()
	return &log
}

func waitJob(t *testing.T, a *app, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var j map[string]any
		a.do(t, "GET", "/api/import/"+id, nil, &j)
		if st := j["state"]; st == "done" || st == "failed" || st == "canceled" {
			return j
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("iş zamanında bitmedi")
	return nil
}

func TestImportCurrentCreatesThenMerges(t *testing.T) {
	a := newApp(t)
	if code := a.do(t, "POST", "/api/import/current", map[string]any{}, nil); code != http.StatusConflict {
		t.Fatalf("uzantısız = %d", code)
	}
	c := connect(t, a)
	fakeExt(t, c, map[string]int{"Ayşe": 200}, "Mustafa")
	var job map[string]any
	if code := a.do(t, "POST", "/api/import/current", map[string]any{"max_messages": 500}, &job); code != 202 {
		t.Fatalf("başlat = %d", code)
	}
	done := waitJob(t, a, job["id"].(string))
	if done["state"] != "done" {
		t.Fatalf("%v", done)
	}
	ids := done["person_ids"].([]any)
	if len(ids) != 1 {
		t.Fatalf("kişi oluşmadı: %v", done)
	}
	var p map[string]any
	a.do(t, "GET", "/api/people/"+ids[0].(string), nil, &p)
	if p["name"] != "Ayşe" || p["owner"] != "Mustafa" || p["messages_total"].(float64) != 200 {
		t.Fatalf("%v", p)
	}
	// aynı sohbet tekrar okununca çoğalmaz (birleştirme)
	a.do(t, "POST", "/api/import/current", map[string]any{}, &job)
	done = waitJob(t, a, job["id"].(string))
	a.do(t, "GET", "/api/people/"+ids[0].(string), nil, &p)
	if done["state"] != "done" || p["messages_total"].(float64) != 200 {
		t.Fatalf("birleştirme: %v %v", done, p["messages_total"])
	}
}

func TestImportBulkAndOpen(t *testing.T) {
	a := newApp(t)
	a.srv.UpdateInfo(func(i *Info) { i.BulkSpeed = "hizli" })
	c := connect(t, a)
	opened := fakeExt(t, c, map[string]int{"Ayşe": 40, "Mehmet": 30}, "Mustafa")
	var job map[string]any
	if code := a.do(t, "POST", "/api/import/bulk", map[string]any{"names": []string{"Ayşe", "Yok Kişi", "Mehmet"}}, &job); code != 202 {
		t.Fatalf("toplu = %d", code)
	}
	done := waitJob(t, a, job["id"].(string))
	if done["state"] != "done" || len(done["person_ids"].([]any)) != 2 {
		t.Fatalf("%v", done)
	}
	if got := strings.Join(*opened, ","); got != "Ayşe,Yok Kişi,Mehmet" {
		t.Fatalf("açılan sohbetler: %s", got)
	}
	var list []map[string]any
	a.do(t, "GET", "/api/people", nil, &list)
	if len(list) != 2 {
		t.Fatalf("%v", list)
	}
	id := list[0]["id"].(string)
	if code := a.do(t, "POST", "/api/people/"+id+"/open", nil, nil); code != 202 {
		t.Fatalf("aç = %d", code)
	}
	if code := a.do(t, "POST", "/api/import/bulk", map[string]any{"names": []string{}}, nil); code != 400 {
		t.Fatalf("boş liste = %d", code)
	}
}

func TestImportCancelAndFallbackFlag(t *testing.T) {
	a := newApp(t)
	c := connect(t, a)
	// hiç yanıt vermeyen uzantı: iptal edilince iş "canceled" olmalı
	go func() { // yanıt vermeyen ama bağlantıyı açık tutan uzantı
		for {
			if _, _, err := c.Read(context.Background()); err != nil {
				return
			}
		}
	}()
	var job map[string]any
	if code := a.do(t, "POST", "/api/import/current", map[string]any{"max_seconds": 15}, &job); code != 202 {
		t.Fatal(code)
	}
	if code := a.do(t, "POST", "/api/import/current", map[string]any{}, nil); code != http.StatusConflict {
		t.Fatalf("ikinci iş = %d", code)
	}
	a.do(t, "POST", "/api/import/"+job["id"].(string)+"/cancel", nil, nil)
	if j := waitJob(t, a, job["id"].(string)); j["state"] != "canceled" {
		t.Fatalf("%v", j)
	}
	// boş okuma → failed + can_fallback_ai
	c2 := connect(t, a)
	fakeExt(t, c2, map[string]int{"Ayşe": 0}, "M")
	a.do(t, "POST", "/api/import/current", map[string]any{}, &job)
	j := waitJob(t, a, job["id"].(string))
	if j["state"] != "failed" || j["can_fallback_ai"] != true {
		t.Fatalf("%v", j)
	}
}

func TestAutoLearnMergesLiveMessages(t *testing.T) {
	a := newApp(t)
	id := a.addPerson(t)
	a.srv.UpdateInfo(func(i *Info) { i.AutoLearn = true })
	c := connect(t, a)
	b, _ := json.Marshal(map[string]any{"type": "incoming", "chat": map[string]any{"id": "Ayşe", "partner": "Ayşe", "incoming_key": "z1",
		"history": []map[string]any{{"from": "me", "text": "yeni bir mesaj benden", "ts": time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Unix()},
			{"from": "them", "text": "yeni cevap ondan", "ts": time.Date(2026, 11, 1, 10, 1, 0, 0, time.UTC).Unix()}}}})
	_ = c.Write(context.Background(), websocket.MessageText, b)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var p map[string]any
		a.do(t, "GET", "/api/people/"+id, nil, &p)
		if p["messages_total"].(float64) == 802 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("canlı mesajlar geçmişe eklenmedi")
}

func TestImportAIModeUsesAgentAndNeverSends(t *testing.T) {
	a := newApp(t)
	c := connect(t, a)
	log := fakeExt(t, c, map[string]int{"Ayşe": 60}, "Mustafa")
	// model önce yasak bir eylem dener, sonra okur ve bitirir
	seq := []string{`{"eylem":"send_message","metin":"selam"}`, `{"eylem":"read"}`, `{"eylem":"scroll_up"}`, `{"eylem":"done"}`}
	var mu sync.Mutex
	i := 0
	a.srv.Svc.Caller = func() (reply.Caller, error) {
		return callerFunc(func(string, string) string {
			mu.Lock()
			defer mu.Unlock()
			o := seq[min(i, len(seq)-1)]
			i++
			return o
		}), nil
	}
	var job map[string]any
	if code := a.do(t, "POST", "/api/import/current", map[string]any{"mode": "ai"}, &job); code != 202 {
		t.Fatalf("ai başlat = %d", code)
	}
	done := waitJob(t, a, job["id"].(string))
	if done["state"] != "done" || len(done["person_ids"].([]any)) != 1 {
		t.Fatalf("%v", done)
	}
	for _, l := range *log {
		if strings.Contains(l, "send") {
			t.Fatalf("yasak eylem uzantıya gitti: %v", *log)
		}
	}
	got := strings.Join(*log, ",")
	if !strings.Contains(got, "ai:read") || !strings.Contains(got, "ai:scroll_up") {
		t.Fatalf("eylemler: %s", got)
	}
}

type callerFunc func(system, user string) string

func (f callerFunc) Complete(_ context.Context, system, user string, _ int) (string, llm.Usage, error) {
	return f(system, user), llm.Usage{}, nil
}

// Düzenlenen mesaj (aynı incoming_key, farklı metin): canlı görünüm tazelenir, yapay zekâ çağrılmaz.
func TestLiveUpdateRefreshesWithoutAI(t *testing.T) {
	a := newApp(t)
	a.addPerson(t)
	c := connect(t, a)
	frame := func(text string) []byte {
		b, _ := json.Marshal(map[string]any{"type": "live_update", "chat": map[string]any{"id": "Ayşe", "partner": "Ayşe", "incoming_key": "e1",
			"history": []map[string]any{{"from": "them", "text": "selam", "ts": 1}, {"from": "me", "text": text, "ts": 2}}}})
		return b
	}
	_ = c.Write(context.Background(), websocket.MessageText, frame("naber"))
	_ = c.Write(context.Background(), websocket.MessageText, frame("naberr"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var live struct {
			Chat struct{ History []map[string]any }
		}
		a.do(t, "GET", "/api/live", nil, &live)
		if h := live.Chat.History; len(h) == 2 && h[1]["text"] == "naberr" {
			var st struct {
				Stats struct {
					CallsToday int `json:"calls_today"`
				}
			}
			a.do(t, "GET", "/api/state", nil, &st)
			if st.Stats.CallsToday != 0 {
				t.Fatalf("live_update yapay zekâ çağırdı: %d", st.Stats.CallsToday)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("düzenlenen mesaj canlı görünüme yansımadı")
}
