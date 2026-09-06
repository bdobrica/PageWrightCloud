package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrDeploymentBusy = errors.New("another deployment requires reconciliation")

// One bounded durable record per site. Sequence never resets on replacement.
type Deployment struct {
	SiteID   string `db:"site_id" json:"site_id"`
	Sequence int64  `db:"sequence" json:"sequence"`
	FQDN     string `db:"fqdn" json:"fqdn"`
	Version  string `db:"version" json:"version"`
	Target   string `db:"target" json:"target"`
	Status   string `db:"status" json:"status"`
}

const deploymentColumns = `site_id,sequence,fqdn,version,target,status`

func (db *DB) ReserveDeployment(ctx context.Context, siteID, version, target string) (*Deployment, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var fqdn string
	if err = tx.GetContext(ctx, &fqdn, `SELECT fqdn FROM sites WHERE id=$1 FOR UPDATE`, siteID); err != nil {
		return nil, err
	}
	var d Deployment
	err = tx.GetContext(ctx, &d, `SELECT `+deploymentColumns+` FROM deployments WHERE site_id=$1`, siteID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && d.Status == "pending" {
		if d.Version != version || d.Target != target {
			return nil, ErrDeploymentBusy
		}
	} else {
		err = tx.GetContext(ctx, &d, `INSERT INTO deployments(site_id,fqdn,version,target,status) VALUES($1,$2,$3,$4,'pending')
   ON CONFLICT(site_id) DO UPDATE SET sequence=nextval('deployment_sequence'),fqdn=EXCLUDED.fqdn,version=EXCLUDED.version,target=EXCLUDED.target,status='pending',updated_at=now()
   RETURNING `+deploymentColumns, siteID, fqdn, version, target)
		if err != nil {
			return nil, err
		}
	}
	return &d, tx.Commit()
}

// The receipt and pointer commit together, conditional on the exact intent.
func (db *DB) FinishDeployment(ctx context.Context, d *Deployment, status string) error {
	if status != "completed" && status != "failed" {
		return errors.New("nonterminal receipt")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err = tx.GetContext(ctx, &id, `SELECT id FROM sites WHERE id=$1 FOR UPDATE`, d.SiteID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE deployments SET status=$1,updated_at=now() WHERE site_id=$2 AND sequence=$3 AND version=$4 AND target=$5 AND status='pending'`, status, d.SiteID, d.Sequence, d.Version, d.Target)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrDeploymentBusy
	}
	if status == "completed" {
		_, err = tx.ExecContext(ctx, `UPDATE sites SET live_version_id=CASE WHEN $2='live' THEN $3 ELSE live_version_id END,preview_version_id=CASE WHEN $2='preview' THEN $3 ELSE preview_version_id END,updated_at=now() WHERE id=$1`, d.SiteID, d.Target, d.Version)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) PendingDeployments(ctx context.Context) ([]Deployment, error) {
	ds := []Deployment{}
	err := db.SelectContext(ctx, &ds, `SELECT `+deploymentColumns+` FROM deployments WHERE status='pending' ORDER BY updated_at LIMIT 25`)
	return ds, err
}

func (db *DB) TouchDeployment(ctx context.Context, d *Deployment) {
	_, _ = db.ExecContext(ctx, `UPDATE deployments SET updated_at=now() WHERE site_id=$1 AND sequence=$2 AND status='pending'`, d.SiteID, d.Sequence)
}

func (db *DB) GetDeployment(ctx context.Context, siteID string) (*Deployment, error) {
	var d Deployment
	err := db.GetContext(ctx, &d, `SELECT `+deploymentColumns+` FROM deployments WHERE site_id=$1`, siteID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}

// Keep the receipt/tombstone for enrolled sites: deleting it would let delayed
// requests recreate published content. Serialize first enrollment with deletion.
func (db *DB) DeleteUndeployedSite(ctx context.Context, siteID string, remove func() error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err = tx.GetContext(ctx, &id, `SELECT id FROM sites WHERE id=$1 FOR UPDATE`, siteID); err != nil {
		return err
	}
	var exists bool
	if err = tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM deployments WHERE site_id=$1)`, siteID); err != nil {
		return err
	}
	if exists {
		return ErrDeploymentBusy
	}
	if err = remove(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sites WHERE id=$1`, siteID); err != nil {
		return err
	}
	return tx.Commit()
}
