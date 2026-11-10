package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MustafaKemal0146/barney/internal/council"
	"github.com/MustafaKemal0146/barney/internal/ingest"
	"github.com/MustafaKemal0146/barney/internal/ingest/zipfs"
	"github.com/MustafaKemal0146/barney/internal/llm"
	"github.com/MustafaKemal0146/barney/internal/model"
	"github.com/MustafaKemal0146/barney/internal/reply"
	"github.com/MustafaKemal0146/barney/internal/style"
)

type provider struct{ id, baseURL, model, keyEnv string }

var providers = map[string]provider{
	"deepseek":  {"deepseek", "https://api.deepseek.com", "deepseek-flash", "DEEPSEEK_API_KEY"},
	"anthropic": {"anthropic", "https://api.anthropic.com", "claude-haiku-5-5", "ANTHROPIC_API_KEY"},
	"openai":    {"openai", "https://api.openai.com/v1", "gpt-4o-mini", "OPENAI_API_KEY"},
}

// runReply: barney reply <sohbet> --owner Ad --partner Ad [--goal ...] [--dry]
// --dry modele gidecek maskelenmiş istemi yazar, hiçbir ağ çağrısı yapmaz.
func runReply(args []string) error {
	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	owner := fs.String("owner", "", "sohbetin sahibi (sen)")
	partner := fs.String("partner", "", "karşı taraf (birden çok sohbet varsa gerekir)")
	goal := fs.String("goal", "", "isteğe bağlı yönlendirme")
	prov := fs.String("provider", "deepseek", "deepseek | anthropic | openai")
	mdl := fs.String("model", "", "model adı (boşsa sağlayıcı varsayılanı)")
	useCouncil := fs.Bool("council", false, "konsey: 3 persona + sentez + Vicdan, 3 aday (daha pahalı, ~5 çağrı)")
	fast := fs.String("fast-model", "", "konseyde persona/Vicdan için ucuz model (boşsa --model)")
	dry := fs.Bool("dry", false, "çağrı yapma, modele gidecek istemi göster")
	seed := fs.Int64("seed", time.Now().UnixNano(), "rastgelelik tohumu")
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	if len(pos) != 1 || *owner == "" {
		return fmt.Errorf("kullanım: barney reply <sohbet> --owner \"Ad\" [--partner \"Ad\"] [--goal ...] [--dry]")
	}
	src, err := zipfs.Open(pos[0])
	if err != nil {
		return err
	}
	defer src.Close()
	imp, err := ingest.Parse(src, time.Local)
	if err != nil {
		return err
	}
	imp.OwnerName = *owner
	ownerCard, partners := style.FromImport(imp)

	conv, pcard, err := pickConversation(imp, *partner, partners)
	if err != nil {
		return err
	}
	hist := conv.Messages
	if n := len(hist); n == 0 || model.Norm(hist[n-1].Sender) == model.Norm(*owner) {
		return fmt.Errorf("sohbetin son mesajı sana ait; cevaplanacak karşı taraf mesajı yok")
	}
	req := reply.Request{
		Owner: ownerCard, Partner: pcard, History: hist, OwnerName: *owner, PartnerName: pcard.Who,
		Goal: *goal, Seed: *seed, Timing: reply.DefaultTiming,
	}
	if *dry && *useCouncil {
		fmt.Printf("=== KONSEY (kuru çalışma) ===\npersona çağrıları: %d paralel + 1 sentez + 1 Vicdan = %d çağrı (stil ihlalinde en çok +%d sentez)\n", len(council.Personas), len(council.Personas)+2, council.MaxRetries)
		fmt.Printf("\n--- Okuyucu istemi (diğerleri aynı bağlamı alır) ---\n%s\n", council.PersonaPromptPreview(req, council.Personas[0]))
		fmt.Fprintln(os.Stderr, "\n(--dry: ağ çağrısı yapılmadı)")
		return nil
	}
	if *dry {
		sys, usr := reply.Prompts(req)
		fmt.Printf("=== SİSTEM ===\n%s\n=== KULLANICI ===\n%s\n", sys, usr)
		fmt.Fprintln(os.Stderr, "\n(--dry: ağ çağrısı yapılmadı; yukarıdaki metin maskelenmiş hâliyle modele giderdi)")
		return nil
	}
	p, ok := providers[*prov]
	if !ok {
		return fmt.Errorf("bilinmeyen sağlayıcı %q", *prov)
	}
	key := os.Getenv(p.keyEnv)
	if key == "" {
		return fmt.Errorf("%s ortam değişkeni boş", p.keyEnv)
	}
	if *mdl != "" {
		p.model = *mdl
	}
	tr := llm.NewTransport(p.id, p.baseURL, key)
	caller := reply.LLMCaller{Transport: tr, Model: p.model, Temperature: 0.8}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if *useCouncil {
		fm := p.model
		if *fast != "" {
			fm = *fast
		}
		cr, err := council.Run(ctx, council.Options{
			Fast:  reply.LLMCaller{Transport: tr, Model: fm, Temperature: 0.5},
			Synth: caller,
		}, req)
		for _, o := range cr.Opinions {
			fmt.Printf("[%s] %s → %s\n", o.Persona, o.Ozet, o.Oneri)
		}
		for i, c := range cr.Candidates {
			mark := ""
			switch {
			case i == cr.Recommended:
				mark = " ★ önerilen"
			case c.Blocked:
				mark = " ✗ Vicdan: " + c.BlockReason
			}
			fmt.Printf("\n-- %s%s\n", c.Tur, mark)
			for j, s := range c.Steps {
				fmt.Printf("  %d. [%.0f sn sonra] %s\n", j+1, s.WaitSec, s.Text)
			}
			for _, v := range c.Violations {
				fmt.Println("  ihlal:", v)
			}
		}
		for _, w := range cr.Warnings {
			fmt.Println("uyarı:", w)
		}
		fmt.Printf("\nçağrı: %d · token: %d\n", cr.Calls, cr.Usage.Total())
		return err
	}
	res, err := reply.Generate(ctx, caller, req)
	if err != nil {
		return err
	}
	for i, s := range res.Steps {
		fmt.Printf("%d. [%.0f sn sonra] %s\n", i+1, s.WaitSec, s.Text)
	}
	if res.Note != "" {
		fmt.Println("not:", res.Note)
	}
	fmt.Printf("deneme: %d · güvenilir: %v · token: %d\n", res.Attempts, res.Confident, res.Usage.Total())
	for _, v := range res.Violations {
		fmt.Println("ihlal:", v)
	}
	return nil
}

func pickConversation(imp *model.Import, partner string, cards []style.Card) (model.Conversation, style.Card, error) {
	var direct []model.Conversation
	for _, c := range imp.Conversations {
		if !c.IsGroup {
			direct = append(direct, c)
		}
	}
	if len(direct) == 0 {
		return model.Conversation{}, style.Card{}, fmt.Errorf("bire-bir sohbet yok")
	}
	idx := -1
	if partner == "" && len(direct) == 1 {
		idx = 0
	}
	for i := range direct {
		if i < len(cards) && partner != "" && model.Norm(cards[i].Who) == model.Norm(partner) {
			idx = i
		}
	}
	if idx < 0 {
		var names []string
		for _, c := range cards {
			names = append(names, c.Who)
		}
		return model.Conversation{}, style.Card{}, fmt.Errorf("karşı taraf belirsiz; --partner ile seç: %s", strings.Join(names, ", "))
	}
	var pc style.Card
	if idx < len(cards) {
		pc = cards[idx]
	}
	conv := direct[idx]
	conv.Messages = append([]model.Message(nil), conv.Messages...)
	sort.SliceStable(conv.Messages, func(i, j int) bool { return conv.Messages[i].TS.Before(conv.Messages[j].TS) })
	return conv, pc, nil
}
