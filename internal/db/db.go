package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type DB struct {
	conn *pgx.Conn
}

func Connect(ctx context.Context, cfg Config) (*DB, error) {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close(ctx context.Context) {
	d.conn.Close(ctx)
}

func (d *DB) CreateRun(ctx context.Context) (int, error) {
	var id int
	err := d.conn.QueryRow(ctx, `INSERT INTO scrape_runs DEFAULT VALUES RETURNING id`).Scan(&id)
	return id, err
}

func (d *DB) FinalizeRun(ctx context.Context, runID int) error {
	_, err := d.conn.Exec(ctx, `
		UPDATE scrape_runs SET finished_at = NOW(), status = 'done' WHERE id = $1
	`, runID)
	return err
}

func (d *DB) UpsertPage(ctx context.Context, url string) (int, error) {
	var id int
	err := d.conn.QueryRow(ctx, `
		INSERT INTO pages (url) VALUES ($1)
		ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		RETURNING id
	`, url).Scan(&id)
	return id, err
}

func (d *DB) InsertContent(ctx context.Context, runID, pageID, statusCode int, rawHTML string, extracted any) error {
	jsonPayload, err := json.Marshal(extracted)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(ctx, `
		INSERT INTO page_content (run_id, page_id, status_code, raw_html, extracted)
		VALUES ($1, $2, $3, $4, $5)
	`, runID, pageID, statusCode, rawHTML, jsonPayload)
	return err
}

func (d *DB) PageHasContent(ctx context.Context, url string) (bool, error) {
	var exists bool
	err := d.conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pages p
			JOIN page_content pc ON pc.page_id = p.id
			WHERE p.url = $1 AND pc.error IS NULL
		)
	`, url).Scan(&exists)
	return exists, err
}

func (d *DB) InsertError(ctx context.Context, runID, pageID, statusCode int, errMsg string) error {
	_, err := d.conn.Exec(ctx, `
		INSERT INTO page_content (run_id, page_id, status_code, error)
		VALUES ($1, $2, $3, $4)
	`, runID, pageID, statusCode, errMsg)
	return err
}
