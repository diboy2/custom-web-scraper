# custom-web-scraper

Scrapes the [Hello Interview Dashboard](https://www.hellointerview.com/dashboard) using Go and stores results in PostgreSQL.

Because the site requires authentication and relies heavily on client-side rendering, a traditional HTML parser like `goquery` will not work — browser automation is required.

## System Architecture

```
+-----------------------------------------------------------+
|                        Go Application                     |
|                                                           |
|  +--------------------+         +----------------------+  |
|  |   Browser Agent    |         |    Database Worker   |  |
|  | (chromedp)         |         |      (pgx/Bun)       |  |
+--+--------+-----------+---------+-----------+----------+--+
            |                                 |
            | Navigates & Extracts            | Persists Relational Data
            v                                 v
+-----------------------+    Hit  +----------------------+
|   Target Website      |<--------|    Redis Cache       |
| Hello Interview Dash  |  Miss   | (recently fetched    |
+-----------------------+         |   page HTML/data)    |
                                  +----------------------+
                                           |
                                           | Cache Miss → persist
                                           v
                                  +----------------------+
                                  | PostgreSQL Database  |
                                  |                      |
                                  +----------------------+
```

## Implementation Blueprint

### 1. Initialize Go Project

```bash
mkdir go-agent-scraper && cd go-agent-scraper
go mod init go-agent-scraper
go get -u github.com/chromedp/chromedp
go get -u github.com/jackc/pgx/v5
go get -u github.com/redis/go-redis/v9
```

### 2. PostgreSQL Schema

```sql
CREATE TABLE IF NOT EXISTS dashboard_metrics (
    id SERIAL PRIMARY KEY,
    scraped_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    user_track VARCHAR(255),
    progress_percentage INT,
    completed_lessons INT,
    raw_json_payload JSONB
);
```

### 3. Redis Cache Layer

Before launching a browser session, the scraper checks Redis for a recently cached copy of the page. Cache entries expire after a configurable TTL (default: 10 minutes), keeping results fresh without unnecessary browser automation.

```go
package cache

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"
)

const defaultTTL = 10 * time.Minute

type PageCache struct {
    client *redis.Client
    ttl    time.Duration
}

func New(addr string) *PageCache {
    return &PageCache{
        client: redis.NewClient(&redis.Options{Addr: addr}),
        ttl:    defaultTTL,
    }
}

// Get returns the cached HTML for a URL, or ("", false) on a miss.
func (c *PageCache) Get(ctx context.Context, url string) (string, bool) {
    val, err := c.client.Get(ctx, url).Result()
    if err != nil {
        return "", false
    }
    return val, true
}

// Set stores raw HTML for a URL with the configured TTL.
func (c *PageCache) Set(ctx context.Context, url string, html string) error {
    return c.client.Set(ctx, url, html, c.ttl).Err()
}
```

Usage in the scraper — check the cache before opening a browser:

```go
cache := cache.New("localhost:6379")

const targetURL = "https://www.hellointerview.com/dashboard"

if html, ok := cache.Get(ctx, targetURL); ok {
    log.Println("Cache hit — skipping browser session")
    parseAndStore(html)
    return
}

// Cache miss: run the full browser session
var rawHTML string
err := chromedp.Run(ctx,
    chromedp.Navigate(targetURL),
    chromedp.WaitVisible(`.dashboard-track-title`, chromedp.ByQuery),
    chromedp.OuterHTML(`html`, &rawHTML, chromedp.ByQuery),
)
if err != nil {
    log.Fatalf("Scrape failed: %v", err)
}

cache.Set(ctx, targetURL, rawHTML)
parseAndStore(rawHTML)
```

**Cache key strategy**: use the full URL as the key. For authenticated sessions where the same URL returns user-specific data, append a user identifier (e.g., `targetURL + ":" + userID`) so cached entries are scoped per user.

### 4. Go Scraper

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "time"

    "github.com/chromedp/chromedp"
    "github.com/jackc/pgx/v5"
)

type MetricData struct {
    UserTrack          string `json:"user_track"`
    ProgressPercentage int    `json:"progress_percentage"`
    CompletedLessons   int    `json:"completed_lessons"`
}

func main() {
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()

    ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
    defer cancel()

    var userTrack string
    var rawHTML string

    log.Println("Starting agentic browser session...")

    err := chromedp.Run(ctx,
        chromedp.Navigate("https://www.hellointerview.com/dashboard"),

        // If login is required, inject cookies or automate the login form:
        // chromedp.WaitVisible(`#login-email`),
        // chromedp.SendKeys(`#login-email`, "your-email@example.com"),
        // chromedp.Click(`#login-submit`),

        chromedp.WaitVisible(`.dashboard-track-title`, chromedp.ByQuery),
        chromedp.Text(`.dashboard-track-title`, &userTrack, chromedp.ByQuery),
        chromedp.OuterHTML(`html`, &rawHTML, chromedp.ByQuery),
    )
    if err != nil {
        log.Fatalf("Failed scraping dashboard: %v", err)
    }

    extractedMetrics := MetricData{
        UserTrack:          userTrack,
        ProgressPercentage: 75,
        CompletedLessons:   12,
    }

    dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer dbCancel()

    connStr := "postgres://username:password@localhost:5432/your_database"
    conn, err := pgx.Connect(dbCtx, connStr)
    if err != nil {
        log.Fatalf("Unable to connect to database: %v", err)
    }
    defer conn.Close(dbCtx)

    jsonPayload, err := json.Marshal(extractedMetrics)
    if err != nil {
        log.Fatalf("Failed to marshal metrics: %v", err)
    }

    insertStmt := `
        INSERT INTO dashboard_metrics (user_track, progress_percentage, completed_lessons, raw_json_payload)
        VALUES ($1, $2, $3, $4);
    `
    _, err = conn.Exec(dbCtx, insertStmt,
        extractedMetrics.UserTrack,
        extractedMetrics.ProgressPercentage,
        extractedMetrics.CompletedLessons,
        jsonPayload,
    )
    if err != nil {
        log.Fatalf("Failed to insert data into Postgres: %v", err)
    }

    log.Println("Successfully scraped dashboard and stored data to PostgreSQL!")
}
```

## Making It Agentic

To handle unexpected site changes autonomously:

1. **Fallback LLM Parsing** — if `WaitVisible` fails due to updated class names, capture the full page text via `chromedp.Text("body", &rawText)` and send it to an LLM.
2. **LLM Function Calling** — use the [Google Vertex AI GenAI SDK](https://pkg.go.dev/google.golang.org/genai) or an OpenAI Go client to map raw text to the `MetricData` schema.
3. **Self-Healing Selectors** — have the LLM analyze the HTML structure and return updated CSS selectors that your app can cache for future runs.
