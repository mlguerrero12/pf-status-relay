package subscribe

import (
	"context"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/mlguerrero12/pf-status-relay/pkg/log"
)

// Start starts subscription to link changes.
func Start(ctx context.Context, indexes []int, queue chan<- int, wg *sync.WaitGroup) error {
	log.Log.Debug("subscribing to link changes")
	update := make(chan netlink.LinkUpdate)

	// There is another function that allows to register an error handler which might be useful to retry subscription in case of errors.
	err := netlink.LinkSubscribe(update, ctx.Done())
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case u := <-update:
				log.Log.Debug("event received", "index", u.Index)
				// Add index to the queue if there is a match.
				for _, index := range indexes {
					if int(u.Index) == index {
						log.Log.Debug("adding index to queue", "index", index)
						queue <- index
					}
				}
			case <-ctx.Done():
				log.Log.Debug("ctx cancelled", "routine", "subscribe")
				return
			}
		}
	}()

	return nil
}
