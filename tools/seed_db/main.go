package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"

	"noumena/internal/auth"
)

func main() {
	_ = godotenv.Load()

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./noumena.db"
	}

	ctx := context.Background()
	database, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate&_fk=true")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	users := []struct {
		email    string
		password string
	}{
		{"alice@noumena.dev", "password123"},
		{"bob@noumena.dev", "password123"},
		{"carol@noumena.dev", "password123"},
	}

	userIDs := make(map[string]string)

	for _, u := range users {
		hash, err := auth.HashPassword(u.password)
		if err != nil {
			log.Fatalf("hash password for %s: %v", u.email, err)
		}

		var id string
		err = database.QueryRowContext(ctx,
			`INSERT INTO users (id, email, password_hash, email_verified_at, created_at, updated_at)
			 VALUES (?, ?, ?, datetime('now'), datetime('now'), datetime('now'))
			 ON CONFLICT (email) DO UPDATE SET email_verified_at = excluded.email_verified_at
			 RETURNING id`,
			generateUUID(), u.email, hash).Scan(&id)
		if err != nil {
			err = database.QueryRowContext(ctx,
				`SELECT id FROM users WHERE email = ?`, u.email).Scan(&id)
			if err != nil {
				log.Fatalf("lookup existing user %s: %v", u.email, err)
			}
			fmt.Printf("user exists  %-24s id=%s\n", u.email, id)
		} else {
			fmt.Printf("user created %-24s id=%s\n", u.email, id)
		}
		userIDs[u.email] = id
	}

	type ytChannel struct {
		channelID   string
		displayName string
	}

	channels := []ytChannel{
		{"UCX6OQ3DkcsbYNE6H8uQQuVA", "MrBeast"},
		{"UCBJycsmduvYEL83R_U4JriQ", "Marques Brownlee"},
		{"UCsooa4yRKGN_zEE8iknghZA", "TED"},
		{"UCJ0-OtVpF0wOKEqT2Z1HEtA", "ElectroBOOM"},
		{"UCsBjURrPoezykLs9EqgamOA", "Fireship"},
		{"UC7cs8q-gJRlGwj4A8OmCmXg", "Alex Fridman"},
		{"UC29ju8bIPH5as8OGnQzwJyA", "Traversy Media"},
		{"UCWv7vMbMWH4-V0ZXdmDpPBA", "Programming with Mosh"},
	}

	channelIDs := make(map[string]string)

	for _, ch := range channels {
		var id string
		err := database.QueryRowContext(ctx,
			`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name, created_at)
			 VALUES (?, 'youtube', ?, ?, datetime('now'))
			 ON CONFLICT (platform, platform_channel_id)
			 DO UPDATE SET display_name = excluded.display_name
			 RETURNING id`,
			generateUUID(), ch.channelID, ch.displayName).Scan(&id)
		if err != nil {
			log.Fatalf("upsert channel %s: %v", ch.displayName, err)
		}
		fmt.Printf("channel exists   %-30s id=%s\n", ch.displayName, id)
		channelIDs[ch.channelID] = id
	}

	placeholderAccess, _ := auth.Encrypt("ya29.placeholder-access-token")
	placeholderRefresh, _ := auth.Encrypt("1//placeholder-refresh-token")
	expiresAt := time.Now().Add(24 * 365 * time.Hour)

	tracked := 0
	for email, userID := range userIDs {
		for _, ch := range channels {
			platformChannelUUID := channelIDs[ch.channelID]

			_, err := database.ExecContext(ctx,
				`INSERT INTO user_channel_tracking
				   (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc,
				    token_expires_at, token_priority, tracking_status, backfill_days, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 0, 'active', 90, datetime('now'))
				 ON CONFLICT (user_id, platform_channel_id) DO NOTHING`,
				generateUUID(), userID, platformChannelUUID,
				placeholderAccess, placeholderRefresh,
				expiresAt.Format(time.RFC3339))
			if err != nil {
				log.Printf("tracking %s -> %s: %v", email, ch.displayName, err)
				continue
			}
			tracked++
		}
	}

	fmt.Printf("\nDone. %d tracking entries seeded.\n", tracked)
	fmt.Println("\nTo connect real OAuth tokens:")
	fmt.Println("  1. Set YOUTUBE_OAUTH_CLIENT_ID + YOUTUBE_OAUTH_CLIENT_SECRET in .env")
	fmt.Println("  2. Visit http://localhost:3000 and connect a YouTube account")
	fmt.Println("  3. The ingestion worker will start gathering videos & comments")
}

func generateUUID() string {
	return uuid.New().String()
}
