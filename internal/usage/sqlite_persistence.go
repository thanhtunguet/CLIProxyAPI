package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

const defaultSQLiteFlushInterval = 30 * time.Second

type sqliteSnapshotEnvelope struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at,omitempty"`
	Usage      StatisticsSnapshot `json:"usage"`
}

type sqliteUsagePersistence struct {
	stats *RequestStatistics
	db    *sql.DB

	flushInterval time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}

	mu     sync.Mutex
	closed bool
}

// StartSQLitePersistence enables SQLite-backed persistence for the shared usage statistics store.
// When sqlitePath is empty, persistence is disabled and no error is returned.
func StartSQLitePersistence(
	stats *RequestStatistics,
	sqlitePath string,
	flushInterval time.Duration,
) (func(context.Context) error, error) {
	path := strings.TrimSpace(sqlitePath)
	if path == "" {
		return nil, nil
	}
	if stats == nil {
		return nil, fmt.Errorf("usage sqlite persistence: statistics store is nil")
	}
	if flushInterval <= 0 {
		flushInterval = defaultSQLiteFlushInterval
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("usage sqlite persistence: create parent directory failed: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("usage sqlite persistence: open sqlite failed: %w", err)
	}

	if err = configureSQLite(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	p := &sqliteUsagePersistence{
		stats:         stats,
		db:            db,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	if err = p.loadSnapshot(); err != nil {
		_ = db.Close()
		return nil, err
	}

	go p.loop()
	return p.Stop, nil
}

func configureSQLite(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("usage sqlite persistence: db is nil")
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("usage sqlite persistence: set busy_timeout failed: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		log.Warnf("usage sqlite persistence: set WAL mode failed: %v", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS usage_statistics_snapshot (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	version INTEGER NOT NULL,
	exported_at TEXT NOT NULL,
	payload TEXT NOT NULL
);`); err != nil {
		return fmt.Errorf("usage sqlite persistence: create table failed: %w", err)
	}
	return nil
}

func (p *sqliteUsagePersistence) loop() {
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()
	defer close(p.doneCh)
	defer func() {
		if err := p.db.Close(); err != nil {
			log.Errorf("usage sqlite persistence: close sqlite failed: %v", err)
		}
	}()

	for {
		select {
		case <-ticker.C:
			if err := p.persistSnapshot(); err != nil {
				log.Errorf("usage sqlite persistence: periodic flush failed: %v", err)
			}
		case <-p.stopCh:
			if err := p.persistSnapshot(); err != nil {
				log.Errorf("usage sqlite persistence: shutdown flush failed: %v", err)
			}
			return
		}
	}
}

func (p *sqliteUsagePersistence) loadSnapshot() error {
	if p == nil || p.db == nil || p.stats == nil {
		return nil
	}

	var payload string
	err := p.db.QueryRow(`SELECT payload FROM usage_statistics_snapshot WHERE id = 1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("usage sqlite persistence: read snapshot failed: %w", err)
	}

	var envelope sqliteSnapshotEnvelope
	if err = json.Unmarshal([]byte(payload), &envelope); err != nil {
		return fmt.Errorf("usage sqlite persistence: decode snapshot failed: %w", err)
	}
	result := p.stats.MergeSnapshot(envelope.Usage)
	log.Infof("usage sqlite persistence: loaded snapshot (added=%d skipped=%d)", result.Added, result.Skipped)
	return nil
}

func (p *sqliteUsagePersistence) persistSnapshot() error {
	if p == nil || p.db == nil || p.stats == nil {
		return nil
	}

	envelope := sqliteSnapshotEnvelope{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      p.stats.Snapshot(),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("usage sqlite persistence: encode snapshot failed: %w", err)
	}

	_, err = p.db.Exec(`
INSERT INTO usage_statistics_snapshot (id, version, exported_at, payload)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	version = excluded.version,
	exported_at = excluded.exported_at,
	payload = excluded.payload
`, envelope.Version, envelope.ExportedAt.Format(time.RFC3339Nano), string(raw))
	if err != nil {
		return fmt.Errorf("usage sqlite persistence: write snapshot failed: %w", err)
	}
	return nil
}

func (p *sqliteUsagePersistence) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.stopCh)
	p.mu.Unlock()

	if ctx == nil {
		<-p.doneCh
		return nil
	}
	select {
	case <-p.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
