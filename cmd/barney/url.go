package main

import (
	"flag"
	"fmt"

	"github.com/MustafaKemal0146/barney/internal/server"
)

// runURL: barney url [--dev] — arayüz adresini (token hash'te) yazdırır.
func runURL(args []string) error {
	fs := flag.NewFlagSet("url", flag.ContinueOnError)
	port := fs.Int("port", 8777, "sunucu portu")
	vite := fs.Int("vite", 5173, "Vite geliştirme portu")
	dev := fs.Bool("dev", false, "geliştirme (Vite) adresini ver")
	if err := fs.Parse(args); err != nil {
		return err
	}
	secret, err := server.LoadSecret(dataDir())
	if err != nil {
		return err
	}
	p := *port
	if *dev {
		p = *vite
	}
	fmt.Printf("  http://127.0.0.1:%d/#token=%s\n", p, secret)
	return nil
}
