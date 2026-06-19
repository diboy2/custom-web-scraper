package scraper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"

	"custom-web-scraper/internal/db"
)

const (
	maxRetries = 3
	maxPages   = 3
)

type extractedContent struct {
	Paragraphs []string `json:"paragraphs"`
}

type Scraper struct {
	db *db.DB
}

func New(database *db.DB) *Scraper {
	return &Scraper{db: database}
}

func (s *Scraper) Run(ctx context.Context, startURL string) error {
	runID, err := s.db.CreateRun(ctx)
	if err != nil {
		return fmt.Errorf("create scrape run: %w", err)
	}
	log.Printf("Started scrape run %d", runID)

	c := colly.NewCollector(
		colly.AllowedDomains("www.hellointerview.com"),
	)

	extracted := make(map[string]*extractedContent)
	statusCodes := make(map[string]int)
	rawHTMLs := make(map[string]string)
	pagesScraped := 0

	c.OnRequest(func(r *colly.Request) {
		if pagesScraped >= maxPages {
			r.Abort()
			return
		}
		exists, err := s.db.PageHasContent(ctx, r.URL.String())
		if err != nil {
			log.Printf("DB check failed for %s: %v", r.URL, err)
		} else if exists {
			log.Printf("Skipping already-scraped %s", r.URL)
			r.Abort()
			return
		}
		pagesScraped++
		r.Headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		fmt.Println("Visiting", r.URL)
		extracted[r.URL.String()] = &extractedContent{}
	})

	c.OnResponse(func(r *colly.Response) {
		fmt.Println("Response received", r.StatusCode)
		url := r.Request.URL.String()
		statusCodes[url] = r.StatusCode
		rawHTMLs[url] = string(r.Body)
	})

	c.OnError(func(r *colly.Response, collyErr error) {
		retries, _ := strconv.Atoi(r.Request.Ctx.Get("retries"))
		if retries < maxRetries {
			r.Request.Ctx.Put("retries", strconv.Itoa(retries+1))
			log.Printf("Retrying %s (attempt %d/%d): %v", r.Request.URL, retries+1, maxRetries, collyErr)
			time.Sleep(time.Duration(retries+1) * time.Second)
			r.Request.Retry()
			return
		}

		log.Printf("Failed %s after %d retries: %v", r.Request.URL, maxRetries, collyErr)
		url := r.Request.URL.String()

		pageID, dbErr := s.db.UpsertPage(ctx, url)
		if dbErr != nil {
			log.Printf("Failed to upsert page %s: %v", url, dbErr)
			return
		}
		if dbErr = s.db.InsertError(ctx, runID, pageID, r.StatusCode, collyErr.Error()); dbErr != nil {
			log.Printf("Failed to insert error content for %s: %v", url, dbErr)
		}
	})

	c.OnHTML("a[href]", func(h *colly.HTMLElement) {
		h.Request.Visit(h.Attr("href"))
	})

	c.OnHTML("div.mdx-p", func(h *colly.HTMLElement) {
		url := h.Request.URL.String()
		text := strings.TrimSpace(h.Text)
		if text != "" {
			extracted[url].Paragraphs = append(extracted[url].Paragraphs, text)
		}
	})

	c.OnScraped(func(r *colly.Response) {
		url := r.Request.URL.String()

		pageID, err := s.db.UpsertPage(ctx, url)
		if err != nil {
			log.Printf("Failed to upsert page %s: %v", url, err)
			return
		}
		if err = s.db.InsertContent(ctx, runID, pageID, statusCodes[url], rawHTMLs[url], extracted[url]); err != nil {
			log.Printf("Failed to insert page content for %s: %v", url, err)
			return
		}
		log.Printf("Saved %s (%d paragraphs)", url, len(extracted[url].Paragraphs))
	})

	c.Visit(startURL)

	if err := s.db.FinalizeRun(ctx, runID); err != nil {
		log.Printf("Failed to finalize scrape run: %v", err)
	}
	log.Printf("Scrape run %d complete", runID)
	return nil
}
