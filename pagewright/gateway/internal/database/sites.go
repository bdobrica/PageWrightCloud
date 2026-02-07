package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

// Site operations

func (db *DB) CreateSite(userID, fqdn, templateID string) (*types.Site, error) {
	site := &types.Site{
		ID:         uuid.New().String(),
		FQDN:       fqdn,
		UserID:     userID,
		TemplateID: templateID,
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	query := `
		INSERT INTO sites (id, fqdn, user_id, template_id, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := db.Exec(query, site.ID, site.FQDN, site.UserID, site.TemplateID, site.Enabled, site.CreatedAt, site.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create site: %w", err)
	}

	return site, nil
}

func (db *DB) GetSiteByFQDN(fqdn string) (*types.Site, error) {
	site := &types.Site{}
	query := `
		SELECT id, fqdn, user_id, template_id, live_version_id, preview_version_id, enabled, created_at, updated_at
		FROM sites WHERE fqdn = $1
	`

	err := db.Get(site, query, fqdn)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site by fqdn: %w", err)
	}

	return site, nil
}

func (db *DB) GetUserSites(userID string, limit, offset int) ([]types.Site, int, error) {
	var sites []types.Site
	query := `
		SELECT id, fqdn, user_id, template_id, live_version_id, preview_version_id, enabled, created_at, updated_at
		FROM sites WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	err := db.Select(&sites, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user sites: %w", err)
	}

	// Get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM sites WHERE user_id = $1`
	err = db.Get(&totalCount, countQuery, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get sites count: %w", err)
	}

	return sites, totalCount, nil
}

func (db *DB) UpdateSiteEnabled(fqdn string, enabled bool) error {
	query := `UPDATE sites SET enabled = $1, updated_at = $2 WHERE fqdn = $3`
	_, err := db.Exec(query, enabled, time.Now(), fqdn)
	if err != nil {
		return fmt.Errorf("failed to update site enabled: %w", err)
	}
	return nil
}

func (db *DB) UpdateSiteVersions(fqdn string, liveVersionID, previewVersionID *string) error {
	query := `
		UPDATE sites 
		SET live_version_id = $1, preview_version_id = $2, updated_at = $3 
		WHERE fqdn = $4
	`
	_, err := db.Exec(query, liveVersionID, previewVersionID, time.Now(), fqdn)
	if err != nil {
		return fmt.Errorf("failed to update site versions: %w", err)
	}
	return nil
}

func (db *DB) DeleteSite(fqdn string) error {
	query := `DELETE FROM sites WHERE fqdn = $1`
	_, err := db.Exec(query, fqdn)
	if err != nil {
		return fmt.Errorf("failed to delete site: %w", err)
	}
	return nil
}
