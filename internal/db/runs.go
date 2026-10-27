package db

import (
	"database/sql"
	"time"
)

// GenerationRun contains only safe generation metadata, never prompts or output.
type GenerationRun struct {
	ID             int64   `json:"id"`
	Platform       string  `json:"platform"`
	Owner          string  `json:"owner"`
	Repo           string  `json:"repo"`
	Source         string  `json:"source"`
	ToTag          string  `json:"to_tag"`
	FromTag        string  `json:"from_tag"`
	Profile        string  `json:"profile"`
	DisplayVersion string  `json:"display_version"`
	Status         string  `json:"status"`
	ErrorCode      *string `json:"error_code"`
	WarningCode    *string `json:"warning_code"`
	StartedAt      string  `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
}

const runColumns = `id, platform, owner, repo, source, to_tag, from_tag, profile, display_version, status, error_code, warning_code, started_at, finished_at`

// StartRun records acceptance before generation is allowed to perform remote work.
func (s *Store) StartRun(run GenerationRun) (int64, error) {
	result, err := s.db.Exec(`INSERT INTO generation_runs (platform, owner, repo, source, to_tag, from_tag, profile, display_version, status, started_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', ?)`, run.Platform, run.Owner, run.Repo, run.Source, run.ToTag, run.FromTag, run.Profile, run.DisplayVersion, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FinishRun deliberately does not use the cancelled request's context.
func (s *Store) FinishRun(id int64, fromTag, status, errorCode, warningCode string) error {
	_, err := s.db.Exec(`UPDATE generation_runs SET from_tag = ?, status = ?, error_code = ?, warning_code = ?, finished_at = ? WHERE id = ? AND status = 'running'`, fromTag, status, strOrNil(errorCode), strOrNil(warningCode), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ReconcileRuns is called once by daemon startup, never by database construction.
func (s *Store) ReconcileRuns() error {
	_, err := s.db.Exec(`UPDATE generation_runs SET status = 'interrupted', error_code = 'interrupted', finished_at = ? WHERE status = 'running'`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func scanRuns(rows *sql.Rows) ([]GenerationRun, error) {
	defer rows.Close()
	runs := make([]GenerationRun, 0)
	for rows.Next() {
		var run GenerationRun
		if err := rows.Scan(&run.ID, &run.Platform, &run.Owner, &run.Repo, &run.Source, &run.ToTag, &run.FromTag, &run.Profile, &run.DisplayVersion, &run.Status, &run.ErrorCode, &run.WarningCode, &run.StartedAt, &run.FinishedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// ListRuns returns newest accepted attempts first. Empty identity means all repos.
func (s *Store) ListRuns(platform, owner, repo string, limit int) ([]GenerationRun, error) {
	query := `SELECT ` + runColumns + ` FROM generation_runs`
	args := make([]any, 0, 4)
	if platform != "" {
		query += ` WHERE platform = ? AND owner = ? AND repo = ?`
		args = append(args, platform, owner, repo)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanRuns(rows)
}

// LatestRuns loads one indexed latest attempt for each managed repository in one query.
func (s *Store) LatestRuns() ([]GenerationRun, error) {
	rows, err := s.db.Query(`SELECT ` + runColumns + ` FROM generation_runs WHERE id IN (
		SELECT (SELECT id FROM generation_runs AS r WHERE r.platform = s.platform AND r.owner = s.owner AND r.repo = s.repo ORDER BY id DESC LIMIT 1) FROM repo_settings AS s
	)`)
	if err != nil {
		return nil, err
	}
	return scanRuns(rows)
}
