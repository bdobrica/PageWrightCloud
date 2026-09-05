package database

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/bootstrap"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

var ErrSiteConflict = errors.New("domain already reserved or incompatible template")

type SiteBootstrap struct {
	SiteID       string `db:"site_id"`
	VersionID    string `db:"version_id"`
	Archive      []byte `db:"archive"`
	Manifest     []byte `db:"manifest"`
	ExecutionLog []byte `db:"execution_log"`
}

// Persist exact upload bytes with the new site. FQDN is the stable reservation
// identity; concurrent/restarted requests read the winning owner's same bytes.
func (db *DB) ReserveSiteBootstrap(ctx context.Context, owner, fqdn string) (*types.Site, *SiteBootstrap, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	id, created := uuid.NewString(), time.Now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO sites(id,fqdn,user_id,template_id,enabled,created_at,updated_at,initialization_status)
 VALUES($1,$2,$3,'starter',false,$4,$4,'pending') ON CONFLICT(fqdn) DO NOTHING`, id, fqdn, owner, created)
	if err != nil {
		return nil, nil, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	var site types.Site
	if err := tx.GetContext(ctx, &site, `SELECT * FROM sites WHERE fqdn=$1 FOR UPDATE`, fqdn); err != nil {
		return nil, nil, err
	}
	if site.UserID != owner || site.TemplateID != "starter" || site.InitializationStatus == "legacy" {
		return nil, nil, ErrSiteConflict
	}
	if inserted == 1 {
		archive, manifest, log, err := bootstrap.Generate(id, created)
		if err != nil {
			return nil, nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO site_bootstraps(site_id,version_id,archive,manifest,execution_log) VALUES($1,$2,$3,$4,$5)`, id, bootstrap.Version, archive, manifest, log); err != nil {
			return nil, nil, err
		}
	}
	var record SiteBootstrap
	if err := tx.GetContext(ctx, &record, `SELECT * FROM site_bootstraps WHERE site_id=$1`, site.ID); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return &site, &record, nil
}

func (db *DB) CompleteSiteBootstrap(ctx context.Context, siteID string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions(site_id,build_id,status) SELECT site_id,version_id,'completed' FROM site_bootstraps WHERE site_id=$1 ON CONFLICT(site_id,build_id) DO UPDATE SET status='completed'`, siteID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sites SET initialization_status='ready',updated_at=now() WHERE id=$1 AND initialization_status IN ('pending','ready')`, siteID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("bootstrap site no longer exists")
	}
	return tx.Commit()
}
