package storage

import (
	"database/sql"
	"fmt"
	"strings"
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

type CloudTrailRow struct {
	EventID      string
	EventTime    time.Time
	EventName    string
	PrincipalID  string
	UserAgent    string
	ResourceType string
	ResourceID   string
	Region       string
	DeployID     string
}

func (d *DB) UpsertCloudTrail(rows []CloudTrailRow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO cloudtrail_events
		(event_id,event_time,event_name,principal_id,user_agent,resource_type,resource_id,region,deploy_id)
		VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.EventID, r.EventTime.UTC().Format(time.RFC3339),
			r.EventName, r.PrincipalID, r.UserAgent, r.ResourceType, r.ResourceID, r.Region, r.DeployID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) CloudTrailForDeploy(deployID string) ([]CloudTrailRow, error) {
	rows, err := d.db.Query(
		`SELECT event_id,event_time,event_name,principal_id,user_agent,resource_type,resource_id,region,deploy_id
		 FROM cloudtrail_events WHERE deploy_id=? ORDER BY event_time`,
		deployID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudTrailRow
	for rows.Next() {
		var r CloudTrailRow
		var ts string
		if err := rows.Scan(&r.EventID, &ts, &r.EventName, &r.PrincipalID, &r.UserAgent,
			&r.ResourceType, &r.ResourceID, &r.Region, &r.DeployID); err != nil {
			return nil, err
		}
		r.EventTime, _ = time.Parse(time.RFC3339, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) CloudTrailForDeployList(deployIDs []string) ([]CloudTrailRow, error) {
	if len(deployIDs) == 0 {
		return nil, nil
	}
	// build IN clause
	placeholders := strings.Repeat("?,", len(deployIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(deployIDs))
	for i, id := range deployIDs {
		args[i] = id
	}
	rows, err := d.db.Query(
		`SELECT event_id,event_time,event_name,principal_id,user_agent,resource_type,resource_id,region,deploy_id
		 FROM cloudtrail_events WHERE deploy_id IN (`+placeholders+`) ORDER BY event_time`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudTrailRow
	for rows.Next() {
		var r CloudTrailRow
		var ts string
		if err := rows.Scan(&r.EventID, &ts, &r.EventName, &r.PrincipalID, &r.UserAgent,
			&r.ResourceType, &r.ResourceID, &r.Region, &r.DeployID); err != nil {
			return nil, err
		}
		r.EventTime, _ = time.Parse(time.RFC3339, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

type DeployRow struct {
	ID          string
	Repo        string
	Branch      string
	CommitSHA   string
	PRNumber    *int
	Title       string
	StartedAt   time.Time
	CompletedAt time.Time
	Status      string
}

func (d *DB) UpsertDeploys(rows []DeployRow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO deploy_events(id,repo,branch,commit_sha,pr_number,title,started_at,completed_at,status)
		VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Repo, r.Branch, r.CommitSHA, r.PRNumber, r.Title,
			r.StartedAt.UTC().Format(time.RFC3339), r.CompletedAt.UTC().Format(time.RFC3339), r.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) DeploysBetween(start, end time.Time) ([]DeployRow, error) {
	rows, err := d.db.Query(
		`SELECT id,repo,branch,commit_sha,pr_number,title,started_at,completed_at,status
		 FROM deploy_events WHERE started_at >= ? AND started_at <= ? AND status='success'
		 ORDER BY started_at`,
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeployRow
	for rows.Next() {
		var r DeployRow
		var startedStr, completedStr string
		if err := rows.Scan(&r.ID, &r.Repo, &r.Branch, &r.CommitSHA, &r.PRNumber, &r.Title,
			&startedStr, &completedStr, &r.Status); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		r.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

type CorrelationRow struct {
	ID         int64
	AnomalyID  int64
	DeployID   string
	Confidence float64
	Evidence   string
}

func (d *DB) UpsertCorrelations(rows []CorrelationRow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO correlations(anomaly_id,deploy_id,confidence,evidence)
		VALUES(?,?,?,?)
		ON CONFLICT(anomaly_id,deploy_id) DO UPDATE SET confidence=excluded.confidence,evidence=excluded.evidence`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.AnomalyID, r.DeployID, r.Confidence, r.Evidence); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) CorrelationsForAnomaly(anomalyID int64) ([]CorrelationRow, error) {
	rows, err := d.db.Query(
		`SELECT id,anomaly_id,deploy_id,confidence,evidence FROM correlations
		 WHERE anomaly_id=? ORDER BY confidence DESC`,
		anomalyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CorrelationRow
	for rows.Next() {
		var r CorrelationRow
		if err := rows.Scan(&r.ID, &r.AnomalyID, &r.DeployID, &r.Confidence, &r.Evidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
