package scraper

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"

	"custom-web-scraper/internal/cache"
)

type cachedTransport struct {
	base  http.RoundTripper
	cache *cache.Cache
	ctx   context.Context
}

func (t *cachedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()

	if html, ok := t.cache.Get(t.ctx, url); ok {
		log.Printf("Cache hit: %s", url)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(html))),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != 200 {
		return resp, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	t.cache.Set(t.ctx, url, string(body))
	log.Printf("Cache miss — stored: %s", url)

	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
