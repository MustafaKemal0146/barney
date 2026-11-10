package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MustafaKemal0146/barney/internal/chat"
	"github.com/MustafaKemal0146/barney/internal/engine"
	"github.com/MustafaKemal0146/barney/internal/history"
	"github.com/MustafaKemal0146/barney/internal/insights"
	"github.com/MustafaKemal0146/barney/internal/memory"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/people"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/watch"
	"github.com/coder/websocket"
)

// Services arayüz API'sinin dayandığı bileşenlerdir.
type Services struct {
	People *people.Store
	// Caller yapılandırılmış modeli verir; anahtar yoksa hata döner (mesajı kullanıcıya gösterilir).
	Caller func() (reply.Caller, error)
	Timing reply.Timing
}

// SettingsIn arayüzden gelen ayar değişikliğidir (nil = dokunma). Key "" = kayıtlı anahtarı sil.
type SettingsIn struct {
	Provider           string  `json:"provider"`
	Key                *string `json:"key"`
	Model              *string `json:"model"`
	Notifications      *bool   `json:"notifications"`
	LearnFeedback      *bool   `json:"learn_feedback"`
	HistoryMaxMessages *int    `json:"history_max_messages"`
	HistoryMaxSeconds  *int    `json:"history_max_seconds"`
	HistoryAll         *bool   `json:"history_all"`
	AutoLearn          *bool   `json:"auto_learn"`
	BulkSpeed          *string `json:"bulk_speed"`
}

// liveState uzantının bildirdiği açık sohbettir.
type liveState struct {
	Platform string
	Chat     wireChat
	At       time.Time
}

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) sub() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[chan []byte]struct{}{}
	}
	ch := make(chan []byte, 64)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsub(ch chan []byte) { h.mu.Lock(); delete(h.subs, ch); h.mu.Unlock() }

func (h *hub) publish(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default: // yavaş istemci: olay düşer, sonraki durum olayı toparlar
		}
	}
}

// ---- yardımcılar

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "geçersiz gövde", http.StatusBadRequest)
		return false
	}
	return true
}

func fail(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, people.ErrNotFound) {
		code = http.StatusNotFound
	}
	http.Error(w, err.Error(), code)
}

func (s *Server) person(w http.ResponseWriter, r *http.Request) (*people.View, bool) {
	v, err := s.Svc.People.Get(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return nil, false
	}
	return v, true
}

// ---- durum

type stateOut struct {
	Connected     bool        `json:"connected"`
	KeySet        bool        `json:"key_set"`
	Provider      string      `json:"provider"`
	Providers     []string    `json:"providers"`
	Model         string      `json:"model"`
	HasPeople     bool        `json:"has_people"`
	Paused        bool        `json:"paused"`
	Stats         watch.Stats `json:"stats"`
	ExtDir        string      `json:"ext_dir"`
	Version       string      `json:"version"`
	Notifications bool        `json:"notifications"`
	LearnFeedback bool        `json:"learn_feedback"`
	LAN           bool        `json:"lan"`
	History       HistoryCfg  `json:"history"`
	AutoLearn     bool        `json:"auto_learn"`
	BulkSpeed     string      `json:"bulk_speed"`
}

func (s *Server) state() stateOut {
	s.mu.Lock()
	info := s.Info
	connected := s.conn != nil
	s.mu.Unlock()
	st := s.getBrain().Stats()
	return stateOut{
		Connected: connected, KeySet: info.KeySet, Provider: info.Provider, Providers: info.Providers, Model: info.Model,
		HasPeople: s.Svc.People.Count() > 0, Paused: st.Paused, Stats: st, ExtDir: info.ExtDir, Version: "0.2.0",
		Notifications: info.Notifications, LearnFeedback: info.LearnFeedback, LAN: info.LAN,
		History: info.History, AutoLearn: info.AutoLearn, BulkSpeed: info.BulkSpeed,
	}
}

func (s *Server) publishState() { s.hub.publish(map[string]any{"type": "state", "state": s.state()}) }

// syncExtension durdur/sürdür durumunu bağlı uzantıya da bildirir (arayüzden değişince eşitlenir).
func (s *Server) syncExtension() {
	s.publishState()
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c != nil {
		st := s.getBrain().Stats()
		go s.write(context.Background(), c, outFrame{Type: "state", Stats: &st})
	}
}

// ---- canlı sohbet

type liveOut struct {
	Partner     string    `json:"partner"`
	History     []wireMsg `json:"history"`
	IncomingKey string    `json:"incoming_key"`
	PersonID    string    `json:"person_id,omitempty"`
}

func (s *Server) liveView() (string, *liveOut) {
	s.mu.Lock()
	l := s.live
	connected := s.conn != nil
	s.mu.Unlock()
	if !connected || l == nil {
		return "", nil
	}
	out := &liveOut{Partner: l.Chat.Partner, History: l.Chat.History, IncomingKey: l.Chat.IncomingKey}
	if v, ok := s.Svc.People.Find(l.Chat.Partner); ok {
		out.PersonID = v.ID
	}
	return l.Platform, out
}

// resyncLive "Senk" düğmesi: ekrandaki mesajları okur, canlı görünümü hemen tazeler ve kayıtlı kişinin
// geçmişine (10 sn sınırı olmadan, eşzamanlı) ekler. Yalnızca şu an DOM'da yüklü mesajlara bakar.
func (s *Server) resyncLive(f inFrame) {
	w := f.Chat
	platform := f.Platform
	if platform == "" {
		platform = "whatsapp"
	}
	s.mu.Lock()
	s.live = &liveState{Platform: platform, Chat: w, At: time.Now()}
	s.mu.Unlock()
	_, out := s.liveView()
	s.hub.publish(map[string]any{"type": "live", "live": out, "person_id": out.PersonID})
	added, known := 0, false
	if v, ok := s.Svc.People.Find(w.Partner); ok {
		known = true
		if prof, err := s.Svc.People.Profile(v.ID); err == nil {
			var msgs []model.Message
			if len(f.Messages) > 0 {
				msgs = wireToModel(f.Messages, prof.OwnerName, prof.PartnerName)
			} else {
				msgs = wireToModel(w.History, prof.OwnerName, prof.PartnerName)
			}
			added, _ = s.Svc.People.Merge(v.ID, msgs)
		}
	}
	s.hub.publish(map[string]any{"type": "resynced", "partner": w.Partner, "added": added, "known": known, "seen": max(len(f.Messages), len(w.History))})
}

func (s *Server) handleLiveResync(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		http.Error(w, "WhatsApp sekmesi bağlı değil: uzantıyı yenile ve web.whatsapp.com'u aç", http.StatusConflict)
		return
	}
	b, _ := jsonMarshal(outFrame{Type: "command", JobID: newID(), Cmd: "resync"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		http.Error(w, "uzantıya komut gönderilemedi", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) setLive(w wireChat, platform string) {
	if platform == "" {
		platform = "whatsapp"
	}
	s.mu.Lock()
	changed := s.live == nil || s.live.Chat.IncomingKey != w.IncomingKey || s.live.Chat.Partner != w.Partner || !sameHistory(s.live.Chat.History, w.History)
	s.live = &liveState{Platform: platform, Chat: w, At: time.Now()}
	s.mu.Unlock()
	if changed {
		s.learn(w)
		_, out := s.liveView()
		s.hub.publish(map[string]any{"type": "live", "live": out, "person_id": out.PersonID})
	}
}

// ---- model bağlamı

func (s *Server) buildCtx(id string, real []model.Message) (chat.Ctx, *people.View, error) {
	v, err := s.Svc.People.Get(id)
	if err != nil {
		return chat.Ctx{}, nil, err
	}
	prof, err := s.Svc.People.Profile(id)
	if err != nil {
		return chat.Ctx{}, nil, err
	}
	dir := s.Svc.People.Dir(id)
	mem := memory.Load(dir)
	stage := v.Stage
	if stage == "" {
		stage = mem.Stage
	}
	if real == nil {
		real, _ = s.Svc.People.Messages(id)
	}
	if len(real) > 30 {
		real = real[len(real)-30:]
	}
	var ex []reply.Example
	s.mu.Lock()
	learn := s.Info.LearnFeedback
	s.mu.Unlock()
	if learn {
		ex = history.Examples(dir, 5)
	}
	return chat.Ctx{
		OwnerName: prof.OwnerName, PartnerName: prof.PartnerName, Owner: prof.Owner, Partner: prof.Partner,
		Real: real, Tone: v.Tone, Goal: goalText(v.Goal), Stage: stage, Notes: v.Notes, Facts: memory.Texts(mem), Examples: ex,
	}, v, nil
}

func goalText(g string) string {
	switch g {
	case "flort":
		return "Kullanıcı bu kişiyle flört ediyor: doğal, saygılı, ilgi çekici ol; baskı yapma."
	case "arkadaslik":
		return "Bu kişiyle arkadaşlık ediliyor: sıcak ve rahat ol."
	case "is":
		return "Bu kişiyle iş ilişkisi var: net, kibar, profesyonel ol (ama kullanıcının yazma biçiminde)."
	case "aile":
		return "Bu kişi aileden: samimi ve ilgili ol."
	case "barisma":
		return "Kullanıcı bu kişiyle barışmak istiyor: yumuşak, samimi, suçlayıcı olmayan bir dil kullan."
	}
	return ""
}

func (s *Server) opts(council bool) (engine.Options, error) {
	c, err := s.Svc.Caller()
	if err != nil {
		return engine.Options{}, err
	}
	return engine.Options{Synth: c, Council: council, Timing: s.Svc.Timing}, nil
}

type candOut struct {
	Tur        string       `json:"tur"`
	Steps      []reply.Step `json:"steps"`
	Confident  bool         `json:"confident"`
	Blocked    bool         `json:"blocked"`
	Sensitive  bool         `json:"sensitive"`
	Violations []string     `json:"violations"`
}

func candsOut(d watch.Decision) []candOut {
	out := make([]candOut, 0, len(d.Cands))
	for _, c := range d.Cands {
		v := c.Violations
		if v == nil {
			v = []string{}
		}
		out = append(out, candOut{Tur: c.Tur, Steps: c.Steps, Confident: c.Confident, Blocked: c.Blocked, Sensitive: c.Sensitive, Violations: v})
	}
	return out
}

func joinSteps(st []reply.Step) string {
	p := make([]string, len(st))
	for i, x := range st {
		p[i] = x.Text
	}
	return strings.Join(p, " / ")
}

// remember önerileri geçmişe yazar ve gönderim eşleştirmesi için saklar.
func (s *Server) remember(personID, chatID string, d watch.Decision) {
	if personID == "" {
		return
	}
	dir := s.Svc.People.Dir(personID)
	s.mu.Lock()
	if s.lastCands == nil {
		s.lastCands = map[string][]string{}
	}
	s.lastCands[chatID] = nil
	for _, c := range d.Cands {
		t := joinSteps(c.Steps)
		s.lastCands[chatID] = append(s.lastCands[chatID], t)
	}
	s.mu.Unlock()
	for _, c := range d.Cands {
		_ = history.Add(dir, history.Entry{Kind: history.Suggested, Text: joinSteps(c.Steps)})
	}
}

// suggestFor modelden aday ister (kişi sayfası ve canlı sohbet ortak yolu).
func (s *Server) suggestFor(ctx context.Context, id string, hist []model.Message, goal string, opener bool) (watch.Decision, error) {
	cx, v, err := s.buildCtx(id, hist)
	if err != nil {
		return watch.Decision{}, err
	}
	o, err := s.opts(v.Council)
	if err != nil {
		return watch.Decision{}, err
	}
	g := cx.Goal
	if strings.TrimSpace(goal) != "" {
		g = strings.TrimSpace(goal)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	d, err := engine.Suggest(ctx, o, engine.SuggestInput{
		Owner: cx.Owner, Partner: cx.Partner, OwnerName: cx.OwnerName, PartnerName: cx.PartnerName,
		History: cx.Real, Goal: g, Stage: cx.Stage, Opener: opener, Tone: cx.Tone, Facts: cx.Facts, Examples: cx.Examples,
	})
	s.getBrain().AddCalls(max(d.Calls, 1))
	return d, err
}

// Decider Gözcü için kişi çözen karar işlevidir: açık sohbetin başlığından kişiyi bulur.
func (s *Server) Decider() watch.Decider {
	return func(ctx context.Context, c watch.Chat) (watch.Decision, error) {
		v, ok := s.Svc.People.Find(c.Partner)
		if !ok {
			return watch.Decision{}, fmt.Errorf("%q kişi olarak ekli değil: arayüzden ekle", c.Partner)
		}
		prof, err := s.Svc.People.Profile(v.ID)
		if err != nil {
			return watch.Decision{}, err
		}
		h := make([]model.Message, len(c.History))
		for i, m := range c.History {
			if m.Sender == s.OwnerName {
				m.Sender = prof.OwnerName
			} else {
				m.Sender = prof.PartnerName
			}
			h[i] = m
		}
		d, err := s.suggestForNoCount(ctx, v.ID, h)
		return d, err
	}
}

func (s *Server) suggestForNoCount(ctx context.Context, id string, hist []model.Message) (watch.Decision, error) {
	cx, v, err := s.buildCtx(id, hist)
	if err != nil {
		return watch.Decision{}, err
	}
	o, err := s.opts(v.Council)
	if err != nil {
		return watch.Decision{}, err
	}
	return engine.Suggest(ctx, o, engine.SuggestInput{
		Owner: cx.Owner, Partner: cx.Partner, OwnerName: cx.OwnerName, PartnerName: cx.PartnerName,
		History: cx.Real, Goal: cx.Goal, Stage: cx.Stage, Tone: cx.Tone, Facts: cx.Facts, Examples: cx.Examples,
	})
}

// Policy Gözcü'nün kişi bazlı kuralıdır.
func (s *Server) Policy(c watch.Chat) watch.Policy {
	v, ok := s.Svc.People.Find(c.Partner)
	if !ok {
		return watch.Policy{}
	}
	g := v.Guards
	left := max(0, g.FirstNApprovals-v.ApprovalsDone)
	if v.FullAuto {
		// Tam otonom: onay ve sessiz saat atlanır. Hassas konu, saatlik/günlük sınır ve Durdur geçerli kalır.
		return watch.Policy{Watch: true, Auto: true, Full: true, MaxPerHour: g.MaxPerHour, MaxPerDay: g.MaxPerDay, StopSensitive: true}
	}
	return watch.Policy{Watch: v.Watch, Auto: v.AutoSend, Quiet: g.Quiet(time.Now()), ApprovalsLeft: left,
		MaxPerHour: g.MaxPerHour, MaxPerDay: g.MaxPerDay, StopSensitive: g.StopSensitive}
}

// ---- uçlar

func (s *Server) apiRoutes(mux *http.ServeMux) {
	a := s.authed
	mux.HandleFunc("GET /api/state", a(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.state()) }))
	mux.HandleFunc("POST /api/settings", a(s.handleSettings))
	mux.HandleFunc("POST /api/pause", a(func(w http.ResponseWriter, _ *http.Request) {
		s.getBrain().Pause()
		s.publishState()
		writeJSON(w, 200, s.state())
	}))
	mux.HandleFunc("POST /api/resume", a(func(w http.ResponseWriter, _ *http.Request) {
		s.getBrain().Resume()
		s.publishState()
		writeJSON(w, 200, s.state())
	}))

	mux.HandleFunc("GET /api/people", a(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.Svc.People.List()) }))
	mux.HandleFunc("POST /api/people", a(s.handleImport))
	mux.HandleFunc("GET /api/people/{id}", a(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := s.person(w, r); ok {
			writeJSON(w, 200, v)
		}
	}))
	mux.HandleFunc("POST /api/people/{id}/owner", a(s.handleOwner))
	mux.HandleFunc("PATCH /api/people/{id}", a(s.handlePatch))
	mux.HandleFunc("DELETE /api/people/{id}", a(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Svc.People.Delete(r.PathValue("id")); err != nil {
			fail(w, err)
			return
		}
		s.publishState()
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/people/{id}/chat", a(s.handleChatGet))
	mux.HandleFunc("POST /api/people/{id}/chat", a(s.handleChatPost))
	mux.HandleFunc("DELETE /api/people/{id}/chat", a(s.handleChatClear))
	mux.HandleFunc("POST /api/people/{id}/suggest", a(s.handleSuggest))
	mux.HandleFunc("POST /api/people/{id}/nudge", a(s.handleNudge))
	mux.HandleFunc("GET /api/people/{id}/memory", a(s.handleMemory))
	mux.HandleFunc("POST /api/people/{id}/memory/refresh", a(s.handleMemoryRefresh))
	mux.HandleFunc("DELETE /api/people/{id}/memory/{fid}", a(s.handleMemoryDelete))
	mux.HandleFunc("GET /api/people/{id}/insights", a(s.handleInsights))
	mux.HandleFunc("GET /api/people/{id}/history", a(s.handleHistory))

	s.importRoutes(a, mux)
	mux.HandleFunc("GET /api/live", a(s.handleLive))
	mux.HandleFunc("POST /api/live/suggest", a(s.handleLiveSuggest))
	mux.HandleFunc("POST /api/live/resync", a(s.handleLiveResync))
	mux.HandleFunc("POST /api/live/send", a(s.handleLiveSend(false)))
	mux.HandleFunc("POST /api/live/approve", a(s.handleLiveSend(true)))
	mux.HandleFunc("GET /api/events", a(s.handleEvents))
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.OnSettings == nil {
		http.Error(w, "ayar desteklenmiyor", http.StatusNotImplemented)
		return
	}
	var in SettingsIn
	if !readJSON(w, r, &in) {
		return
	}
	if in.Key != nil {
		k := strings.TrimSpace(*in.Key)
		in.Key = &k
	}
	if err := s.OnSettings(in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishState()
	writeJSON(w, 200, s.state())
}

const maxUpload = 25 << 20

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "yükleme okunamadı (en fazla 25 MB)", http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "dosya yok", http.StatusBadRequest)
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext != ".zip" && ext != ".txt" && ext != ".json" {
		http.Error(w, "yalnız .zip, .txt ya da .json dışa aktarımı", http.StatusBadRequest)
		return
	}
	dir, err := os.MkdirTemp("", "barney-import-")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.RemoveAll(dir)
	tmp := filepath.Join(dir, "export"+ext)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, err := io.Copy(out, io.LimitReader(f, maxUpload+1))
	out.Close()
	if err != nil || n > maxUpload {
		http.Error(w, "dosya çok büyük (en fazla 25 MB)", http.StatusRequestEntityTooLarge)
		return
	}
	v, err := s.Svc.People.Import(tmp, strings.TrimSpace(r.FormValue("owner")))
	var need *people.ErrNeedsOwner
	if errors.As(err, &need) {
		writeJSON(w, http.StatusAccepted, map[string]any{"needs_owner": true, "id": need.ID, "owner_candidates": need.Candidates})
		return
	}
	if err != nil {
		http.Error(w, "içe aktarılamadı: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.publishState()
	writeJSON(w, 200, v)
}

func (s *Server) handleOwner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Owner string `json:"owner"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v, err := s.Svc.People.SetOwner(r.PathValue("id"), in.Owner)
	if err != nil {
		fail(w, err)
		return
	}
	s.publishState()
	writeJSON(w, 200, v)
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Goal        *string        `json:"goal"`
		Notes       *string        `json:"notes"`
		Stage       *string        `json:"stage"`
		Tone        *reply.Tone    `json:"tone"`
		Watch       *bool          `json:"watch"`
		AutoSend    *bool          `json:"auto_send"`
		FullAuto    *bool          `json:"full_auto"`
		Council     *bool          `json:"council"`
		Guards      *people.Guards `json:"guards"`
		SilenceDays *int           `json:"silence_days"`
		Aliases     *[]string      `json:"aliases"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if in.Goal != nil {
		switch *in.Goal {
		case "", "flort", "arkadaslik", "is", "aile", "barisma":
		default:
			http.Error(w, "geçersiz hedef", http.StatusBadRequest)
			return
		}
	}
	if in.Tone != nil {
		switch in.Tone.Preset {
		case "benim", "barney", "sade", "samimi", "flort":
		default:
			http.Error(w, "geçersiz ton", http.StatusBadRequest)
			return
		}
	}
	v, err := s.Svc.People.Update(id, people.Patch{Goal: in.Goal, Notes: in.Notes, Stage: in.Stage, Tone: in.Tone, Watch: in.Watch,
		AutoSend: in.AutoSend, FullAuto: in.FullAuto, Council: in.Council, Guards: in.Guards, SilenceDays: in.SilenceDays, Aliases: in.Aliases})
	if err != nil {
		fail(w, err)
		return
	}
	if in.Stage != nil {
		_ = memory.SetStage(s.Svc.People.Dir(id), *in.Stage)
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleChatGet(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	mode := r.URL.Query().Get("mode")
	if !chat.ValidMode(mode) {
		http.Error(w, "mode: advisor | roleplay", http.StatusBadRequest)
		return
	}
	ms, err := chat.Load(s.Svc.People.Dir(v.ID), mode)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"messages": ms})
}

func (s *Server) handleChatClear(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	mode := r.URL.Query().Get("mode")
	if !chat.ValidMode(mode) {
		http.Error(w, "mode: advisor | roleplay", http.StatusBadRequest)
		return
	}
	if err := chat.Clear(s.Svc.People.Dir(v.ID), mode); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChatPost(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	var in struct {
		Mode string `json:"mode"`
		Text string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if !chat.ValidMode(in.Mode) || in.Text == "" || len(in.Text) > 4000 {
		http.Error(w, "geçersiz mod ya da mesaj", http.StatusBadRequest)
		return
	}
	caller, err := s.Svc.Caller()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cx, _, err := s.buildCtx(v.ID, nil)
	if err != nil {
		fail(w, err)
		return
	}
	dir := s.Svc.People.Dir(v.ID)
	dialog, _ := chat.Load(dir, in.Mode)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	user := chat.NewMsg("user", in.Text, nil)
	var bot chat.Msg
	if in.Mode == chat.ModeAdvisor {
		text, err := chat.Advisor(ctx, caller, cx, dialog, in.Text)
		if err != nil {
			http.Error(w, "model hatası: "+err.Error(), http.StatusBadGateway)
			return
		}
		bot = chat.NewMsg("assistant", text, nil)
	} else {
		steps, err := chat.Roleplay(ctx, caller, cx, dialog, in.Text, s.Svc.Timing)
		if err != nil {
			http.Error(w, "model hatası: "+err.Error(), http.StatusBadGateway)
			return
		}
		bot = chat.NewMsg("assistant", joinSteps(steps), steps)
	}
	s.getBrain().AddCalls(1)
	if err := chat.Append(dir, in.Mode, user, bot); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"messages": []chat.Msg{user, bot}})
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	var in struct {
		Goal string `json:"goal"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	d, err := s.suggestFor(r.Context(), v.ID, nil, in.Goal, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.remember(v.ID, v.Name, d)
	writeJSON(w, 200, map[string]any{"cands": candsOut(d), "recommended": d.Recommended, "note": d.Note})
}

func (s *Server) handleNudge(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	d, err := s.suggestFor(r.Context(), v.ID, nil, "", true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.remember(v.ID, v.Name, d)
	writeJSON(w, 200, map[string]any{"cands": candsOut(d), "recommended": d.Recommended, "note": d.Note})
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	d := memory.Load(s.Svc.People.Dir(v.ID))
	stage := v.Stage
	if stage == "" {
		stage = d.Stage
	}
	writeJSON(w, 200, map[string]any{"facts": d.Facts, "stage": stage})
}

func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	if err := memory.Delete(s.Svc.People.Dir(v.ID), r.PathValue("fid")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMemoryRefresh(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	caller, err := s.Svc.Caller()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msgs, _ := s.Svc.People.Messages(v.ID)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	d, err := memory.Refresh(ctx, caller, s.Svc.People.Dir(v.ID), v.Owner, v.Name, msgs)
	s.getBrain().AddCalls(1)
	if err != nil {
		http.Error(w, "hafıza yenilenemedi: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, 200, map[string]any{"facts": d.Facts, "stage": d.Stage})
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	msgs, _ := s.Svc.People.Messages(v.ID)
	writeJSON(w, 200, insights.Analyze(msgs, v.Owner, time.Now()))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	v, ok := s.person(w, r)
	if !ok {
		return
	}
	h, err := history.List(s.Svc.People.Dir(v.ID))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	platform, out := s.liveView()
	m := map[string]any{"connected": out != nil || s.isConnected(), "platform": platform}
	if out != nil {
		m["chat"] = out
	}
	writeJSON(w, 200, m)
}

func (s *Server) isConnected() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.conn != nil }

func (s *Server) handleLiveSuggest(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	l := s.live
	s.mu.Unlock()
	if l == nil {
		http.Error(w, "açık sohbet yok: WhatsApp Web'de bir sohbet aç", http.StatusConflict)
		return
	}
	v, ok := s.Svc.People.Find(l.Chat.Partner)
	if !ok {
		http.Error(w, fmt.Sprintf("%q kişi olarak ekli değil: önce sohbet dışa aktarımını ekle", l.Chat.Partner), http.StatusNotFound)
		return
	}
	prof, err := s.Svc.People.Profile(v.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wc := s.toChat(l.Chat)
	for i := range wc.History {
		if wc.History[i].Sender == s.OwnerName {
			wc.History[i].Sender = prof.OwnerName
		} else {
			wc.History[i].Sender = prof.PartnerName
		}
	}
	d, err := s.suggestFor(r.Context(), v.ID, wc.History, "", false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.remember(v.ID, wc.ID, d)
	out := map[string]any{"cands": candsOut(d), "recommended": d.Recommended, "note": d.Note}
	s.hub.publish(map[string]any{"type": "suggestions", "chat_id": wc.ID, "incoming_key": wc.IncomingKey, "person_id": v.ID,
		"cands": candsOut(d), "recommended": d.Recommended, "note": d.Note})
	writeJSON(w, 200, out)
}

type sendIn struct {
	ChatID      string       `json:"chat_id"`
	IncomingKey string       `json:"incoming_key"`
	Steps       []reply.Step `json:"steps"`
	CandTur     string       `json:"cand_tur"`
	Draft       bool         `json:"draft"` // true: yalnız mesaj kutusuna yaz, gönderme
}

func (s *Server) handleLiveSend(approval bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in sendIn
		if !readJSON(w, r, &in) {
			return
		}
		if in.ChatID == "" || len(in.Steps) == 0 || len(in.Steps) > 6 {
			http.Error(w, "sohbet ya da mesaj eksik (en fazla 6 mesaj)", http.StatusBadRequest)
			return
		}
		for i, st := range in.Steps {
			in.Steps[i].Text = strings.TrimSpace(st.Text)
			if in.Steps[i].Text == "" || len([]rune(in.Steps[i].Text)) > 1000 {
				http.Error(w, "mesaj boş ya da çok uzun", http.StatusBadRequest)
				return
			}
			// Kullanıcı kendisi bastı: beklemeden gider (yazma süresi uzantıda zaten simüle edilir).
			// Yalnızca art arda mesajlar arasında kısa, insan gibi bir nefes payı kalır.
			in.Steps[i].WaitSec = 0
			if i > 0 {
				in.Steps[i].WaitSec = max(0, min(3, st.WaitSec))
			}
		}
		if !in.Draft && s.getBrain().Paused() { // taslak yazmak göndermek değildir: durdurulmuşken de olur
			http.Error(w, "Barney durduruldu: önce sürdür", http.StatusConflict)
			return
		}
		if !s.isConnected() {
			http.Error(w, "uzantı bağlı değil", http.StatusConflict)
			return
		}
		if approval {
			if v, ok := s.Svc.People.Find(in.ChatID); ok {
				s.Svc.People.AddApproval(v.ID)
				_ = history.Add(s.Svc.People.Dir(v.ID), history.Entry{Kind: history.Approved, Text: joinSteps(in.Steps)})
			}
		}
		kind := "send"
		if in.Draft {
			kind = "draft"
		}
		s.pushAction(watch.Action{Kind: kind, ChatID: in.ChatID, IncomingKey: in.IncomingKey, Steps: in.Steps, Recommended: -1}, false)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "akış desteklenmiyor", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	ch := s.hub.sub()
	defer s.hub.unsub(ch)
	send := func(b []byte) { fmt.Fprintf(w, "data: %s\n\n", b); fl.Flush() }
	st, _ := json.Marshal(map[string]any{"type": "state", "state": s.state()})
	send(st)
	if _, out := s.liveView(); out != nil {
		b, _ := json.Marshal(map[string]any{"type": "live", "live": out, "person_id": out.PersonID})
		send(b)
	}
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			send(b)
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// ---- SPA

// spaHandler gömülü arayüzü sunar; bilinmeyen yollar index.html'e düşer.
func (s *Server) spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		web := s.Web
		if web == nil {
			http.Error(w, "arayüz derlenmemiş: make web", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'self' chrome-extension:; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if st, err := fs.Stat(web, p); err != nil || st.IsDir() {
			p = "index.html"
			r.URL.Path = "/"
		}
		if p == "index.html" {
			w.Header().Set("Cache-Control", "no-store")
		}
		http.FileServerFS(web).ServeHTTP(w, r)
	})
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// learn kayıtlı kişinin canlı sohbetindeki mesajları geçmişine ekler (en çok 10 sn'de bir).
func sameHistory(a, b []wireMsg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].From != b[i].From || a[i].Text != b[i].Text || a[i].Kind != b[i].Kind {
			return false
		}
	}
	return true
}

func (s *Server) learn(w wireChat) {
	s.mu.Lock()
	on := s.Info.AutoLearn
	if s.learnAt == nil {
		s.learnAt = map[string]time.Time{}
		s.learnNext = map[string]wireChat{}
	}
	if !on {
		s.mu.Unlock()
		return
	}
	if wait := 10*time.Second - time.Since(s.learnAt[w.Partner]); wait > 0 {
		// Sınır içinde gelen son durum kaybolmasın: süre dolunca en güncel hâli öğrenilir.
		_, scheduled := s.learnNext[w.Partner]
		s.learnNext[w.Partner] = w
		if !scheduled {
			p := w.Partner
			time.AfterFunc(wait, func() {
				s.mu.Lock()
				nw, ok := s.learnNext[p]
				delete(s.learnNext, p)
				s.mu.Unlock()
				if ok {
					s.learn(nw)
				}
			})
		}
		s.mu.Unlock()
		return
	}
	s.learnAt[w.Partner] = time.Now()
	s.mu.Unlock()
	v, ok := s.Svc.People.Find(w.Partner)
	if !ok {
		return
	}
	prof, err := s.Svc.People.Profile(v.ID)
	if err != nil {
		return
	}
	msgs := wireToModel(w.History, prof.OwnerName, prof.PartnerName)
	go func() { _, _ = s.Svc.People.Merge(v.ID, msgs) }()
}
