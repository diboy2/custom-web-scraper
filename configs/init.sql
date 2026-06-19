CREATE TABLE IF NOT EXISTS scrape_runs (
    id          SERIAL PRIMARY KEY,
    started_at  TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    status      VARCHAR(20) DEFAULT 'running'
);

CREATE TABLE IF NOT EXISTS pages (
    id          SERIAL PRIMARY KEY,
    url         TEXT UNIQUE NOT NULL,
    nav_section VARCHAR(255),
    nav_label   VARCHAR(255),
    first_seen  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS page_content (
    id          SERIAL PRIMARY KEY,
    run_id      INT REFERENCES scrape_runs(id) ON DELETE CASCADE,
    page_id     INT REFERENCES pages(id) ON DELETE CASCADE,
    scraped_at  TIMESTAMPTZ DEFAULT NOW(),
    status_code INT,
    raw_html    TEXT,
    extracted   JSONB,
    error       TEXT
);

CREATE INDEX IF NOT EXISTS idx_page_content_run_id ON page_content (run_id);
CREATE INDEX IF NOT EXISTS idx_page_content_page_id ON page_content (page_id);
CREATE INDEX IF NOT EXISTS idx_pages_nav_section ON pages (nav_section);
