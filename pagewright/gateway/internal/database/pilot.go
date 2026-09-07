package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrPilotLimit = errors.New("pilot limit reached")

type PilotLimits struct{ UserDaily, SiteDaily, Active int }

// AdmitPilot serializes admission across processes before any LLM calls. Failed
// and unclear attempts consume quota; exact retries do not consume another slot.
// A crashed preparing attempt remains busy (fail closed, operator recovery).
func (db *DB) AdmitPilot(ctx context.Context, owner, site, key, hash string, limits PilotLimits) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(741001)`); err != nil {
		return err
	}
	var prior struct {
		Hash string `db:"request_hash"`
		Busy bool   `db:"busy"`
	}
	err = tx.GetContext(ctx, &prior, `SELECT request_hash,busy FROM pilot_attempts WHERE owner_id=$1 AND site_id=$2 AND request_key=$3`, owner, site, key)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if exists && prior.Hash != hash {
		return ErrSubmissionConflict
	}
	if exists && prior.Busy {
		return ErrPilotLimit
	}
	var global, userActive, userDaily, siteDaily int
	if err = tx.GetContext(ctx, &global, `SELECT (SELECT count(*) FROM pilot_attempts WHERE busy)+(SELECT count(*) FROM build_submissions WHERE status IN ('pending','running'))`); err != nil {
		return err
	}
	if err = tx.GetContext(ctx, &userActive, `SELECT (SELECT count(*) FROM pilot_attempts WHERE busy AND owner_id=$1::text)+(SELECT count(*) FROM build_submissions WHERE owner_id=$1::uuid AND status IN ('pending','running'))`, owner); err != nil {
		return err
	}
	if global >= limits.Active || userActive >= 1 {
		return ErrPilotLimit
	}
	if !exists {
		if err = tx.GetContext(ctx, &userDaily, `SELECT count(*) FROM pilot_attempts WHERE owner_id=$1 AND created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`, owner); err != nil {
			return err
		}
		if err = tx.GetContext(ctx, &siteDaily, `SELECT count(*) FROM pilot_attempts WHERE site_id=$1 AND created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`, site); err != nil {
			return err
		}
		if userDaily >= limits.UserDaily || siteDaily >= limits.SiteDaily {
			return ErrPilotLimit
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO pilot_attempts(owner_id,site_id,request_key,request_hash) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,site_id,request_key) DO UPDATE SET busy=true`, owner, site, key, hash)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ReleasePilot(ctx context.Context, owner, site, key string) error {
	_, err := db.ExecContext(ctx, `UPDATE pilot_attempts SET busy=false WHERE owner_id=$1 AND site_id=$2 AND request_key=$3`, owner, site, key)
	return err
}

// Fixed minute windows use database time and hashed, bounded identity keys.
func (db *DB) TakePilotRate(ctx context.Context, key string, limit int) error {
	var count int
	err := db.GetContext(ctx, &count, `INSERT INTO pilot_rates(key,window_start,count) VALUES($1,date_trunc('minute',now()),1)
 ON CONFLICT(key) DO UPDATE SET window_start=excluded.window_start,count=CASE WHEN pilot_rates.window_start=excluded.window_start THEN pilot_rates.count+1 ELSE 1 END
 WHERE pilot_rates.window_start<>excluded.window_start OR pilot_rates.count<$2 RETURNING count`, key, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPilotLimit
	}
	return err
}

// Each provider attempt burns a conservative $1 reservation BEFORE network I/O.
// Restarts, errors, missing usage and cancellations never refund that allowance.
func (db *DB) ReservePilotProvider(ctx context.Context, id string, capCents, concurrency int) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(741002)`); err != nil {
		return err
	}
	var used, active int
	if err = tx.GetContext(ctx, &used, `SELECT coalesce(sum(reserved_cents),0) FROM pilot_provider_reservations`); err != nil {
		return err
	}
	if err = tx.GetContext(ctx, &active, `SELECT count(*) FROM pilot_provider_reservations WHERE active`); err != nil {
		return err
	}
	if capCents-used < 100 || active >= concurrency {
		return ErrPilotLimit
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO pilot_provider_reservations(id,reserved_cents) VALUES($1,100)`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (db *DB) FinishPilotProvider(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `UPDATE pilot_provider_reservations SET active=false WHERE id=$1`, id)
	return err
}

func (db *DB) PrunePilotRates(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `DELETE FROM pilot_rates WHERE window_start < now()-interval '2 minutes'`)
	return err
}

// Release operations must not inherit an already canceled browser request.
func PilotCleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
