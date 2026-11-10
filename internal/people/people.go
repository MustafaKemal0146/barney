// Package people kişi deposudur: her kişi, içe aktarılmış bir sohbetten (WhatsApp/Instagram dışa
// aktarımı) ve kullanıcının o kişiye özel ayarlarından oluşur. Dosyalar yalnızca sahibine açıktır.
package people

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MustafaKemal0146/barney/internal/ingest"
	"github.com/MustafaKemal0146/barney/internal/ingest/zipfs"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

// Guards otomatik gönderim korumalarıdır (kişi bazında).
type Guards struct {
	FirstNApprovals int    `json:"first_n_approvals"`
	QuietOn         bool   `json:"quiet_on"`
	QuietStart      string `json:"quiet_start"`  // "01:00"
	QuietEnd        string `json:"quiet_end"`    // "08:00"
	MaxPerHour      int    `json:"max_per_hour"` // 0 = sınırsız
	MaxPerDay       int    `json:"max_per_day"`  // 0 = sınırsız
	StopSensitive   bool   `json:"stop_sensitive"`
}

// DefaultGuards güvenli başlangıç değerleridir.
var DefaultGuards = Guards{FirstNApprovals: 3, QuietOn: true, QuietStart: "01:00", QuietEnd: "08:00", MaxPerHour: 6, MaxPerDay: 30, StopSensitive: true}

// Meta diske yazılan kişi ayarlarıdır.
type Meta struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`  // karşı tarafın adı (dışa aktarımdaki)
	Owner         string     `json:"owner"` // sahibin dışa aktarımdaki adı; boşsa bekliyor
	Candidates    []string   `json:"candidates,omitempty"`
	Goal          string     `json:"goal"`
	Notes         string     `json:"notes"`
	Stage         string     `json:"stage"`
	Tone          reply.Tone `json:"tone"`
	Watch         bool       `json:"watch"`
	AutoSend      bool       `json:"auto_send"`
	FullAuto      bool       `json:"full_auto"` // tam otonom: onay/sessiz saat yok, hemen cevaplar
	Council       bool       `json:"council"`
	Guards        Guards     `json:"guards"`
	SilenceDays   int        `json:"silence_days"`
	ApprovalsDone int        `json:"approvals_done"`
	Aliases       []string   `json:"aliases,omitempty"`
	CreatedAt     int64      `json:"created_at"`
}

// CardSummary arayüzde gösterilen kart özetidir (yüzdeler 0-100).
type CardSummary struct {
	Messages         int      `json:"messages"`
	Burst            Burst    `json:"burst"`
	WordsMedian      float64  `json:"words_median"`
	WordsP90         float64  `json:"words_p90"`
	LowerStartPct    float64  `json:"lower_start_pct"`
	NoCapsPct        float64  `json:"no_caps_pct"`
	PunctNonePct     float64  `json:"punct_none_pct"`
	EmojiPct         float64  `json:"emoji_pct"`
	TopEmoji         []string `json:"top_emoji"`
	LatencyMedianSec float64  `json:"latency_median_sec"`
	Signature        []string `json:"signature"`
}

// Burst patlama dağılımı özeti.
type Burst struct {
	Single   float64 `json:"single"`
	TwoThree float64 `json:"two_three"`
	FourPlus float64 `json:"four_plus"`
	Median   float64 `json:"median"`
	Max      int     `json:"max"`
}

// View API'nin döndürdüğü kişidir.
type View struct {
	Meta
	MessagesTotal   int            `json:"messages_total"`
	MessagesOwner   int            `json:"messages_owner"`
	MessagesPartner int            `json:"messages_partner"`
	Confidence      string         `json:"confidence"`
	LastMessageTS   int64          `json:"last_message_ts"`
	Card            map[string]any `json:"card"`
}

func summarize(c style.Card) CardSummary {
	p := func(x float64) float64 { return float64(int(x*1000+0.5)) / 10 }
	top := c.Emoji.Top
	if top == nil {
		top = []string{}
	}
	sig := make([]string, 0, len(c.Signature))
	for _, w := range c.Signature {
		sig = append(sig, w.Word)
	}
	return CardSummary{
		Messages:    c.Messages,
		Burst:       Burst{Single: p(c.Burst.P1), TwoThree: p(c.Burst.P2to3), FourPlus: p(c.Burst.P4plus), Median: c.Burst.MedianSize, Max: c.Burst.MaxSize},
		WordsMedian: c.Length.WordsMedian, WordsP90: c.Length.WordsP90,
		LowerStartPct: p(c.Case.LowerStartRatio), NoCapsPct: p(c.Case.AllLowerRatio), PunctNonePct: p(c.Punct.EndNoneRatio),
		EmojiPct: p(c.Emoji.MessageRatio), TopEmoji: top, LatencyMedianSec: c.Latency.MedianSec, Signature: sig,
	}
}

type entry struct {
	Meta
	imp *model.Import // diskten yüklenen ham içe aktarma
}

// cardCache kart özetleri: mesajlar değişmedikçe yeniden hesaplanmaz (List her durum sorgusunda çağrılır).
type cardCache struct {
	gver                    int
	conf                    string
	owner, partner          CardSummary
	total, nOwner, nPartner int
	last                    int64
}

// Store kişi deposu.
type Store struct {
	dir   string
	mu    sync.Mutex
	m     map[string]*entry
	gver  int // herhangi bir kişinin mesajları değişince artar
	cache map[string]cardCache
}

// Open dizini açar (yoksa oluşturur) ve kişileri yükler.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, m: map[string]*entry{}, cache: map[string]cardCache{}}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range ents {
		if !d.IsDir() {
			continue
		}
		e, err := s.load(d.Name())
		if err != nil {
			continue // bozuk kişi klasörü diğerlerini engellemesin
		}
		s.m[e.ID] = e
	}
	return s, nil
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id) }

func (s *Store) load(id string) (*entry, error) {
	var e entry
	b, err := os.ReadFile(filepath.Join(s.path(id), "meta.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &e.Meta); err != nil {
		return nil, err
	}
	b, err = os.ReadFile(filepath.Join(s.path(id), "import.json"))
	if err != nil {
		return nil, err
	}
	e.imp = &model.Import{}
	if err := json.Unmarshal(b, e.imp); err != nil {
		return nil, err
	}
	return &e, nil
}

func writeAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) save(e *entry) error {
	if err := os.MkdirAll(s.path(e.ID), 0o700); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.path(e.ID), "meta.json"), e.Meta); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.path(e.ID), "import.json"), e.imp)
}

func newID(name string) string {
	var r [3]byte
	_, _ = rand.Read(r[:])
	slug := model.Norm(name)
	if len(slug) > 12 {
		slug = slug[:12]
	}
	if slug == "" {
		slug = "kisi"
	}
	return fmt.Sprintf("%s-%s", slug, hex.EncodeToString(r[:]))
}

// ErrNeedsOwner sahip adı belirsiz; Candidates ile tekrar denenmeli.
type ErrNeedsOwner struct {
	ID         string
	Candidates []string
}

func (e *ErrNeedsOwner) Error() string { return "sohbetin sahibi belirsiz" }

// Import bir dışa aktarım dosyasından (zip/txt/dizin) kişi oluşturur. owner boşsa ve sahip
// güvenle bulunamazsa ErrNeedsOwner döner (kişi "sahip bekliyor" olarak kaydedilir).
func (s *Store) Import(path, owner string) (*View, error) {
	src, err := zipfs.Open(path)
	if err != nil {
		return nil, err
	}
	imp, err := ingest.Parse(src, time.Local)
	src.Close()
	if err != nil {
		return nil, err
	}
	var name string
	for _, c := range imp.Conversations {
		if !c.IsGroup {
			name = c.Title
			break
		}
	}
	if name == "" {
		return nil, errors.New("birebir sohbet bulunamadı (grup sohbetleri desteklenmiyor)")
	}
	e := &entry{Meta: Meta{
		ID: newID(name), Name: name, Tone: reply.DefaultTone, Guards: DefaultGuards, Council: false,
		SilenceDays: 3, CreatedAt: time.Now().Unix(),
	}, imp: imp}
	switch {
	case owner != "":
		e.Owner = owner
	case imp.OwnerName != "" && !(imp.OwnerGuessed && len(imp.Candidates) > 1):
		e.Owner = imp.OwnerName
	default:
		e.Candidates = imp.Candidates
	}
	if e.Owner != "" {
		e.Name = partnerOf(imp, e.Owner, name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.save(e); err != nil {
		return nil, err
	}
	s.m[e.ID] = e
	s.gver++
	if e.Owner == "" {
		return nil, &ErrNeedsOwner{ID: e.ID, Candidates: e.Candidates}
	}
	v := s.viewLocked(e)
	return &v, nil
}

// partnerOf sahip bilindiğinde karşı tarafın adını sohbet katılımcılarından çıkarır.
func partnerOf(imp *model.Import, owner, fallback string) string {
	for _, c := range imp.Conversations {
		if c.IsGroup {
			continue
		}
		for _, p := range c.Participants {
			if model.Norm(p) != model.Norm(owner) && p != "" {
				return p
			}
		}
		for _, m := range c.Messages {
			if m.Sender != "" && model.Norm(m.Sender) != model.Norm(owner) {
				return m.Sender
			}
		}
	}
	return fallback
}

// SetOwner bekleyen kişinin sahip adını belirler.
func (s *Store) SetOwner(id, owner string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("sahip adı boş")
	}
	e.Owner, e.Candidates = owner, nil
	e.Name = partnerOf(e.imp, owner, e.Name)
	s.gver++
	if err := s.save(e); err != nil {
		return nil, err
	}
	v := s.viewLocked(e)
	return &v, nil
}

// ErrNotFound kişi yok.
var ErrNotFound = errors.New("kişi bulunamadı")

// Patch alanları güncelleyen değişiklik isteğidir (nil = dokunma).
type Patch struct {
	Goal, Notes, Stage *string
	Tone               *reply.Tone
	Watch, AutoSend    *bool
	FullAuto           *bool
	Council            *bool
	Guards             *Guards
	SilenceDays        *int
	Aliases            *[]string
}

// Update kişiyi günceller.
func (s *Store) Update(id string, p Patch) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	if p.Goal != nil {
		e.Goal = *p.Goal
	}
	if p.Notes != nil {
		e.Notes = *p.Notes
	}
	if p.Stage != nil {
		e.Stage = *p.Stage
	}
	if p.Tone != nil {
		e.Tone = p.Tone.Clamp()
	}
	if p.Watch != nil {
		e.Watch = *p.Watch
	}
	if p.FullAuto != nil {
		e.FullAuto = *p.FullAuto
		if e.FullAuto {
			e.AutoSend, e.Watch = true, true
		}
	}
	if p.AutoSend != nil {
		e.AutoSend = *p.AutoSend
		if !e.AutoSend {
			e.FullAuto = false
		}
	}
	if p.Council != nil {
		e.Council = *p.Council
	}
	if p.Guards != nil {
		g := *p.Guards
		g.FirstNApprovals = max(0, min(50, g.FirstNApprovals))
		// 0 = sınırsız
		g.MaxPerHour = max(0, min(1000, g.MaxPerHour))
		g.MaxPerDay = max(0, min(10000, g.MaxPerDay))
		e.Guards = g
	}
	if p.SilenceDays != nil {
		e.SilenceDays = max(0, *p.SilenceDays)
	}
	if p.Aliases != nil {
		e.Aliases = *p.Aliases
	}
	if err := writeAtomic(filepath.Join(s.path(id), "meta.json"), e.Meta); err != nil {
		return nil, err
	}
	v := s.viewLocked(e)
	return &v, nil
}

// AddApproval ilk-N onay sayacını artırır.
func (s *Store) AddApproval(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[id]; ok {
		e.ApprovalsDone++
		_ = writeAtomic(filepath.Join(s.path(id), "meta.json"), e.Meta)
	}
}

// Delete kişiyi ve tüm verisini siler.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return ErrNotFound
	}
	delete(s.m, id)
	delete(s.cache, id)
	s.gver++
	return os.RemoveAll(s.path(id))
}

// Dir kişinin ek verileri (hafıza, geçmiş, sohbet) için klasörünü verir.
func (s *Store) Dir(id string) string { return s.path(id) }

func (e *entry) conv() *model.Conversation {
	for i := range e.imp.Conversations {
		if !e.imp.Conversations[i].IsGroup {
			return &e.imp.Conversations[i]
		}
	}
	return nil
}

func (e *entry) ownerCard() style.Card {
	if c := e.conv(); c != nil && e.Owner != "" {
		return style.Extract(e.Owner, []model.Conversation{*c}, style.ByName(e.Owner))
	}
	return style.Card{}
}

func (e *entry) partnerCard() style.Card {
	if c := e.conv(); c != nil {
		return style.Extract(e.Name, []model.Conversation{*c}, style.ByName(e.Name))
	}
	return style.Card{}
}

// Messages kişiyle olan sohbetin mesajlarıdır (eskiden yeniye).
func (s *Store) Messages(id string) ([]model.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := e.conv()
	if c == nil {
		return nil, nil
	}
	msgs := append([]model.Message(nil), c.Messages...)
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].TS.Before(msgs[j].TS) })
	return msgs, nil
}

func (s *Store) viewLocked(e *entry) View {
	v := View{Meta: e.Meta, Confidence: style.ConfLow, Card: map[string]any{}}
	cc, ok := s.cache[e.ID]
	if !ok || cc.gver != s.gver {
		cc = cardCache{gver: s.gver, conf: style.ConfLow}
		if c := e.conv(); c != nil {
			for _, m := range c.Messages {
				if m.Kind != model.KindText {
					continue
				}
				cc.total++
				if e.Owner != "" && model.Norm(m.Sender) == model.Norm(e.Owner) {
					cc.nOwner++
				} else {
					cc.nPartner++
				}
				if ts := m.TS.Unix(); ts > cc.last {
					cc.last = ts
				}
			}
		}
		if e.Owner != "" {
			oc := s.ownerVoiceLocked(e)
			cc.conf, cc.owner, cc.partner = oc.Confidence, summarize(oc), summarize(e.partnerCard())
		}
		s.cache[e.ID] = cc
	}
	v.MessagesTotal, v.MessagesOwner, v.MessagesPartner, v.LastMessageTS, v.Confidence = cc.total, cc.nOwner, cc.nPartner, cc.last, cc.conf
	if e.Owner != "" {
		v.Card["owner"], v.Card["partner"] = cc.owner, cc.partner
	}
	return v
}

// ownerVoiceLocked sahibin sesini, aynı sahip adına sahip TÜM kişilerin sohbetlerinden birlikte çıkarır:
// ne kadar çok sohbet o kadar güvenilir kart.
func (s *Store) ownerVoiceLocked(e *entry) style.Card {
	var convs []model.Conversation
	for _, o := range s.m {
		if model.Norm(o.Owner) == model.Norm(e.Owner) {
			if c := o.conv(); c != nil {
				convs = append(convs, *c)
			}
		}
	}
	return style.Extract(e.Owner, convs, style.ByName(e.Owner))
}

// List tüm kişiler (en yeni yazışma önce).
func (s *Store) List() []View {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]View, 0, len(s.m))
	for _, e := range s.m {
		out = append(out, s.viewLocked(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastMessageTS > out[j].LastMessageTS })
	return out
}

// Get tek kişi.
func (s *Store) Get(id string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	v := s.viewLocked(e)
	return &v, nil
}

// Profile üretim için sahibin birleşik kartı + karşı tarafın kartı + isimler.
type Profile struct {
	OwnerName, PartnerName string
	Owner, Partner         style.Card
}

// Profile kişinin üretim profilini verir; sahip bekliyorsa hata.
func (s *Store) Profile(id string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	if e.Owner == "" {
		return Profile{}, errors.New("sohbetin sahibi seçilmedi")
	}
	return Profile{OwnerName: e.Owner, PartnerName: e.Name, Owner: s.ownerVoiceLocked(e), Partner: e.partnerCard()}, nil
}

// Find açık sohbetin başlığına (ör. WhatsApp kişi adı) uyan kişiyi bulur. Önce ad/takma ad tam eşleşir
// (Türkçe karakterden ve emojiden bağımsız); yoksa kelime kümesi biri ötekinin alt kümesiyse ve aday
// TEK ise eşleşir ("Ayşe" ≠ "Ayşegül"; belirsizlikte eşleşme yok — yanlış kişiye mesaj yazmaktan iyidir).
func (s *Store) Find(title string) (*View, bool) {
	t := fold(title)
	if t == "" {
		return nil, false
	}
	toks := tokens(title)
	s.mu.Lock()
	defer s.mu.Unlock()
	var fuzzy []*entry
	for _, e := range s.m {
		match, near := false, false
		for _, n := range append([]string{e.Name}, e.Aliases...) {
			if k := fold(n); k != "" && k == t {
				match = true
				break
			}
			if subset(tokens(n), toks) || subset(toks, tokens(n)) {
				near = true
			}
		}
		if match {
			v := s.viewLocked(e)
			return &v, true
		}
		if near {
			fuzzy = append(fuzzy, e)
		}
	}
	if len(fuzzy) == 1 {
		v := s.viewLocked(fuzzy[0])
		return &v, true
	}
	return nil, false
}

// tokens adı kelimelerine ayırır (katlanmış, ≥3 harfli).
func tokens(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if f := fold(w); len(f) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

// subset a boş değilse ve her elemanı b'de varsa true.
func subset(a, b []string) bool {
	if len(a) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	for _, x := range a {
		if !set[x] {
			return false
		}
	}
	return true
}

var asciiFold = strings.NewReplacer("ş", "s", "ı", "i", "ğ", "g", "ü", "u", "ö", "o", "ç", "c", "â", "a", "î", "i", "û", "u")

// fold adı Türkçe karakterlerden bağımsız karşılaştırmak için sadeleştirir ("Ayşe" = "ayse").
func fold(s string) string { return asciiFold.Replace(model.Norm(s)) }

// Count kişi sayısı (kart hesaplamadan).
func (s *Store) Count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.m) }

// OwnerNames kayıtlı kişilerde kullanılan sahip adı (varsa ilki); aynı kullanıcıyı yeni kişilerde yeniden kullanmak için.
func (s *Store) OwnerName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.m {
		if e.Owner != "" {
			return e.Owner
		}
	}
	return ""
}

// CreateFromMessages tarayıcıdan okunan mesajlarla yeni kişi oluşturur.
func (s *Store) CreateFromMessages(name, owner, platform string, msgs []model.Message) (*View, error) {
	name, owner = strings.TrimSpace(name), strings.TrimSpace(owner)
	if name == "" || owner == "" {
		return nil, errors.New("kişi ya da sahip adı boş")
	}
	if len(msgs) == 0 {
		return nil, errors.New("okunan mesaj yok")
	}
	if platform == "" {
		platform = "whatsapp"
	}
	conv := model.Conversation{Title: name, Platform: platform, Participants: []string{owner, name}, Messages: dedupe(nil, msgs)}
	imp := &model.Import{Platform: platform, Label: name, OwnerName: owner, Conversations: []model.Conversation{conv}, Stats: map[string]int{"messages": len(conv.Messages)}}
	e := &entry{Meta: Meta{ID: newID(name), Name: name, Owner: owner, Tone: reply.DefaultTone, Guards: DefaultGuards, SilenceDays: 3, CreatedAt: time.Now().Unix()}, imp: imp}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.save(e); err != nil {
		return nil, err
	}
	s.m[e.ID] = e
	s.gver++
	v := s.viewLocked(e)
	return &v, nil
}

// mergeKey tekilleştirme anahtarı: dakikaya yuvarlanmış zaman + gönderen + metin (zip'in saniyeli kayıtlarıyla da çakışır).
func mergeKey(m model.Message) string {
	return fmt.Sprintf("%d|%s|%s|%s", m.TS.Unix()/60, model.Norm(m.Sender), m.Kind, model.Norm(m.Text))
}

func dedupe(existing, add []model.Message) []model.Message {
	seen := make(map[string]bool, len(existing)+len(add))
	out := make([]model.Message, 0, len(existing)+len(add))
	for _, m := range existing {
		seen[mergeKey(m)] = true
		out = append(out, m)
	}
	for _, m := range add {
		k := mergeKey(m)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out
}

// Merge yeni mesajları kişinin geçmişine ekler (tekilleştirir); eklenen sayısını döner.
func (s *Store) Merge(id string, msgs []model.Message) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return 0, ErrNotFound
	}
	c := e.conv()
	if c == nil {
		return 0, errors.New("sohbet yok")
	}
	merged := dedupe(c.Messages, msgs)
	added := len(merged) - len(c.Messages)
	if added <= 0 {
		return 0, nil
	}
	c.Messages = merged
	s.gver++
	return added, writeAtomic(filepath.Join(s.path(id), "import.json"), e.imp)
}

// Quiet g.QuietOn iken now'un sessiz saat aralığında olup olmadığını söyler ("01:00"-"08:00" gece yarısını aşabilir).
func (g Guards) Quiet(now time.Time) bool {
	if !g.QuietOn {
		return false
	}
	parse := func(s string) (int, bool) {
		t, err := time.Parse("15:04", s)
		return t.Hour()*60 + t.Minute(), err == nil
	}
	a, ok1 := parse(g.QuietStart)
	b, ok2 := parse(g.QuietEnd)
	if !ok1 || !ok2 || a == b {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if a < b {
		return m >= a && m < b
	}
	return m >= a || m < b
}
