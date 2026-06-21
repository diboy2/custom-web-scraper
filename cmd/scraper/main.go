package main

import (
	"context"
	"log"
	"os"

	"custom-web-scraper/internal/cache"
	"custom-web-scraper/internal/db"
	"custom-web-scraper/internal/scraper"
)

func main() {
	ctx := context.Background()

	database, err := db.Connect(ctx, db.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "scraper"),
		Password: getEnv("DB_PASSWORD", "scraper"),
		Name:     getEnv("DB_NAME", "scraper"),
	})
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer database.Close(ctx)

	var c *cache.Cache
	if addr := getEnv("REDIS_ADDR", ""); addr != "" {
		c = cache.New(addr)
		defer c.Close()
		log.Printf("Redis cache enabled at %s", addr)
	}

	s := scraper.New(database, c)
	if err := s.Run(ctx, "https://www.hellointerview.com"); err != nil {
		log.Fatalf("Scraper failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
