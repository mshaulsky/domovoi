package readings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

// Repository holds the SQL of the series and readings tables. It keeps no
// state: series IDs are resolved per call, which at a batch a minute costs
// nothing measurable and keeps a repository over a transaction free.
type Repository struct {
	db DBTX
}

// Point is one stored value of a series.
type Point struct {
	At    time.Time
	Value model.Value
}

// seriesKey identifies a series within one Write.
type seriesKey struct {
	device model.DeviceID
	metric model.Metric
}

// New returns a repository over a connection or transaction.
func New(db DBTX) *Repository {
	return &Repository{db: db}
}

// Write stores the readings, creating series on first sight. A reading at
// a time the series already has replaces that row.
func (r *Repository) Write(ctx context.Context, readings []model.Reading) error {
	const q = `INSERT INTO readings (series, at, num, text) VALUES (?, ?, ?, ?)
		ON CONFLICT (series, at) DO UPDATE SET num = excluded.num, text = excluded.text`
	ids := map[seriesKey]int64{}
	for _, rd := range readings {
		key := seriesKey{rd.Device, rd.Metric}
		id, ok := ids[key]
		if !ok {
			var err error
			if id, err = r.series(ctx, rd.Device, rd.Metric); err != nil {
				return err
			}
			ids[key] = id
		}
		num, text := columns(rd.Value)
		if _, err := r.db.ExecContext(ctx, q, id, rd.At.Unix(), num, text); err != nil {
			return fmt.Errorf("readings: write %s/%s: %w", rd.Device, rd.Metric, err)
		}
	}
	return nil
}

// Series returns the points of a series since a moment, oldest first.
func (r *Repository) Series(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) ([]Point, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT r.at, r.num, r.text FROM readings r
		JOIN series s ON s.id = r.series
		WHERE s.device = ? AND s.metric = ? AND r.at >= ? ORDER BY r.at`, string(id), string(m), since.Unix())
	if err != nil {
		return nil, fmt.Errorf("readings: series %s/%s: %w", id, m, err)
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		p, err := scan(rows, m)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("readings: series %s/%s: %w", id, m, err)
	}
	return out, nil
}

// Extremes returns the lowest and highest numeric points since a moment;
// ok is false when the series has no numeric point in the window.
func (r *Repository) Extremes(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (lo, hi Point, ok bool, err error) {
	const q = `SELECT r.at, r.num, r.text FROM readings r
		JOIN series s ON s.id = r.series
		WHERE s.device = ? AND s.metric = ? AND r.at >= ? AND r.num IS NOT NULL
		ORDER BY r.num %s, r.at DESC LIMIT 1`
	lo, ok, err = r.one(ctx, fmt.Sprintf(q, "ASC"), m, string(id), string(m), since.Unix())
	if err != nil || !ok {
		return Point{}, Point{}, false, err
	}
	hi, _, err = r.one(ctx, fmt.Sprintf(q, "DESC"), m, string(id), string(m), since.Unix())
	if err != nil {
		return Point{}, Point{}, false, err
	}
	return lo, hi, true, nil
}

// Last returns the newest point of a series: because only changes are
// written, it is also the last change.
func (r *Repository) Last(ctx context.Context, id model.DeviceID, m model.Metric) (Point, bool, error) {
	return r.one(ctx, `SELECT r.at, r.num, r.text FROM readings r
		JOIN series s ON s.id = r.series
		WHERE s.device = ? AND s.metric = ? ORDER BY r.at DESC LIMIT 1`, m, string(id), string(m))
}

// Before returns the newest point at or before a moment: what the value
// was back then.
func (r *Repository) Before(ctx context.Context, id model.DeviceID, m model.Metric, t time.Time) (Point, bool, error) {
	return r.one(ctx, `SELECT r.at, r.num, r.text FROM readings r
		JOIN series s ON s.id = r.series
		WHERE s.device = ? AND s.metric = ? AND r.at <= ? ORDER BY r.at DESC LIMIT 1`, m, string(id), string(m), t.Unix())
}

// LastAll returns the newest point of every series as readings: the state
// to restore after a restart.
func (r *Repository) LastAll(ctx context.Context) ([]model.Reading, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.device, s.metric, r.at, r.num, r.text FROM series s
		JOIN readings r ON r.series = s.id AND r.at = (SELECT MAX(at) FROM readings WHERE series = s.id)
		ORDER BY s.device, s.metric`)
	if err != nil {
		return nil, fmt.Errorf("readings: last of every series: %w", err)
	}
	defer rows.Close()
	var out []model.Reading
	for rows.Next() {
		var rd model.Reading
		var at int64
		var num sql.NullFloat64
		var text sql.NullString
		if err := rows.Scan(&rd.Device, &rd.Metric, &at, &num, &text); err != nil {
			return nil, fmt.Errorf("readings: scan: %w", err)
		}
		rd.At = time.Unix(at, 0)
		rd.Value = value(rd.Metric, num, text)
		out = append(out, rd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("readings: last of every series: %w", err)
	}
	return out, nil
}

// Prune deletes readings older than a moment and reports how many; series
// left without readings go with them.
func (r *Repository) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM readings WHERE at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("readings: prune: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("readings: prune: %w", err)
	}
	// A series left without readings is recreated by the next write; kept,
	// it would pin a retired device's row forever.
	if _, err := r.db.ExecContext(ctx, `DELETE FROM series WHERE NOT EXISTS (SELECT 1 FROM readings WHERE readings.series = series.id)`); err != nil {
		return n, fmt.Errorf("readings: prune empty series: %w", err)
	}
	return n, nil
}

// series returns the ID of a device's metric series, creating it on first
// sight with the catalogue's unit.
func (r *Repository) series(ctx context.Context, id model.DeviceID, m model.Metric) (int64, error) {
	if _, err := r.db.ExecContext(ctx, `INSERT INTO series (device, metric, unit) VALUES (?, ?, ?)
		ON CONFLICT (device, metric) DO NOTHING`, string(id), string(m), nullable(m.Unit())); err != nil {
		return 0, fmt.Errorf("readings: series %s/%s: %w", id, m, err)
	}
	var sid int64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM series WHERE device = ? AND metric = ?`, string(id), string(m)).Scan(&sid); err != nil {
		return 0, fmt.Errorf("readings: series %s/%s: %w", id, m, err)
	}
	return sid, nil
}

// one runs a single-point query.
func (r *Repository) one(ctx context.Context, q string, m model.Metric, args ...any) (Point, bool, error) {
	var at int64
	var num sql.NullFloat64
	var text sql.NullString
	err := r.db.QueryRowContext(ctx, q, args...).Scan(&at, &num, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return Point{}, false, nil
	}
	if err != nil {
		return Point{}, false, fmt.Errorf("readings: query: %w", err)
	}
	return Point{At: time.Unix(at, 0), Value: value(m, num, text)}, true, nil
}

// scan reads one row of (at, num, text).
func scan(rows *sql.Rows, m model.Metric) (Point, error) {
	var at int64
	var num sql.NullFloat64
	var text sql.NullString
	if err := rows.Scan(&at, &num, &text); err != nil {
		return Point{}, fmt.Errorf("readings: scan: %w", err)
	}
	return Point{At: time.Unix(at, 0), Value: value(m, num, text)}, nil
}

// columns splits a value into the num and text columns.
func columns(v model.Value) (num sql.NullFloat64, text sql.NullString) {
	if v.Kind == model.Text {
		return sql.NullFloat64{}, sql.NullString{String: v.Text, Valid: true}
	}
	return sql.NullFloat64{Float64: v.Num, Valid: true}, sql.NullString{}
}

// value rebuilds a value from the columns; the catalogue tells a Bool from
// a Number, and an uncatalogued metric is a Number when num is set.
func value(m model.Metric, num sql.NullFloat64, text sql.NullString) model.Value {
	if text.Valid {
		return model.TextValue(text.String)
	}
	if info, ok := model.Lookup(m); ok && info.Kind == model.Bool {
		return model.BoolValue(num.Float64 != 0)
	}
	return model.NumberValue(num.Float64)
}

func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
