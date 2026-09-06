// recovery-audit never mutates Redis. Run with writers stopped for a stable view.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/config"
	queueRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue/redis"
)

func main() {
	cfg := config.LoadConfig()
	backend, err := queueRedis.NewRedisBackend(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatal("Redis audit connection unavailable")
	}
	defer backend.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := backend.Audit(ctx)
	if err != nil {
		log.Fatalf("Audit incomplete: %v", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		log.Fatal(err)
	}
	if len(report.Issues) > 0 {
		os.Exit(2)
	}
}
