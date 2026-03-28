package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/qzhello/code-review/internal/model"
)

// ReviewRecord represents a stored review.
type ReviewRecord struct {
	ID         int
	Repo       string
	Branch     string
	CommitHash string
	Mode       string
	Findings   []model.Finding
	Errors     int
	Warnings   int
	Infos      int
	CreatedAt  time.Time
}

// SaveReview stores a review result in the database.
func (d *DB) SaveReview(repo, branch, commitHash, mode string, findings []model.Finding, errors, warnings, infos int) (int64, error) {
	data, err := json.Marshal(findings)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal findings: %w", err)
	}

	result, err := d.db.Exec(
		`INSERT INTO reviews (repo, branch, commit_hash, mode, findings_json, errors, warnings, infos) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		repo, branch, commitHash, mode, string(data), errors, warnings, infos,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to save review: %w", err)
	}

	return result.LastInsertId()
}

// ListReviews returns the most recent reviews.
func (d *DB) ListReviews(limit int) ([]ReviewRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := d.db.Query(
		`SELECT id, repo, branch, commit_hash, mode, errors, warnings, infos, created_at FROM reviews ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ReviewRecord
	for rows.Next() {
		var r ReviewRecord
		if err := rows.Scan(&r.ID, &r.Repo, &r.Branch, &r.CommitHash, &r.Mode, &r.Errors, &r.Warnings, &r.Infos, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetReview returns a single review by ID.
func (d *DB) GetReview(id int) (*ReviewRecord, error) {
	var r ReviewRecord
	var findingsJSON string

	err := d.db.QueryRow(
		`SELECT id, repo, branch, commit_hash, mode, findings_json, errors, warnings, infos, created_at FROM reviews WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Repo, &r.Branch, &r.CommitHash, &r.Mode, &findingsJSON, &r.Errors, &r.Warnings, &r.Infos, &r.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(findingsJSON), &r.Findings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal findings: %w", err)
	}

	return &r, nil
}

// ClearReviews deletes all review history.
func (d *DB) ClearReviews() error {
	_, err := d.db.Exec(`DELETE FROM reviews`)
	return err
}

// DismissFinding records a finding dismissal (legacy, uses dismissals table).
func (d *DB) DismissFinding(ruleID, filePath, contentHash, reason string) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO dismissals (rule_id, file_path, content_hash, reason) VALUES (?, ?, ?, ?)`,
		ruleID, filePath, contentHash, reason,
	)
	return err
}

// IsDismissed checks if a finding has been dismissed (legacy, uses dismissals table).
func (d *DB) IsDismissed(ruleID, filePath, contentHash string) (bool, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM dismissals WHERE rule_id = ? AND file_path = ? AND content_hash = ?`,
		ruleID, filePath, contentHash,
	).Scan(&count)
	return count > 0, err
}

// DismissedFinding represents a persistently dismissed finding.
type DismissedFinding struct {
	ID          int
	Hash        string
	FilePath    string
	RuleID      string
	Message     string
	DismissedAt time.Time
}

// DismissFindingByHash inserts a dismissed finding by its hash (INSERT OR IGNORE).
func (d *DB) DismissFindingByHash(hash, filePath, ruleID, message string) error {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO dismissed_findings (hash, file_path, rule_id, message) VALUES (?, ?, ?, ?)`,
		hash, filePath, ruleID, message,
	)
	return err
}

// UndismissFinding removes a dismissed finding by its hash.
func (d *DB) UndismissFinding(hash string) error {
	_, err := d.db.Exec(`DELETE FROM dismissed_findings WHERE hash = ?`, hash)
	return err
}

// IsDismissedByHash checks if a finding hash exists in the dismissed_findings table.
func (d *DB) IsDismissedByHash(hash string) (bool, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM dismissed_findings WHERE hash = ?`, hash,
	).Scan(&count)
	return count > 0, err
}

// ListDismissed returns all persistently dismissed findings.
func (d *DB) ListDismissed() ([]DismissedFinding, error) {
	rows, err := d.db.Query(
		`SELECT id, hash, file_path, rule_id, message, dismissed_at FROM dismissed_findings ORDER BY dismissed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DismissedFinding
	for rows.Next() {
		var df DismissedFinding
		if err := rows.Scan(&df.ID, &df.Hash, &df.FilePath, &df.RuleID, &df.Message, &df.DismissedAt); err != nil {
			return nil, err
		}
		results = append(results, df)
	}
	return results, rows.Err()
}

// ClearDismissed removes all dismissed findings.
func (d *DB) ClearDismissed() error {
	_, err := d.db.Exec(`DELETE FROM dismissed_findings`)
	return err
}
