// Command tenantctl provisions tenants for the M-Pesa gateway: each tenant
// gets an API key (used to authenticate /initiate) and a webhook signing
// secret (used to HMAC-sign outgoing webhooks). Both are shown once, at
// creation time, and are never retrievable afterwards.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mpesa-gateway/internal/database"
	"github.com/mpesa-gateway/internal/tenant"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		runCreate(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tenantctl create -name <tenant name>")
}

func runCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	name := fs.String("name", "", "tenant name (required)")
	fs.Parse(args)

	if *name == "" {
		log.Fatal("-name is required")
	}

	databaseURL := os.Getenv("MPESA_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MPESA_DATABASE_URL is required")
	}

	ctx := context.Background()

	db, err := database.NewDatabase(ctx, databaseURL, 1, 2)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	store := tenant.NewStore(db.Pool)

	t, apiKey, err := store.Create(ctx, *name)
	if err != nil {
		log.Fatalf("Failed to create tenant: %v", err)
	}

	fmt.Printf("Tenant created: %s (id: %s)\n", t.Name, t.ID)
	fmt.Println()
	fmt.Println("Store these securely — they are shown only once:")
	fmt.Printf("  API Key:                %s\n", apiKey)
	fmt.Printf("  Webhook Signing Secret:  %s\n", t.WebhookSigningSecret)
}
