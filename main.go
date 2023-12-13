package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/mlguerrero12/pf-status-relay/pkg/config"
	"github.com/mlguerrero12/pf-status-relay/pkg/lacp"
	"github.com/mlguerrero12/pf-status-relay/pkg/log"
	"github.com/mlguerrero12/pf-status-relay/pkg/subscribe"
)

func main() {
	log.Log.Info("application started")

	// Capture SIGINT and SIGTERM
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Read config file.
	conf := config.ReadConfig()

	ctx, cancel := context.WithCancel(context.Background())

	// Queue to store link events.
	queue := make(chan int, 100)

	var wg sync.WaitGroup

	// Initialize PFs.
	pfs := lacp.New(conf.Interfaces, conf.PollingInterval)
	if len(pfs.PFs) == 0 {
		log.Log.Error("no interfaces found in node")
		os.Exit(1)
	}

	// Start LACP inspection and processing.
	pfs.Start(ctx, queue, &wg)

	// Subscribe to link changes.
	err := subscribe.Start(ctx, pfs.Indexes(), queue, &wg)
	if err != nil {
		log.Log.Error("failed to subscribe to link changes", "error", err)
	}

	<-c
	cancel()
	wg.Wait()
}
