package dispatcher

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
)

type Dispatcher struct {
	Queue          queue.DispatchBackend
	Locks          lock.Manager
	Spawner        spawner.Spawner
	ManagerURL     string
	Limit          int
	Lease, LockTTL time.Duration
}

// Run stops claiming on cancellation and waits for bounded in-flight launches.
// Redis active slots count whole jobs, not just concurrent Spawn calls.
func (d *Dispatcher) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for i := 0; i < d.Limit; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				operation, cancel := context.WithTimeout(ctx, 5*time.Second)
				claim, err := d.Queue.Claim(operation, d.Limit, d.Lease)
				cancel()
				if err != nil {
					log.Printf("Dispatch claim unavailable")
				}
				if claim != nil && err == nil {
					d.Process(ctx, claim)
				}
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	workers.Wait()
}

func (d *Dispatcher) Process(ctx context.Context, c *queue.Claim) {
	preparation, cancel := context.WithTimeout(ctx, 5*time.Second)
	token, fence, err := d.Locks.Acquire(preparation, c.Job.SiteID, d.LockTTL)
	if err != nil {
		cancel()
		return
	} // Claim expires/requeues; no launch was attempted.
	job, err := d.Queue.BeginDispatch(preparation, c, token, fence)
	cancel()
	if err != nil {
		// A lost Begin acknowledgement may have recorded intent. Never Spawn unless
		// it was positively acknowledged; never reset that intent to pending.
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = d.Locks.Release(cleanup, c.Job.SiteID, token)
		return
	}
	launch, stop := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	id, spawnErr := d.Spawner.Spawn(launch, job, d.ManagerURL)
	stop()
	outcome := "started"
	if errors.Is(spawnErr, spawner.ErrNotStarted) {
		outcome = "not_started"
	} else if spawnErr != nil {
		outcome = "uncertain"
	}
	persist, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	if err := d.Queue.ResolveDispatch(persist, c, id, outcome); err != nil {
		log.Printf("Dispatch outcome not confirmed for job %s", job.JobID)
		return
	}
	if outcome == "not_started" {
		_ = d.Locks.Release(persist, job.SiteID, token)
	}
}
