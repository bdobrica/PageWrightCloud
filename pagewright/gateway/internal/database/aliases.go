package database

import (
	"database/sql"
	"fmt"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

// Alias operations

func (db *DB) CreateAlias(siteID, alias string) (*types.SiteAlias, error) {
	siteAlias := &types.SiteAlias{
		ID:     uuid.New().String(),
		SiteID: siteID,
		Alias:  alias,
	}

	query := `
		INSERT INTO site_aliases (id, site_id, alias)
		VALUES ($1, $2, $3)
		RETURNING created_at
	`

	err := db.QueryRow(query, siteAlias.ID, siteAlias.SiteID, siteAlias.Alias).Scan(&siteAlias.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	return siteAlias, nil
}

func (db *DB) GetSiteAliases(siteID string) ([]types.SiteAlias, error) {
	var aliases []types.SiteAlias
	query := `SELECT id, site_id, alias, created_at FROM site_aliases WHERE site_id = $1 ORDER BY created_at`

	err := db.Select(&aliases, query, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get site aliases: %w", err)
	}

	return aliases, nil
}

func (db *DB) DeleteAlias(siteID, alias string) error {
	query := `DELETE FROM site_aliases WHERE site_id = $1 AND alias = $2`
	result, err := db.Exec(query, siteID, alias)
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}
