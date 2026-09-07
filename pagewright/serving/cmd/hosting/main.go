package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/nginx"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/supervisor"
)

func main() {
	cfg := config.LoadConfig()
	if err := cfg.ValidatePaths(); err != nil {
		log.Fatal(err)
	}
	writer, err := nginx.AcquireWriter(cfg.NginxSitesEnabled)
	if err != nil {
		log.Fatal(err)
	}
	defer writer.Close()
	if err := nginx.PrepareRuntime(cfg.NginxSitesEnabled); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	command := exec.CommandContext(ctx, "nginx", "-t")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	err = command.Run()
	cancel()
	if err != nil {
		log.Fatal("nginx startup validation failed: ", err)
	}
	shutdown, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := supervisor.Run(shutdown, [][]string{{"nginx", "-g", "daemon off;"}, {"/app/serving-runner"}}); err != nil {
		log.Fatal(err)
	}
}
