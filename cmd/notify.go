package cmd

import (
	"context"
	"sync"

	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/notify"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// notifyConfigKey is the top-level config block documented in the README.
const notifyConfigKey = "notifications"

// changeNotifier fans detected scope changes out to every configured notifier.
//
// Programs are polled concurrently, so Dispatch is called from several worker
// goroutines. Sends are serialized: notifier transports are not all safe for
// concurrent use, and a burst of parallel webhook posts is more likely to be
// rate-limited than a sequential one.
type changeNotifier struct {
	mu        sync.Mutex
	notifiers []notify.Notifier
}

// loadChangeNotifier builds a notifier set from the "notifications" config
// block. It returns nil when nothing is configured, which Dispatch treats as a
// no-op, so callers do not need to check.
func loadChangeNotifier() *changeNotifier {
	if !viper.IsSet(notifyConfigKey) {
		return nil
	}

	var cfg notify.Config
	if err := viper.UnmarshalKey(notifyConfigKey, &cfg); err != nil {
		utils.Log.Warnf("Could not read the %q config block, notifications are disabled: %v", notifyConfigKey, err)
		return nil
	}

	notifiers := notify.LoadNotifiers(&cfg)
	if len(notifiers) == 0 {
		return nil
	}

	names := make([]string, 0, len(notifiers))
	for _, n := range notifiers {
		names = append(names, n.Name())
	}
	utils.Log.Infof("Notifications enabled: %v", names)

	return &changeNotifier{notifiers: notifiers}
}

// Dispatch sends one event per change to every notifier. Each notifier applies
// its own event-type filter. Delivery failures are logged and skipped: a broken
// webhook must not fail the poll that produced the data.
func (c *changeNotifier) Dispatch(ctx context.Context, changes []storage.Change) {
	if c == nil || len(changes) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return
		}
		event := changeToEvent(change)
		for _, n := range c.notifiers {
			if err := n.Send(ctx, event); err != nil {
				utils.Log.Warnf("Notifier %s failed for %s %s: %v", n.Name(), event.Type, event.Target, err)
			}
		}
	}
}

// changeToEvent maps a stored change onto the notification payload, preferring
// the AI-normalized target when one exists since that is what users recognize.
func changeToEvent(change storage.Change) notify.ChangeEvent {
	target := change.TargetNormalized
	if change.TargetAINormalized != "" {
		target = change.TargetAINormalized
	}
	if target == "" {
		target = change.TargetRaw
	}

	return notify.ChangeEvent{
		Type:          change.ChangeType,
		Platform:      change.Platform,
		ProgramURL:    change.ProgramURL,
		ProgramHandle: change.Handle,
		Target:        target,
		Category:      change.Category,
		InScope:       change.InScope,
		IsBBP:         change.IsBBP,
		OccurredAt:    change.OccurredAt,
	}
}
