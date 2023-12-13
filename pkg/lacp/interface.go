package lacp

import (
	"context"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/mlguerrero12/pf-status-relay/pkg/log"
)

// Interfaces stores the PFs that are inspected.
type Interfaces struct {
	PFs map[int]*PF
}

// New returns a Interfaces structure with interfaces that are found in the node.
func New(nics []string, pollingInterval int) Interfaces {
	i := Interfaces{PFs: make(map[int]*PF)}
	for _, name := range nics {
		link, err := netlink.LinkByName(name)
		if err != nil {
			log.Log.Warn("failed to fetch interface", "interface", name, "error", err)
			continue
		}

		log.Log.Debug("adding interface", "interface", name)

		i.PFs[link.Attrs().Index] = &PF{
			Name:        link.Attrs().Name,
			Index:       link.Attrs().Index,
			OperState:   link.Attrs().OperState,
			MasterIndex: link.Attrs().MasterIndex,

			pollingInterval: pollingInterval,
		}
	}

	return i
}

// Start starts LACP inspection and processing.
func (i *Interfaces) Start(ctx context.Context, queue <-chan int, wg *sync.WaitGroup) {
	log.Log.Debug("LACP inspection and processing started")

	// Verify that PFs are ready to accept/receive LACPDU messages.
	for _, p := range i.PFs {
		err := p.Inspect()
		if err != nil {
			log.Log.Error("interface not ready", "interface", p.Name, "error", err)
			continue
		}

		p.StartMonitoring(ctx)
	}

	// Process link changes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case index := <-queue:
				log.Log.Debug("processing event", "index", index)
				p := i.PFs[index]
				updated, err := p.Update()
				if err != nil {
					log.Log.Error("failed to update link", "interface", p.Name)
					break
				}

				if updated {
					err = p.Inspect()
					if err != nil {
						log.Log.Error("interface not ready", "interface", p.Name, "error", err)
						p.StopMonitoring()
					} else {
						p.StartMonitoring(ctx)
					}
				}
			case <-ctx.Done():
				log.Log.Debug("ctx cancelled", "routine", "interfaces")
				for _, p := range i.PFs {
					p.StopMonitoring()
				}
				return
			}
		}
	}()
}

// Indexes returns a list of indexes.
func (i *Interfaces) Indexes() []int {
	indexes := make([]int, 0, len(i.PFs))
	for index := range i.PFs {
		indexes = append(indexes, index)
	}

	return indexes
}
