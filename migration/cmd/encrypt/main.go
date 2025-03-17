package main

import (
	"flag"
	"log"

	migrations "github.com/opd-ai/paywall/migration"
)

func main() {
	keyPath := flag.String("key", "./keys/store.key", "Path to encryption key file")
	base := flag.String("base", "./paywallet", "Base directory for payment files")
	flag.Parse()

	if err := migrations.EncryptExisting(*keyPath, *base); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
}
