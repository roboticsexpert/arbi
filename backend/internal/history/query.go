package history

import (
	"strings"
	"time"
)

// PathInfo describes a chain path that has history.
type PathInfo struct {
	Path      string `json:"path"`
	FirstSeen int64  `json:"first_seen"` // unix seconds
	LastSeen  int64  `json:"last_seen"`
}

// Point is one bucket of a chain's profit percentage.
type Point struct {
	T        int64   `json:"t"` // unix seconds, bucket start
	Avg      float64 `json:"avg"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Last     float64 `json:"last"`
	AvgNoFee float64 `json:"avg_no_fee"`
	Samples  int     `json:"samples"`
}

// Steps a query can be bucketed to, in minutes. Minute resolution is kept
// until a range would exceed MaxPoints, then it steps up.
var Steps = []int{1, 5, 15, 60, 240, 1440}

// MaxPoints caps the points returned per series for an automatic step.
const MaxPoints = 3000

// AutoStep picks the smallest step (minutes) that keeps a range under MaxPoints.
func AutoStep(from, to time.Time) int {
	minutes := int(to.Sub(from).Minutes())
	for _, s := range Steps {
		if minutes/s <= MaxPoints {
			return s
		}
	}
	return Steps[len(Steps)-1]
}

// Paths lists every path with history, most recently seen first.
func (r *Recorder) Paths() ([]PathInfo, error) {
	rows, err := r.db.Query(`SELECT path, first_seen, last_seen FROM chain_paths ORDER BY last_seen DESC, path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PathInfo{}
	for rows.Next() {
		var p PathInfo
		if err := rows.Scan(&p.Path, &p.FirstSeen, &p.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Series returns bucketed points for each requested path in [from, to).
// Paths with no history are present with an empty slice. Averages are weighted
// by sample count so a half-filled minute doesn't count as much as a full one.
func (r *Recorder) Series(paths []string, from, to time.Time, stepMinutes int) (map[string][]Point, error) {
	out := make(map[string][]Point, len(paths))
	if len(paths) == 0 {
		return out, nil
	}
	for _, p := range paths {
		out[p] = []Point{}
	}

	step := int64(stepMinutes) * 60
	args := []any{step, step, from.Unix(), to.Unix()}
	for _, p := range paths {
		args = append(args, p)
	}

	// Bucket first, then join back on the bucket's latest minute to get its
	// last value (a bare column next to several min/max aggregates is undefined
	// in SQLite).
	rows, err := r.db.Query(`
		WITH b AS (
			SELECT m.path_id,
			       (m.minute / ?) * ?                              AS t,
			       sum(m.avg * m.samples) / sum(m.samples)         AS avg,
			       min(m.min)                                      AS lo,
			       max(m.max)                                      AS hi,
			       max(m.minute)                                   AS latest,
			       sum(m.avg_no_fee * m.samples) / sum(m.samples)  AS avg_no_fee,
			       sum(m.samples)                                  AS samples
			FROM chain_minutes m
			JOIN chain_paths p ON p.id = m.path_id
			WHERE m.minute >= ? AND m.minute < ?
			  AND p.path IN (?`+strings.Repeat(",?", len(paths)-1)+`)
			GROUP BY m.path_id, t
		)
		SELECT p.path, b.t, b.avg, b.lo, b.hi, l.last, b.avg_no_fee, b.samples
		FROM b
		JOIN chain_paths p   ON p.id = b.path_id
		JOIN chain_minutes l ON l.path_id = b.path_id AND l.minute = b.latest
		ORDER BY p.path, b.t`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			path string
			pt   Point
		)
		if err := rows.Scan(&path, &pt.T, &pt.Avg, &pt.Min, &pt.Max, &pt.Last, &pt.AvgNoFee, &pt.Samples); err != nil {
			return nil, err
		}
		out[path] = append(out[path], pt)
	}
	return out, rows.Err()
}
