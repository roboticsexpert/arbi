// Package history logs every arbitrage chain's profit percentage to SQLite in
// one-minute buckets, so the dashboard can chart how a route (and its reverse)
// moved over time.
//
// The finder recalculates every 10s. Storing each sample would be ~6 rows per
// chain per minute for no benefit to a minute-resolution chart, so samples are
// folded in memory into a bucket (avg / min / max / last) and the bucket is
// written once the minute rolls over.
package history

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"arbi/internal/arbitrage"

	_ "modernc.org/sqlite" // pure Go, so the static CGO_ENABLED=0 build still works
)

const schema = `
CREATE TABLE IF NOT EXISTS chain_paths (
	id         INTEGER PRIMARY KEY,
	path       TEXT NOT NULL UNIQUE,
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chain_minutes (
	path_id    INTEGER NOT NULL,
	minute     INTEGER NOT NULL, -- unix seconds, start of the minute (UTC)
	samples    INTEGER NOT NULL,
	avg        REAL NOT NULL,
	min        REAL NOT NULL,
	max        REAL NOT NULL,
	last       REAL NOT NULL,
	avg_no_fee REAL NOT NULL,
	PRIMARY KEY (path_id, minute)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS chain_minutes_minute ON chain_minutes (minute);
`

type bucket struct {
	samples       int
	sum, sumNoFee float64
	min, max      float64
	last          float64
}

func (b *bucket) add(pct, pctNoFee float64) {
	if b.samples == 0 || pct < b.min {
		b.min = pct
	}
	if b.samples == 0 || pct > b.max {
		b.max = pct
	}
	b.samples++
	b.sum += pct
	b.sumNoFee += pctNoFee
	b.last = pct
}

// Recorder accumulates chain samples and persists them per minute.
type Recorder struct {
	db        *sql.DB
	retention time.Duration

	mu        sync.Mutex
	minute    int64 // bucket currently being filled
	buckets   map[string]*bucket
	pathIDs   map[string]int64
	lastPrune time.Time
}

// Open creates (or reopens) the history database at path.
func Open(path string, retention time.Duration) (*Recorder, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	// SQLite has one writer; a single connection avoids SQLITE_BUSY between the
	// flush and the HTTP readers, and WAL keeps reads from blocking on it.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema in %s: %w", path, err)
	}

	return &Recorder{
		db:        db,
		retention: retention,
		buckets:   make(map[string]*bucket),
		pathIDs:   make(map[string]int64),
	}, nil
}

// Record folds one finder run into the current minute. It must be called on
// every run, including runs that found nothing, since that is what rolls the
// minute over and flushes the previous bucket.
func (r *Recorder) Record(at time.Time, chains []arbitrage.ArbitrageChain) {
	minute := at.UTC().Truncate(time.Minute).Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	if minute != r.minute {
		if err := r.flushLocked(); err != nil {
			log.Printf("[History] flush failed: %v", err)
		}
		r.minute = minute
	}

	for _, c := range chains {
		b := r.buckets[c.Path]
		if b == nil {
			b = &bucket{}
			r.buckets[c.Path] = b
		}
		b.add(c.ProfitPercent, c.ProfitPercentNoFee)
	}

	if time.Since(r.lastPrune) > time.Hour {
		r.lastPrune = time.Now()
		r.pruneLocked()
	}
}

// Close writes the partially filled minute and closes the database. A restart
// within the same minute merges into that row rather than replacing it.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.flushLocked(); err != nil {
		log.Printf("[History] final flush failed: %v", err)
	}
	return r.db.Close()
}

func (r *Recorder) flushLocked() error {
	if len(r.buckets) == 0 {
		return nil
	}
	defer func() { r.buckets = make(map[string]*bucket) }()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
			r.pathIDs = make(map[string]int64) // may hold ids from the rolled-back tx
		}
	}()

	for path, b := range r.buckets {
		id, err := r.pathIDLocked(tx, path)
		if err != nil {
			return err
		}
		n := float64(b.samples)
		_, err = tx.Exec(`
			INSERT INTO chain_minutes (path_id, minute, samples, avg, min, max, last, avg_no_fee)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (path_id, minute) DO UPDATE SET
				avg        = (avg * samples + excluded.avg * excluded.samples) / (samples + excluded.samples),
				avg_no_fee = (avg_no_fee * samples + excluded.avg_no_fee * excluded.samples) / (samples + excluded.samples),
				min        = min(min, excluded.min),
				max        = max(max, excluded.max),
				last       = excluded.last,
				samples    = samples + excluded.samples`,
			id, r.minute, b.samples, b.sum/n, b.min, b.max, b.last, b.sumNoFee/n)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE chain_paths SET last_seen = max(last_seen, ?) WHERE id = ?`, r.minute, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *Recorder) pathIDLocked(tx *sql.Tx, path string) (int64, error) {
	if id, ok := r.pathIDs[path]; ok {
		return id, nil
	}
	_, err := tx.Exec(`INSERT INTO chain_paths (path, first_seen, last_seen) VALUES (?, ?, ?)
		ON CONFLICT (path) DO NOTHING`, path, r.minute, r.minute)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM chain_paths WHERE path = ?`, path).Scan(&id); err != nil {
		return 0, err
	}
	r.pathIDs[path] = id
	return id, nil
}

func (r *Recorder) pruneLocked() {
	if r.retention <= 0 {
		return
	}
	cutoff := time.Now().Add(-r.retention).Unix()
	res, err := r.db.Exec(`DELETE FROM chain_minutes WHERE minute < ?`, cutoff)
	if err != nil {
		log.Printf("[History] prune failed: %v", err)
		return
	}
	if _, err := r.db.Exec(`DELETE FROM chain_paths WHERE last_seen < ?`, cutoff); err != nil {
		log.Printf("[History] prune paths failed: %v", err)
	}
	r.pathIDs = make(map[string]int64) // ids of pruned paths may be gone
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[History] pruned %d minute rows older than %s", n, r.retention)
	}
}
