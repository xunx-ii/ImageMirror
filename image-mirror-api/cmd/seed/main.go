package main

import (
	"context"
	"log"
	"strings"

	"github.com/linxunxi/image-mirror/internal/auth"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/database"
)

type pricingSeed struct {
	model   string
	size    string
	quality string
	credits int64
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db, "db/migrations"); err != nil {
		log.Fatal(err)
	}

	seeds := []pricingSeed{
		{"gpt-image-2", "1k", "low", 4},
		{"gpt-image-2", "1k", "medium", 8},
		{"gpt-image-2", "1k", "high", 16},
		{"gpt-image-2", "1k", "auto", 8},
		{"gpt-image-2", "2k", "low", 8},
		{"gpt-image-2", "2k", "medium", 16},
		{"gpt-image-2", "2k", "high", 32},
		{"gpt-image-2", "2k", "auto", 16},
		{"gpt-image-2", "4k", "low", 16},
		{"gpt-image-2", "4k", "medium", 32},
		{"gpt-image-2", "4k", "high", 64},
		{"gpt-image-2", "4k", "auto", 32},
	}
	for _, seed := range seeds {
		if _, err := db.Exec(ctx, `
			INSERT INTO pricing_rules(model, size, quality, credits, is_active)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (model, size, quality)
			DO NOTHING
		`, seed.model, seed.size, seed.quality, seed.credits); err != nil {
			log.Fatal(err)
		}
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Fatal(err)
	}
	adminEmail := strings.ToLower(strings.TrimSpace(cfg.AdminEmail))
	if _, err := db.Exec(ctx, `
		INSERT INTO users(email, password_hash, role, status, balance)
		VALUES ($1, $2, 'ADMIN', 'ACTIVE', 1000)
		ON CONFLICT (email)
		DO UPDATE SET password_hash=EXCLUDED.password_hash, role='ADMIN', status='ACTIVE', updated_at=now()
	`, adminEmail, hash); err != nil {
		log.Fatal(err)
	}

	log.Println("seed completed")
}
