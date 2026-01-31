package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/cozyGarage/bbscope/v2/cmd"
	"github.com/sirupsen/logrus"
)

func main() {
	// Global panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Log the panic with stack trace for debugging
			logrus.Errorf("Fatal error: %v", r)
			if logrus.GetLevel() >= logrus.DebugLevel {
				logrus.Debugf("Stack trace:\n%s", debug.Stack())
			} else {
				fmt.Fprintf(os.Stderr, "An unexpected error occurred. Run with --debug for more details.\n")
			}
			os.Exit(1)
		}
	}()

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
