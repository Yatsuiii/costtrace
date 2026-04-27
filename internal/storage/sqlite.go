package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

type CostRow struct {
	Date      string
	Service   string
	UsageType string
	Amount    float64
	Currency  string
}

func (d *DB) UpsertCost(rows []CostRow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO cost_data(date,service,usage_type,amount,currency)
		VALUES(?,?,?,?,?)
		ON CONFLICT(date,service,usage_type) DO UPDATE SET amount=excluded.amount`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.Date, r.Service, r.UsageType, r.Amount, r.Currency); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) CostSince(since time.Time) ([]CostRow, error) {
	rows, err := d.db.Query(
		`SELECT date,service,usage_type,amount,currency FROM cost_data WHERE date >= ? ORDER BY date`,
		since.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostRow
	for rows.Next() {
		var r CostRow
		if err := rows.Scan(&r.Date, &r.Service, &r.UsageType, &r.Amount, &r.Currency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type AnomalyRow struct {
	ID             int64
	DetectedAt     string
	Service        string
	Date           string
	BaselineAmount float64
	ActualAmount   float64
	Delta          float64
	Sigma          float64
}

func (d *DB) InsertAnomalies(rows []AnomalyRow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO anomalies(detected_at,service,date,baseline_amount,actual_amount,delta,sigma)
		VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.DetectedAt, r.Service, r.Date, r.BaselineAmount, r.ActualAmount, r.Delta, r.Sigma); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) Anomalies(since time.Time) ([]AnomalyRow, error) {
	rows, err := d.db.Query(
		`SELECT id,detected_at,service,date,baseline_amount,actual_amount,delta,sigma
		 FROM anomalies WHERE date >= ? ORDER BY delta DESC`,
		since.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AnomalyRow
	for rows.Next() {
		var r AnomalyRow
		if err := rows.Scan(&r.ID, &r.DetectedAt, &r.Service, &r.Date, &r.BaselineAmount, &r.ActualAmount, &r.Delta, &r.Sigma); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetCursor(key string) (string, error) {
	var val string
	err := d.db.QueryRow(`SELECT value FROM sync_cursors WHERE key=?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetCursor(key, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO sync_cursors(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}
