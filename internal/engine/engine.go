// Package engine, Gözcü'nün (watch) cevap üretici işlevini kurar: profil (stil kartları) + model
// çağırıcılar → tek çağrılık reply ya da konsey.
package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MustafaKemal0146/barney/internal/council"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
	"github.com/MustafaKemal0146/barney/internal/watch"
)

// Profile içe aktarılan sohbetlerden çıkarılan kartlardır.
type Profile struct {
	OwnerName string
	Owner     style.Card
	Partners  map[string]style.Card // model.Norm(ad) → kart
}

// NewProfile bir içe aktarmadan profil çıkarır.
func NewProfile(imp *model.Import) Profile {
	owner, partners := style.FromImport(imp)
	p := Profile{OwnerName: imp.OwnerName, Owner: owner, Partners: map[string]style.Card{}}
	for _, c := range partners {
		p.Partners[model.Norm(c.Who)] = c
	}
	return p
}

// Options model ve mod ayarları.
type Options struct {
	Synth   reply.Caller // tek çağrı modunda ve konsey sentezinde kullanılır
	Fast    reply.Caller // konsey persona/Vicdan (boşsa Synth)
	Council bool
	Timing  reply.Timing
}

// SuggestInput kişi bazlı bir öneri isteğidir.
type SuggestInput struct {
	Owner, Partner         style.Card
	OwnerName, PartnerName string
	History                []model.Message
	Goal, Stage            string
	Opener                 bool
	Tone                   reply.Tone
	Facts                  []string
	Examples               []reply.Example
}

// Suggest tek çağrı ya da konseyle aday cevaplar üretir ve hassas konuyu işaretler.
func Suggest(ctx context.Context, o Options, in SuggestInput) (watch.Decision, error) {
	if in.Owner.Messages == 0 {
		return watch.Decision{}, errors.New("stil kartı boş: önce sohbet içe aktar")
	}
	if o.Synth == nil {
		return watch.Decision{}, errors.New("model yok")
	}
	if o.Timing.MaxFirst == 0 {
		o.Timing = reply.DefaultTiming
	}
	req := reply.Request{
		Owner: in.Owner, Partner: in.Partner, History: in.History, OwnerName: in.OwnerName, PartnerName: in.PartnerName,
		Goal: in.Goal, Opener: in.Opener, Tone: in.Tone, Facts: in.Facts, Stage: in.Stage, Examples: in.Examples,
		Seed: time.Now().UnixNano(), Timing: o.Timing,
	}
	lastIn := lastPartnerText(in.History, in.OwnerName)
	if !o.Council {
		res, err := reply.Generate(ctx, o.Synth, req)
		if err != nil {
			return watch.Decision{Calls: res.Attempts}, err
		}
		sens := res.Sensitive || watch.IsSensitive(lastIn) || watch.IsSensitive(stepsText(res.Steps))
		return watch.Decision{
			Recommended: 0, Calls: res.Attempts, Note: res.Note,
			Cands: []watch.Cand{{Tur: "tek", Steps: res.Steps, Confident: res.Confident, Sensitive: sens, Violations: res.Violations}},
		}, nil
	}
	fast := o.Fast
	if fast == nil {
		fast = o.Synth
	}
	res, err := council.Run(ctx, council.Options{Fast: fast, Synth: o.Synth}, req)
	if err != nil {
		return watch.Decision{Calls: res.Calls}, err
	}
	d := watch.Decision{Recommended: res.Recommended, Calls: res.Calls, Note: res.Note}
	incSens := watch.IsSensitive(lastIn)
	for _, cd := range res.Candidates {
		sens := incSens || watch.IsSensitive(stepsText(cd.Steps))
		d.Cands = append(d.Cands, watch.Cand{Tur: cd.Tur, Steps: cd.Steps, Confident: cd.Confident, Blocked: cd.Blocked, Sensitive: sens, Violations: cd.Violations})
	}
	return d, nil
}

func lastPartnerText(h []model.Message, owner string) string {
	for i := len(h) - 1; i >= 0; i-- {
		if model.Norm(h[i].Sender) != model.Norm(owner) {
			return h[i].Text
		}
	}
	return ""
}

func stepsText(st []reply.Step) string {
	var b []string
	for _, s := range st {
		b = append(b, s.Text)
	}
	return strings.Join(b, " ")
}

// NewDecider watch.Decider döndürür (tek profilli kullanım).
func NewDecider(p Profile, o Options) (watch.Decider, error) {
	if p.Owner.Messages == 0 {
		return nil, errors.New("stil kartı boş: önce sohbet dışa aktarımını --profile ile ver")
	}
	if o.Synth == nil {
		return nil, errors.New("model yok")
	}
	return func(ctx context.Context, c watch.Chat) (watch.Decision, error) {
		return Suggest(ctx, o, SuggestInput{
			Owner: p.Owner, Partner: p.Partners[model.Norm(c.Partner)], History: c.History,
			OwnerName: p.OwnerName, PartnerName: c.Partner,
		})
	}, nil
}
