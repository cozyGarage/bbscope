package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cozyGarage/bbscope/v2/cmd"
)

func main() {
	// Create a context that cancels on interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	cmd.ExecuteContext(ctx)
}
