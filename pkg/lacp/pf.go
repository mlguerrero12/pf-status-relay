package lacp

import (
	"context"
	"fmt"
	"time"

	"github.com/vishvananda/netlink"

	"github.com/mlguerrero12/pf-status-relay/pkg/log"
)

// PF contains information about the physical function as well as a context to manage lacp monitoring.
type PF struct {
	// Name is the name of the interface.
	Name string
	// Index is the index of the interface.
	Index int
	// OperState is the operational state of the interface.
	OperState netlink.LinkOperState
	// MasterIndex is the index of the bond interface.
	MasterIndex int

	Monitoring bool
	ctx        context.Context
	cancel     context.CancelFunc
	endChan    chan struct{}

	pollingInterval int
}

func (p *PF) Inspect() error {
	// Verify that link is up.
	if p.OperState != netlink.OperUp {
		return fmt.Errorf("link is not up")
	}

	// Verify that link has a master.
	if p.MasterIndex == 0 {
		return fmt.Errorf("no master interface associated")
	}

	// Verify that bond runs in mode 802.3ad.
	bond, err := netlink.LinkByIndex(p.MasterIndex)
	if err != nil {
		return fmt.Errorf("failed to fetch master interface with index %d: %w", p.MasterIndex, err)
	}

	// Verify that bond has mode 802.3ad
	if bond.(*netlink.Bond).Mode != netlink.BOND_MODE_802_3AD {
		return fmt.Errorf("bond %s does not have mode 802.3ad", bond.Attrs().Name)
	}

	return nil
}

// Update updates the info of the PF when there is an operational state change.
func (p *PF) Update() (bool, error) {
	// Fetch link again. Do not use attrs from subscribe since it might be obsolete.
	link, err := netlink.LinkByIndex(p.Index)
	if err != nil {
		return false, err
	}

	log.Log.Debug("link state", "state", link.Attrs().OperState)

	if link.Attrs().OperState == p.OperState {
		log.Log.Debug("PF was not updated", "interface", link.Attrs().Name)
		return false, nil
	}

	p.Name = link.Attrs().Name
	p.Index = link.Attrs().Index
	p.OperState = link.Attrs().OperState
	p.MasterIndex = link.Attrs().MasterIndex

	log.Log.Info("PF was updated", "interface", p.Name, "operational state", p.OperState)

	return true, nil
}

// StartMonitoring starts lacp monitoring.
func (p *PF) StartMonitoring(ctx context.Context) {
	if p.Monitoring {
		log.Log.Debug("lacp monitoring has already started", "interface", p.Name)
		return
	}

	log.Log.Info("starting lacp monitoring", "interface", p.Name)
	p.Monitoring = true

	// Context to cancel monitoring.
	stop, cancel := context.WithCancel(ctx)
	p.ctx = stop
	p.cancel = cancel
	p.endChan = make(chan struct{})

	go func() {
		defer func() {
			p.endChan <- struct{}{}
		}()

		lacpUp := false
		noVFLog := true
		firstDownLog := true
		for {
			select {
			case <-time.Tick(time.Duration(p.pollingInterval) * time.Millisecond):
				link, err := netlink.LinkByIndex(p.Index)
				if err != nil {
					log.Log.Warn("failed to fetch interface", "interface", p.Name, "error", err)
					break
				}

				// Stop if interface has no configured VFs.
				vfs := link.Attrs().Vfs
				if len(vfs) == 0 {
					if noVFLog {
						log.Log.Info("interface has no VFs", "interface", p.Name)
						noVFLog = false
					}
					break
				}
				noVFLog = true

				// Check lacp state.
				slave := link.Attrs().Slave
				if slave != nil {
					s, ok := slave.(*netlink.BondSlave)
					if !ok {
						log.Log.Error("interface does not have BondSlave type on Slave attribute", "interface", p.Name)
						break
					}

					if isProtocolUp(s) {
						if !lacpUp {
							log.Log.Info("lacp is up", "interface", p.Name)
							lacpUp = true

							if !IsFastRate(s) {
								log.Log.Warn("partner is using slow lacp rate", "interface", p.Name)
							}
						}

						// Bring to auto all VFs whose state is disable.
						for _, vf := range vfs {
							log.Log.Debug("vf info", "id", vf.ID, "state", vf.LinkState, "interface", p.Name)
							if vf.LinkState == netlink.VF_LINK_STATE_DISABLE {
								err = netlink.LinkSetVfState(link, vf.ID, netlink.VF_LINK_STATE_AUTO)
								if err != nil {
									log.Log.Error("failed to set vf link state", "id", vf.ID, "interace", p.Name, "error", err)
								}
								log.Log.Info("vf link state was set", "id", vf.ID, "state", "auto", "interface", p.Name)
							}
						}
					} else {
						if lacpUp || firstDownLog {
							log.Log.Info("lacp is down", "interface", p.Name)
							lacpUp = false
							firstDownLog = false
						}

						// Bring to disable all VFs whose state is auto.
						for _, vf := range vfs {
							log.Log.Debug("vf info", "id", vf.ID, "state", vf.LinkState, "interface", p.Name)
							if vf.LinkState == netlink.VF_LINK_STATE_AUTO {
								err = netlink.LinkSetVfState(link, vf.ID, netlink.VF_LINK_STATE_DISABLE)
								if err != nil {
									log.Log.Error("failed to set vf link state", "id", vf.ID, "interface", p.Name, "error", err)
								}
								log.Log.Info("vf link state was set", "id", vf.ID, "state", "disable", "interface", p.Name)
							}
						}
					}
				} else {
					log.Log.Error("interface has no slave attribute", "interface", p.Name)
				}
			case <-stop.Done():
				log.Log.Debug("ctx cancelled", "routine", "monitoring")
				return
			}
		}
	}()
}

// StopMonitoring stops lacp monitoring.
func (p *PF) StopMonitoring() {
	if !p.Monitoring {
		return
	}

	log.Log.Info("stopping lacp monitoring", "interface", p.Name)
	p.cancel()
	<-p.endChan

	p.Monitoring = false
}
