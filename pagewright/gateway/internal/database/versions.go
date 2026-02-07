package database

import (
	"fmt"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

// Version operations

func (db *DB) CreateVersion(siteID, buildID, status string) (*types.Version, error) {
	version := &types.Version{
		ID:      uuid.New().String(),
		SiteID:  siteID,
		BuildID: buildID,
		Status:  status,
	}

	query := `
		INSERT INTO versions (id, site_id, build_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`

	err := db.QueryRow(query, version.ID, version.SiteID, version.BuildID, version.Status).Scan(&version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create version: %w", err)
	}

	return version, nil
}

func (db *DB) GetSiteVersions(siteID string, limit, offset int) ([]types.Version, int, error) {
	var versions []types.Version
	query := `
		SELECT id, site_id, build_id, status, created_at
		FROM versions WHERE site_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	err := db.Select(&versions, query, siteID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get site versions: %w", err)
	}

	// Get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM versions WHERE site_id = $1`
	err = db.Get(&totalCount, countQuery, siteID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get versions count: %w", err)
	}

	return versions, totalCount, nil
}

func (db *DB) UpdateVersionStatus(buildID, status string) error {
	query := `UPDATE versions SET status = $1 WHERE build_id = $2`
	_, err := db.Exec(query, status, buildID)
	if err != nil {
		return fmt.Errorf("failed to update version status: %w", err)
	}
	return nil
}

func (db *DB) DeleteVersion(siteID, buildID string) error {
	query := `DELETE FROM versions WHERE site_id = $1 AND build_id = $2`
	_, err := db.Exec(query, siteID, buildID)
	if err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}
	return nil
}

// CleanupOldVersions keeps only the most recent N versions per site
func (db *DB) CleanupOldVersions(siteID string, keepCount int) error {
	query := `
		DELETE FROM versions
		WHERE site_id = $1
		AND id NOT IN (
			SELECT id FROM versions
			WHERE site_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		)
	`

	_, err := db.Exec(query, siteID, keepCount)
	if err != nil {
		return fmt.Errorf("failed to cleanup old versions: %w", err)
	}

	return nil
}
