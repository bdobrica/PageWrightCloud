package database

import (
	"context"
	"database/sql"
)

// ReadBuilds returns one consistent, owner-scoped page, including uncertain
// submissions that have no manager receipt. It never initiates execution.
func (db *DB) ReadBuilds(ctx context.Context, owner, site, job string, limit, offset int) ([]BuildSubmission, int, error) {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	const scope = ` FROM build_submissions b JOIN sites s ON s.id=b.site_id WHERE b.owner_id=$1 AND s.user_id=$1 AND b.site_id=$2 AND ($3='' OR b.job_id::text=$3)`
	var total int
	if err := tx.GetContext(ctx, &total, `SELECT count(*)`+scope, owner, site, job); err != nil {
		return nil, 0, err
	}
	rows := []BuildSubmission{}
	if err := tx.SelectContext(ctx, &rows, `SELECT b.*`+scope+` ORDER BY b.created_at DESC,b.job_id DESC LIMIT $4 OFFSET $5`, owner, site, job, limit, offset); err != nil {
		return nil, 0, err
	}
	return rows, total, tx.Commit()
}

// UnconfirmedBuilds prevents storage materialization from overriding durable
// execution outcomes. Legacy artifacts without submissions remain visible.
func (db *DB) UnconfirmedBuilds(ctx context.Context, site string) (map[string]bool, error) {
	ids := []string{}
	err := db.SelectContext(ctx, &ids, `SELECT target_version FROM build_submissions WHERE site_id=$1 AND status <> 'completed'`, site)
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result, err
}
