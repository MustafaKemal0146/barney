package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/MustafaKemal0146/barney/internal/browseragent"
	"github.com/MustafaKemal0146/barney/internal/model"
)

// ImportOpts okuma derinliği ve yöntemi.
type ImportOpts struct {
	MaxMessages int    `json:"max_messages"`
	MaxSeconds  int    `json:"max_seconds"`
	All         bool   `json:"all"`
	Mode        string `json:"mode"` // reader | ai
}

// ImportJob arayüzün gördüğü iş durumudur.
type ImportJob struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`  // current | bulk | sync
	State         string   `json:"state"` // queued | running | done | failed | canceled
	Mode          string   `json:"mode"`
	Loaded        int      `json:"loaded"`
	Chat          string   `json:"chat"`
	I             int      `json:"i"`
	N             int      `json:"n"`
	PersonIDs     []string `json:"person_ids"`
	Note          string   `json:"note"`
	Log           []string `json:"log"`
	StartedAt     int64    `json:"started_at"`
	Error         string   `json:"error,omitempty"`
	CanFallbackAI bool     `json:"can_fallback_ai,omitempty"`
}

// extRes uzantının bir komuta verdiği sonuç.
type extRes struct {
	ID    string
	OK    bool
	Note  string
	Error string
	Count int
	Fall  bool // okuyucu seçicileri bozuk: AI yedeği önerilir
	Chat  string
	Owner string
	Data  json.RawMessage
}

type chunkBuf struct {
	msgs           []wireMsg
	partner, owner string
}

// importer tek seferde tek iş yürütür ve uzantıya komut gönderir.
type importer struct {
	mu       sync.Mutex
	job      *ImportJob
	cancel   context.CancelFunc
	stopKeep atomic.Bool // Durdur: okunanları kaydet, kalan işleri bırak

	cmdMu    sync.Mutex // aynı anda tek komut
	res      chan extRes
	cur      string // bekleyen komut kimliği
	progress func(loaded int, note string, chat string, i, n int)
	chunks   map[string]*chunkBuf
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (im *importer) init() {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.res == nil {
		im.res = make(chan extRes, 64)
		im.chunks = map[string]*chunkBuf{}
	}
}

func (im *importer) snapshot() *ImportJob {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.job == nil {
		return nil
	}
	j := *im.job
	j.Log = append([]string(nil), im.job.Log...)
	j.PersonIDs = append([]string(nil), im.job.PersonIDs...)
	return &j
}

func (s *Server) importUpdate(f func(j *ImportJob)) {
	s.imp.mu.Lock()
	if s.imp.job != nil {
		f(s.imp.job)
	}
	s.imp.mu.Unlock()
	if j := s.imp.snapshot(); j != nil {
		s.hub.publish(map[string]any{"type": "import", "job": j})
	}
}

func (s *Server) importLog(line string) {
	s.importUpdate(func(j *ImportJob) {
		j.Log = append(j.Log, line)
		if len(j.Log) > 40 {
			j.Log = j.Log[len(j.Log)-40:]
		}
		j.Note = line
	})
}

// ---- uzantıdan gelen kareler

func (s *Server) importFrame(f inFrame) {
	s.imp.init()
	switch f.Type {
	case "history_progress":
		s.imp.mu.Lock()
		p, id := s.imp.progress, s.imp.cur
		s.imp.mu.Unlock()
		if p != nil && f.JobID == id {
			p(f.Loaded, f.Note, f.ChatName, f.I, f.N)
		}
	case "history_chunk":
		s.imp.mu.Lock()
		if f.JobID == "" || f.JobID != s.imp.cur { // beklenmeyen/eski parçalar bellekte birikmesin
			s.imp.mu.Unlock()
			return
		}
		b := s.imp.chunks[f.JobID]
		if b == nil {
			b = &chunkBuf{}
			s.imp.chunks[f.JobID] = b
		}
		if len(b.msgs)+len(f.Messages) > 200000 { // aşırı büyük okuma: sınırla
			s.imp.mu.Unlock()
			return
		}
		b.msgs = append(b.msgs, f.Messages...)
		if f.Partner != "" {
			b.partner = f.Partner
		}
		if f.Owner != "" {
			b.owner = f.Owner
		}
		s.imp.mu.Unlock()
	case "chats":
		s.hub.publish(map[string]any{"type": "chats", "chats": f.Chats})
	case "command_result", "command_error":
		r := extRes{ID: f.JobID, OK: f.Type == "command_result" && f.OK, Note: f.Note, Error: f.Error, Count: f.Loaded, Fall: f.Fallback, Data: f.Data}
		if f.Type == "command_error" && r.Error == "" {
			r.Error = f.Note
		}
		select {
		case s.imp.res <- r:
		default:
		}
	}
}

// sendCommand komutu uzantıya yollar ve sonucunu bekler. progress her ilerlemede çağrılır.
func (s *Server) sendCommand(ctx context.Context, cmd string, args map[string]any, timeout time.Duration, progress func(int, string, string, int, int)) (extRes, error) {
	s.imp.init()
	s.imp.cmdMu.Lock()
	defer s.imp.cmdMu.Unlock()
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return extRes{}, errors.New("WhatsApp sekmesi bağlı değil: uzantıyı yenile ve web.whatsapp.com'u aç")
	}
	id := newID()
	s.imp.mu.Lock()
	s.imp.cur, s.imp.progress = id, progress
	s.imp.mu.Unlock()
	defer func() {
		s.imp.mu.Lock()
		s.imp.cur, s.imp.progress = "", nil
		s.imp.mu.Unlock()
	}()
	for len(s.imp.res) > 0 { // eski sonuçları at
		<-s.imp.res
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	data := outFrame{Type: "command", JobID: id, Cmd: cmd, Args: args}
	b, _ := jsonMarshal(data)
	err := c.Write(wctx, websocket.MessageText, b)
	cancel()
	if err != nil {
		return extRes{}, fmt.Errorf("uzantıya komut gönderilemedi: %w", err)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case r := <-s.imp.res:
			if r.ID != id {
				continue
			}
			if r.Error != "" {
				return r, errors.New(r.Error)
			}
			return r, nil
		case <-ctx.Done():
			s.stopExtension()
			return extRes{ID: id}, ctx.Err()
		case <-timer.C:
			s.stopExtension()
			return extRes{ID: id}, errors.New("uzantı zamanında yanıt vermedi")
		}
	}
}

func (s *Server) stopExtension() {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c != nil {
		b, _ := jsonMarshal(outFrame{Type: "command", JobID: newID(), Cmd: "stop"})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.Write(ctx, websocket.MessageText, b)
	}
}

// ---- iş akışları

func (s *Server) normOpts(o ImportOpts) ImportOpts {
	s.mu.Lock()
	h := s.Info.History
	s.mu.Unlock()
	if o.MaxMessages <= 0 {
		o.MaxMessages = h.MaxMessages
	}
	if o.MaxSeconds <= 0 {
		o.MaxSeconds = h.MaxSeconds
	}
	if !o.All {
		o.All = h.All && o.MaxMessages == h.MaxMessages
	}
	o.MaxMessages = max(50, min(100000, o.MaxMessages))
	o.MaxSeconds = max(15, min(3600, o.MaxSeconds))
	if o.Mode != "ai" {
		o.Mode = "reader"
	}
	return o
}

func (s *Server) startImport(kind string, names []string, personID string, o ImportOpts) (*ImportJob, error) {
	s.imp.init()
	o = s.normOpts(o)
	if !s.isConnected() {
		return nil, errors.New("WhatsApp sekmesi bağlı değil: uzantıyı yenile ve web.whatsapp.com'u aç")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.imp.mu.Lock()
	if s.imp.job != nil && (s.imp.job.State == "running" || s.imp.job.State == "queued") {
		s.imp.mu.Unlock()
		cancel()
		return nil, errors.New("başka bir okuma işi çalışıyor")
	}
	s.imp.job = &ImportJob{ID: newID(), Kind: kind, State: "running", Mode: o.Mode, N: max(1, len(names)), StartedAt: time.Now().Unix(), Log: []string{}, PersonIDs: []string{}}
	s.imp.cancel = cancel
	s.imp.stopKeep.Store(false)
	s.imp.mu.Unlock()
	s.importUpdate(func(*ImportJob) {})
	go s.runImport(ctx, kind, names, personID, o)
	return s.imp.snapshot(), nil
}

func (s *Server) finishImport(state, errMsg string, fall bool) {
	s.importUpdate(func(j *ImportJob) {
		j.State, j.Error, j.CanFallbackAI = state, errMsg, fall
		if errMsg != "" {
			j.Note = errMsg
		}
	})
	s.imp.mu.Lock()
	if s.imp.cancel != nil {
		s.imp.cancel()
		s.imp.cancel = nil
	}
	s.imp.mu.Unlock()
	s.publishState()
}

func jitter(minS, maxS int) time.Duration {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64((maxS-minS)*1000+1)))
	return time.Duration(minS)*time.Second + time.Duration(n.Int64())*time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (s *Server) runImport(ctx context.Context, kind string, names []string, personID string, o ImportOpts) {
	s.mu.Lock()
	speed := s.Info.BulkSpeed
	s.mu.Unlock()
	targets := names
	switch kind {
	case "current":
		targets = []string{""}
	case "sync":
		v, err := s.Svc.People.Get(personID)
		if err != nil {
			s.finishImport("failed", err.Error(), false)
			return
		}
		targets = []string{v.Name}
	}
	for i, name := range targets {
		if s.imp.stopKeep.Load() {
			s.finishImport("done", "", false)
			return
		}
		if ctx.Err() != nil {
			s.finishImport("canceled", "", false)
			return
		}
		s.importUpdate(func(j *ImportJob) { j.I, j.Chat = i+1, name })
		if name != "" && o.Mode != "ai" {
			s.importLog(fmt.Sprintf("%q sohbeti açılıyor…", name))
			if _, err := s.sendCommand(ctx, "open_chat", map[string]any{"name": name}, 25*time.Second, nil); err != nil {
				if ctx.Err() != nil {
					s.finishImport("canceled", "", false)
					return
				}
				if kind != "bulk" {
					s.finishImport("failed", err.Error(), o.Mode == "reader")
					return
				}
				s.importLog(fmt.Sprintf("%q açılamadı, atlandı: %v", name, err))
				continue
			}
		}
		fall, err := s.scrapeOne(ctx, o, name)
		if err != nil {
			if ctx.Err() != nil {
				s.finishImport("canceled", "", false)
				return
			}
			if kind != "bulk" {
				s.finishImport("failed", err.Error(), fall)
				return
			}
			s.importLog("atlandı: " + err.Error())
		}
		if s.imp.stopKeep.Load() {
			s.importLog("durduruldu: okunanlar kaydedildi")
			s.finishImport("done", "", false)
			return
		}
		if i < len(targets)-1 {
			d := jitter(5, 15)
			if speed == "hizli" {
				d = jitter(1, 3)
			}
			s.importLog(fmt.Sprintf("sıradaki sohbet için %d sn bekleniyor (yavaş ve insan gibi)", int(d.Seconds())))
			if !sleepCtx(ctx, d) {
				s.finishImport("canceled", "", false)
				return
			}
		}
	}
	s.finishImport("done", "", false)
}

// scrapeOne açık sohbeti okur ve kişiyi oluşturur/günceller. fall: okuyucu bozuk, AI yedeği önerilir.
func (s *Server) scrapeOne(ctx context.Context, o ImportOpts, name string) (fall bool, err error) {
	if o.Mode == "ai" {
		return false, s.aiScrape(ctx, o, name)
	}
	args := map[string]any{"max_messages": o.MaxMessages, "max_seconds": o.MaxSeconds, "all": o.All, "slow": true}
	s.importLog("geçmiş okunuyor…")
	progress := func(loaded int, note, chat string, _, _ int) {
		s.importUpdate(func(j *ImportJob) {
			j.Loaded = loaded
			if chat != "" {
				j.Chat = chat
			}
			if note != "" {
				j.Note = note
			}
		})
	}
	var res extRes
	res, err = s.sendCommand(ctx, "scrape_current", args, time.Duration(o.MaxSeconds+90)*time.Second, progress)
	if err != nil {
		s.imp.mu.Lock()
		delete(s.imp.chunks, res.ID) // yarım kalan parçalar bellekte kalmasın
		s.imp.mu.Unlock()
		return res.Fall, err
	}
	return s.finalizeScrape(res)
}

func (s *Server) finalizeScrape(res extRes) (bool, error) {
	s.imp.mu.Lock()
	b := s.imp.chunks[res.ID]
	delete(s.imp.chunks, res.ID)
	s.imp.mu.Unlock()
	if b == nil {
		return true, errors.New("sohbetten mesaj okunamadı (WhatsApp arayüzü değişmiş olabilir)")
	}
	return s.finalizeMessages(b.partner, b.owner, b.msgs)
}

// finalizeMessages okunan mesajlardan kişiyi oluşturur ya da mevcut kişiye birleştirir.
func (s *Server) finalizeMessages(partner, ownerHint string, msgs []wireMsg) (bool, error) {
	if len(msgs) == 0 {
		return true, errors.New("sohbetten mesaj okunamadı (WhatsApp arayüzü değişmiş olabilir)")
	}
	b := &chunkBuf{msgs: msgs, partner: partner, owner: ownerHint}
	partner = strings.TrimSpace(b.partner)
	if partner == "" {
		return true, errors.New("sohbet adı okunamadı")
	}
	if v, ok := s.Svc.People.Find(partner); ok {
		prof, err := s.Svc.People.Profile(v.ID)
		if err != nil {
			return false, err
		}
		added, err := s.Svc.People.Merge(v.ID, wireToModel(b.msgs, prof.OwnerName, prof.PartnerName))
		if err != nil {
			return false, err
		}
		s.importUpdate(func(j *ImportJob) { j.PersonIDs = append(j.PersonIDs, v.ID); j.Loaded = len(b.msgs) })
		s.importLog(fmt.Sprintf("%s: %d yeni mesaj eklendi (toplam okunan %d)", v.Name, added, len(b.msgs)))
		return false, nil
	}
	owner := s.Svc.People.OwnerName()
	if owner == "" {
		owner = strings.TrimSpace(b.owner)
	}
	if owner == "" {
		owner = "Ben"
	}
	v, err := s.Svc.People.CreateFromMessages(partner, owner, "whatsapp", wireToModel(b.msgs, owner, partner))
	if err != nil {
		return false, err
	}
	s.importUpdate(func(j *ImportJob) { j.PersonIDs = append(j.PersonIDs, v.ID); j.Loaded = len(b.msgs) })
	s.importLog(fmt.Sprintf("%s eklendi: %d mesaj", v.Name, v.MessagesTotal))
	return false, nil
}

// wireToModel uzantı mesajlarını modele çevirir; zamanı bilinmeyenler öncekinin zamanını taşır.
func wireToModel(in []wireMsg, owner, partner string) []model.Message {
	out := make([]model.Message, 0, len(in))
	var last time.Time
	for _, m := range in {
		kind := model.KindText
		if m.Kind == "media" {
			kind = model.KindMedia
		}
		txt := strings.TrimSpace(m.Text)
		if kind == model.KindText && txt == "" {
			continue
		}
		ts := last
		if m.TS > 0 {
			ts = time.Unix(m.TS, 0)
			last = ts
		}
		if ts.IsZero() {
			continue
		}
		sender := partner
		if m.From == "me" {
			sender = owner
		}
		out = append(out, model.Message{Sender: sender, TS: ts, Kind: kind, Text: txt})
	}
	return out
}

// extDriver browseragent.Driver'ın uzantı komutlarıyla gerçeklemesi.
type extDriver struct {
	s   *Server
	ctx context.Context
}

func (d extDriver) Snapshot(ctx context.Context, withText bool) (browseragent.Snapshot, error) {
	var sn browseragent.Snapshot
	r, err := d.s.sendCommand(ctx, "ai_snapshot", map[string]any{"with_text": withText}, 30*time.Second, nil)
	if err != nil {
		return sn, err
	}
	return sn, json.Unmarshal(r.Data, &sn)
}

func (d extDriver) Act(ctx context.Context, a browseragent.Action) (string, error) {
	r, err := d.s.sendCommand(ctx, "ai_act", map[string]any{"action": a.Action, "ref": a.Ref, "text": a.Text}, 60*time.Second, nil)
	return r.Note, err
}

func (d extDriver) Log(line string) { d.s.importLog(line) }

// aiScrape yapay zekâ tarayıcı modu: ajan WhatsApp sekmesini gezer, mesajlar uzantıda toplanır.
func (s *Server) aiScrape(ctx context.Context, o ImportOpts, name string) error {
	caller, err := s.Svc.Caller()
	if err != nil {
		return err
	}
	s.importLog("AI modu başladı: yalnız kaydırır/tıklar/okur, mesaj yazmaz")
	goal := browseragent.Goal{OpenName: name, MaxMessages: o.MaxMessages, All: o.All}
	res, err := browseragent.Run(ctx, caller, extDriver{s: s, ctx: ctx}, goal)
	s.getBrain().AddCalls(max(res.Calls, 1))
	if err != nil {
		return err
	}
	cr, cerr := s.sendCommand(ctx, "ai_collect", map[string]any{"max_messages": o.MaxMessages, "all": o.All}, 60*time.Second, nil)
	if cerr == nil {
		if _, ferr := s.finalizeScrape(cr); ferr == nil {
			return nil
		}
	}
	if len(res.Extracted) > 0 { // okuyucu çalışmadı: modelin metinden çıkardığı mesajlar
		var title struct{ Title string }
		if sn, e := (extDriver{s: s}).Snapshot(ctx, false); e == nil {
			title.Title = sn.Title
		}
		_, ferr := s.finalizeMessages(title.Title, "", extractedToWire(res.Extracted))
		return ferr
	}
	if cerr != nil {
		return cerr
	}
	return errors.New("AI modu mesaj okuyamadı")
}

func extractedToWire(in []browseragent.Msg) []wireMsg {
	out := make([]wireMsg, 0, len(in))
	base := time.Now().Add(-time.Duration(len(in)) * time.Minute)
	for i, m := range in {
		ts := base.Add(time.Duration(i) * time.Minute).Unix()
		if t, err := time.Parse(time.RFC3339, m.TS); err == nil {
			ts = t.Unix()
		}
		from := "them"
		if m.From == "me" {
			from = "me"
		}
		out = append(out, wireMsg{From: from, Text: m.Text, TS: ts})
	}
	return out
}

// ---- uçlar

func (s *Server) importRoutes(a func(http.HandlerFunc) http.HandlerFunc, mux *http.ServeMux) {
	mux.HandleFunc("POST /api/import/current", a(func(w http.ResponseWriter, r *http.Request) {
		var o ImportOpts
		if !readJSON(w, r, &o) {
			return
		}
		s.respondJob(w)(s.startImport("current", nil, "", o))
	}))
	mux.HandleFunc("POST /api/import/bulk", a(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Names []string `json:"names"`
			ImportOpts
		}
		if !readJSON(w, r, &in) {
			return
		}
		var names []string
		seen := map[string]bool{}
		for _, n := range in.Names {
			if n = strings.TrimSpace(n); n != "" && !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
		if len(names) == 0 || len(names) > 10 {
			http.Error(w, "1-10 sohbet seç", http.StatusBadRequest)
			return
		}
		s.respondJob(w)(s.startImport("bulk", names, "", in.ImportOpts))
	}))
	mux.HandleFunc("POST /api/import/chats/list", a(func(w http.ResponseWriter, r *http.Request) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if _, err := s.sendCommand(ctx, "list_chats", nil, 55*time.Second, nil); err != nil {
				s.hub.publish(map[string]any{"type": "aborted", "chat_id": "", "reason": "sohbet listesi alınamadı: " + err.Error()})
			}
		}()
		if !s.isConnected() {
			http.Error(w, "WhatsApp sekmesi bağlı değil: uzantıyı yenile ve web.whatsapp.com'u aç", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	mux.HandleFunc("GET /api/import/active", a(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.imp.snapshot()) }))
	mux.HandleFunc("GET /api/import/{id}", a(func(w http.ResponseWriter, r *http.Request) {
		if j := s.imp.snapshot(); j != nil && j.ID == r.PathValue("id") {
			writeJSON(w, 200, j)
			return
		}
		http.Error(w, "iş bulunamadı", http.StatusNotFound)
	}))
	mux.HandleFunc("POST /api/import/{id}/cancel", a(func(w http.ResponseWriter, r *http.Request) {
		j := s.imp.snapshot()
		if j == nil || j.ID != r.PathValue("id") {
			http.Error(w, "iş bulunamadı", http.StatusNotFound)
			return
		}
		s.imp.mu.Lock()
		cancel := s.imp.cancel
		s.imp.mu.Unlock()
		// Önce uzantıya "dur" denir; o ana kadar okunanları gönderip bitirir ve kaydedilir.
		// Yanıt gelmezse 10 sn sonra iş zorla iptal edilir.
		s.imp.stopKeep.Store(true)
		s.importLog("durduruluyor, okunanlar kaydedilecek…")
		s.stopExtension()
		if cancel != nil {
			time.AfterFunc(10*time.Second, cancel)
		}
		writeJSON(w, 200, s.imp.snapshot())
	}))
	mux.HandleFunc("POST /api/people/{id}/sync", a(func(w http.ResponseWriter, r *http.Request) {
		var o ImportOpts
		if !readJSON(w, r, &o) {
			return
		}
		if _, err := s.Svc.People.Get(r.PathValue("id")); err != nil {
			fail(w, err)
			return
		}
		s.respondJob(w)(s.startImport("sync", nil, r.PathValue("id"), o))
	}))
	mux.HandleFunc("POST /api/people/{id}/open", a(func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.person(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if _, err := s.sendCommand(ctx, "open_chat", map[string]any{"name": v.Name}, 25*time.Second, nil); err != nil {
			code := http.StatusBadGateway
			if !s.isConnected() {
				code = http.StatusConflict
			}
			http.Error(w, err.Error(), code)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
}

func (s *Server) respondJob(w http.ResponseWriter) func(*ImportJob, error) {
	return func(j *ImportJob, err error) {
		if err != nil {
			code := http.StatusConflict
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusAccepted, j)
	}
}
