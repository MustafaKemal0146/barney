// barney komut satırı. Şimdilik: `barney style <dosya|zip|dizin> [--owner "Ad"] [--json]`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MustafaKemal0146/barney/internal/ingest"
	"github.com/MustafaKemal0146/barney/internal/ingest/zipfs"
	"github.com/MustafaKemal0146/barney/internal/style"
)

func main() {
	var err error
	switch {
	case len(os.Args) >= 2 && os.Args[1] == "style":
		err = runStyle(os.Args[2:])
	case len(os.Args) >= 2 && os.Args[1] == "reply":
		err = runReply(os.Args[2:])
	case len(os.Args) >= 2 && os.Args[1] == "serve":
		err = runServe(os.Args[2:])
	case len(os.Args) >= 2 && os.Args[1] == "extension":
		err = runExtension(os.Args[2:])
	case len(os.Args) >= 2 && os.Args[1] == "url":
		err = runURL(os.Args[2:])
	case len(os.Args) >= 2 && os.Args[1] == "doctor":
		err = runDoctor(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "kullanım:\n  barney style <sohbet> [--owner \"Ad\"] [--json]\n  barney reply <sohbet> --owner \"Ad\" [--partner \"Ad\"] [--goal ...] [--council] [--dry]\n  barney serve --profile <sohbet> --owner \"Ad\" --allow \"Kişi\" [--auto-send] [--council]\n  barney extension install [klasör]\n  barney doctor\n  barney url [--dev]")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hata:", err)
		os.Exit(1)
	}
}

func runStyle(args []string) error {
	fs := flag.NewFlagSet("style", flag.ContinueOnError)
	owner := fs.String("owner", "", "sohbetin sahibi (sen) — otomatik bulunamazsa gerekir")
	asJSON := fs.Bool("json", false, "kartları JSON olarak yaz")
	// Bayraklar konumsal argümandan önce de sonra da verilebilsin.
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
	if len(pos) != 1 {
		return fmt.Errorf("tam olarak bir kaynak yolu ver")
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
	if *owner != "" {
		imp.OwnerName, imp.OwnerGuessed = *owner, false
	}
	if imp.OwnerName == "" || imp.OwnerGuessed && len(imp.Candidates) > 1 {
		return fmt.Errorf("sohbetin sahibi belirsiz; --owner ile ver. Adaylar: %s", strings.Join(imp.Candidates, ", "))
	}
	ownerCard, partners := style.FromImport(imp)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"owner": ownerCard, "partners": partners})
	}
	printCard(ownerCard, "SEN")
	for _, p := range partners {
		printCard(p, "KARŞI TARAF")
	}
	for _, w := range imp.Warnings {
		fmt.Println("uyarı:", w)
	}
	return nil
}

func pc(x float64) string { return fmt.Sprintf("%%%.0f", x*100) }

func printCard(c style.Card, role string) {
	fmt.Printf("\n== %s: %s — %d metin mesajı, güven: %s ==\n", role, c.Who, c.Messages, c.Confidence)
	if c.Messages == 0 {
		return
	}
	fmt.Printf("patlama   : tek mesaj %s · 2-3 mesaj %s · 4+ %s (ortanca %.0f, en çok %d, aralık ~%.0f sn)\n",
		pc(c.Burst.P1), pc(c.Burst.P2to3), pc(c.Burst.P4plus), c.Burst.MedianSize, c.Burst.MaxSize, c.Burst.InBurstGapSecs)
	fmt.Printf("uzunluk   : ortanca %.0f sözcük (p90 %.0f), %.0f cümle, çok satırlı %s\n",
		c.Length.WordsMedian, c.Length.WordsP90, c.Length.SentencesMed, pc(c.Length.MultiLineRatio))
	fmt.Printf("harf      : küçük harfle başlama %s, hiç büyük harf yok %s, uzatma (çooook) %s\n",
		pc(c.Case.LowerStartRatio), pc(c.Case.AllLowerRatio), pc(c.Elongation))
	fmt.Printf("noktalama : nokta %s, noktalamasız %s, üç nokta %s, soru %s, ünlem %s, ??/!! %s\n",
		pc(c.Punct.EndPeriodRatio), pc(c.Punct.EndNoneRatio), pc(c.Punct.EllipsisRatio),
		pc(c.Punct.QuestionRatio), pc(c.Punct.ExclaimRatio), pc(c.Punct.RepeatRatio))
	fmt.Printf("emoji     : mesajların %s'inde, mesaj başına %.2f — %s\n", pc(c.Emoji.MessageRatio), c.Emoji.PerMessage, strings.Join(c.Emoji.Top, " "))
	fmt.Printf("gecikme   : ortanca %.0f sn, p90 %.0f sn (%d örnek)\n", c.Latency.MedianSec, c.Latency.P90Sec, c.Latency.Samples)
	var sig []string
	for _, w := range c.Signature {
		sig = append(sig, w.Word)
	}
	fmt.Printf("imza sözcükler: %s\n", strings.Join(sig, ", "))
	if c.Confidence == style.ConfLow {
		fmt.Println("(az mesaj: kart güvenilir değil, en az ~300 mesaj önerilir)")
	}
}
