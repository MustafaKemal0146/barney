// Package chat kullanıcının Barney ile konuştuğu iki modu yürütür:
//   - Danışman: kişiyle ilgili serbest sohbet ("ona ne yazayım?"); model kişinin gerçek sohbetini,
//     hafızasını ve kullanıcının sesini bilir.
//   - Rol provası: model kişiyi kendi yazma biçimiyle canlandırır; kullanıcı kendi gibi yazar.
//     Üretim, reply.Generate'in rolleri tersine çevrilmiş hâlidir; kişinin kart kuralları kodla denetlenir.
package chat

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MustafaKemal0146/barney/internal/mask"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

// Msg konuşmadaki bir mesaj.
type Msg struct {
	ID    string       `json:"id"`
	Role  string       `json:"role"` // user | assistant
	Text  string       `json:"text"`
	TS    int64        `json:"ts"`
	Steps []reply.Step `json:"steps,omitempty"` // rol provasında asistanın art arda mesajları
}

// Modlar.
const (
	ModeAdvisor  = "advisor"
	ModeRoleplay = "roleplay"
)

// ValidMode modu doğrular.
func ValidMode(m string) bool { return m == ModeAdvisor || m == ModeRoleplay }

func file(dir, mode string) string { return filepath.Join(dir, "chat-"+mode+".jsonl") }

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewMsg yeni mesaj üretir.
func NewMsg(role, text string, steps []reply.Step) Msg {
	return Msg{ID: newID(), Role: role, Text: text, TS: time.Now().Unix(), Steps: steps}
}

// Load konuşmayı okur (dosya yoksa boş).
func Load(dir, mode string) ([]Msg, error) {
	f, err := os.Open(file(dir, mode))
	if errors.Is(err, os.ErrNotExist) {
		return []Msg{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Msg{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m Msg
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Role != "" {
			out = append(out, m)
		}
	}
	return out, sc.Err()
}

// Append mesajları sona ekler.
func Append(dir, mode string, ms ...Msg) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(file(dir, mode), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, m := range ms {
		b, _ := json.Marshal(m)
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// Clear konuşmayı siler.
func Clear(dir, mode string) error {
	err := os.Remove(file(dir, mode))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Ctx bir turda modele verilen kişi bağlamıdır.
type Ctx struct {
	OwnerName, PartnerName string
	Owner, Partner         style.Card
	Real                   []model.Message // kişiyle gerçek sohbet (eskiden yeniye)
	Tone                   reply.Tone
	Goal, Stage, Notes     string
	Facts                  []string
	Examples               []reply.Example
}

const dialogMax = 20

// Advisor kullanıcının sorusuna danışman olarak cevap verir.
func Advisor(ctx context.Context, c reply.Caller, in Ctx, dialog []Msg, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("mesaj boş")
	}
	m := mask.New()
	rq := reply.Request{Owner: in.Owner, Partner: in.Partner, OwnerName: in.OwnerName, PartnerName: in.PartnerName,
		Goal: in.Goal, Tone: in.Tone, Facts: in.Facts, Stage: in.Stage, Examples: in.Examples, History: in.Real}
	var sys strings.Builder
	fmt.Fprintf(&sys, "Sen Barney'sin: %s'in kişisel yazışma danışmanısın. %s ile olan yazışması konusunda ona yardım edersin: durumu yorumlar, ne yazacağını önerir, istenirse mesajı YAZACAĞI GİBİ (onun yazma biçimiyle) yazarsın.\n\n", in.OwnerName, in.PartnerName)
	sys.WriteString("KURALLAR\n- Kısa, net ve uygulanabilir konuş; gereksiz uzatma. Türkçe yaz.\n- Mesaj önerirken önerilen mesajı ayrı satırda, tırnak içinde ver; gerekçeyi bir cümleyle söyle.\n")
	sys.WriteString(reply.SharedRules)
	sys.WriteString("- Bilmediğin şeyi uydurma; sohbette olmayan bir olgu varsa sor.\n\n")
	sys.WriteString(reply.StyleBlock(rq, m))
	if g := strings.TrimSpace(in.Notes); g != "" {
		fmt.Fprintf(&sys, "\nKULLANICININ NOTLARI: %s\n", m.Apply(g))
	}
	var usr strings.Builder
	usr.WriteString(reply.ConversationBlock(reply.Request{History: in.Real, OwnerName: in.OwnerName}, m))
	usr.WriteString("\n\n")
	d := dialog
	if len(d) > dialogMax {
		d = d[len(d)-dialogMax:]
	}
	if len(d) > 0 {
		usr.WriteString("ÖNCEKİ DANIŞMA KONUŞMASI\n")
		for _, x := range d {
			who := "Kullanıcı"
			if x.Role == "assistant" {
				who = "Barney"
			}
			fmt.Fprintf(&usr, "%s: %s\n", who, m.Apply(x.Text))
		}
	}
	fmt.Fprintf(&usr, "\nKullanıcı: %s\nBarney:", m.Apply(text))
	out, _, err := c.Complete(ctx, sys.String(), usr.String(), 700)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(m.Unmask(out)), nil
}

// Roleplay kişiyi canlandırarak kullanıcının mesajına cevap verir.
func Roleplay(ctx context.Context, c reply.Caller, in Ctx, dialog []Msg, text string, timing reply.Timing) ([]reply.Step, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("mesaj boş")
	}
	if in.Partner.Messages < 20 {
		return nil, fmt.Errorf("%s'in yeterli mesajı yok (%d): rol provası için en az ~20 metin mesajı gerekir", in.PartnerName, in.Partner.Messages)
	}
	// Geçmişi: gerçek sohbetin son birkaç mesajı + prova turları. Kullanıcı = sahip, asistan = kişi.
	var h []model.Message
	real := in.Real
	if len(real) > 10 {
		real = real[len(real)-10:]
	}
	h = append(h, real...)
	base := time.Now().Add(-time.Hour)
	add := func(sender, t string) {
		base = base.Add(30 * time.Second)
		h = append(h, model.Message{Sender: sender, TS: base, Kind: model.KindText, Text: t})
	}
	d := dialog
	if len(d) > dialogMax {
		d = d[len(d)-dialogMax:]
	}
	for _, x := range d {
		if x.Role == "user" {
			add(in.OwnerName, x.Text)
			continue
		}
		if len(x.Steps) > 0 {
			for _, s := range x.Steps {
				add(in.PartnerName, s.Text)
			}
		} else {
			add(in.PartnerName, x.Text)
		}
	}
	add(in.OwnerName, text)
	req := reply.Request{
		Owner: in.Partner, Partner: in.Owner, OwnerName: in.PartnerName, PartnerName: in.OwnerName,
		History: h, Facts: in.Facts, Seed: time.Now().UnixNano(), Timing: timing,
		Goal: "Bu bir prova: sen " + in.PartnerName + " olarak, kendi doğal tarzınla ve karakterinle cevap ver; kullanıcıyı kolaya kaçırma, gerçekçi ol.",
	}
	res, err := reply.Generate(ctx, c, req)
	if err != nil {
		return nil, err
	}
	return res.Steps, nil
}
