//go:build integration

package database

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestPilotAdmissionRaceQuotaReplayAndRestart(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	limits := PilotLimits{UserDaily: 2, SiteDaily: 1, Active: 2}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.AdmitPilot(ctx, p.OwnerID, p.SiteID, p.RequestKey, p.RequestHash, limits)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrPilotLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("admitted %d", admitted.Load())
	}
	// New DB wrapper represents a restarted client against the same durable state.
	restarted := &DB{db.DB}
	if err := restarted.AdmitPilot(ctx, p.OwnerID, p.SiteID, uuid.NewString(), p.RequestHash, limits); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("restart bypass: %v", err)
	}
	if err := db.ReleasePilot(ctx, p.OwnerID, p.SiteID, p.RequestKey); err != nil {
		t.Fatal(err)
	}
	if err := db.AdmitPilot(ctx, p.OwnerID, p.SiteID, p.RequestKey, "different", limits); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatal(err)
	}
	if err := db.AdmitPilot(ctx, p.OwnerID, p.SiteID, p.RequestKey, p.RequestHash, limits); err != nil {
		t.Fatal(err)
	}
	if err := db.ReleasePilot(ctx, p.OwnerID, p.SiteID, p.RequestKey); err != nil {
		t.Fatal(err)
	}
	if err := db.AdmitPilot(ctx, p.OwnerID, p.SiteID, uuid.NewString(), p.RequestHash, limits); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("site quota bypass: %v", err)
	}
	// A distinct site can consume this owner's second attempt, not a third.
	other := uuid.NewString()
	key := uuid.NewString()
	if err := db.AdmitPilot(ctx, p.OwnerID, other, key, p.RequestHash, limits); err != nil {
		t.Fatal(err)
	}
	if err := db.ReleasePilot(ctx, p.OwnerID, other, key); err != nil {
		t.Fatal(err)
	}
	if err := db.AdmitPilot(ctx, p.OwnerID, uuid.NewString(), uuid.NewString(), p.RequestHash, limits); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("user quota bypass: %v", err)
	}
}

func TestPilotCountsCommittedAndUncertainBuilds(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	if _, _, err := db.ReserveBuildSubmission(ctx, p); err != nil {
		t.Fatal(err)
	}
	limits := PilotLimits{UserDaily: 10, SiteDaily: 5, Active: 1}
	if err := db.AdmitPilot(ctx, uuid.NewString(), uuid.NewString(), uuid.NewString(), "hash", limits); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("active build bypass: %v", err)
	}
	if _, err := db.ClaimBuildDispatch(ctx, p.JobID); err != nil {
		t.Fatal(err)
	}
	if err := db.AdmitPilot(ctx, p.OwnerID, p.SiteID, uuid.NewString(), "hash", limits); !errors.Is(err, ErrPilotLimit) {
		t.Fatal(err)
	}
}

func TestPilotProviderPrepaidReservationRaceAndRestart(t *testing.T) {
	db := migrationDB(t)
	migrate(t, db)
	ctx := context.Background()
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := uuid.NewString()
			err := db.ReservePilotProvider(ctx, id, 300, 16)
			if err == nil {
				count.Add(1)
				if err := db.FinishPilotProvider(ctx, id); err != nil {
					t.Error(err)
				}
			} else if !errors.Is(err, ErrPilotLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 3 {
		t.Fatalf("budget admitted %d", count.Load())
	}
	if err := (&DB{db.DB}).ReservePilotProvider(ctx, uuid.NewString(), 300, 16); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("restart refunded: %v", err)
	}
	// Increasing the allowance grants only the increment, never resets history.
	id := uuid.NewString()
	if err := db.ReservePilotProvider(ctx, id, 500, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.ReservePilotProvider(ctx, uuid.NewString(), 500, 1); !errors.Is(err, ErrPilotLimit) {
		t.Fatalf("concurrency bypass: %v", err)
	}
	if err := db.ReservePilotProvider(ctx, uuid.NewString(), 0, 16); !errors.Is(err, ErrPilotLimit) {
		t.Fatal(err)
	}
}

func TestPilotRateDurableAtomicWindow(t *testing.T) {
	db := migrationDB(t)
	migrate(t, db)
	ctx := context.Background()
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.TakePilotRate(ctx, "test", 5)
			if err == nil {
				count.Add(1)
			} else if !errors.Is(err, ErrPilotLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 5 {
		t.Fatalf("rate admitted %d", count.Load())
	}
	if err := (&DB{db.DB}).TakePilotRate(ctx, "test", 5); !errors.Is(err, ErrPilotLimit) {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE pilot_rates SET window_start=now()-interval '3 minutes' WHERE key='test'`); err != nil {
		t.Fatal(err)
	}
	if err := db.TakePilotRate(ctx, "test", 5); err != nil {
		t.Fatal(err)
	}
}
