package lacp

import (
	"github.com/vishvananda/netlink"
)

// LACP flag constants
const (
	Activity = 1 << iota
	Timeout
	Aggregation
	Synchronization
	Collecting
	Distributing
	Defaulted
	Expired
)

type flags uint8

// isOperational inspects lacp flags to determine if protocol is up.
func (a flags) isOperational() bool {
	if (a & (Expired | Defaulted)) != 0 {
		return false
	}

	if (a & (Distributing | Collecting | Synchronization | Aggregation)) != 60 {
		return false
	}

	return true
}

// IsFastRate indicates if the partner is using lacp fast rate.
func IsFastRate(slave *netlink.BondSlave) bool {
	p := flags(slave.AdPartnerOperPortState)

	return (p & Timeout) != 0
}

// isProtocolUp returns lacp operational status.
func isProtocolUp(slave *netlink.BondSlave) bool {
	actor := flags(slave.AdActorOperPortState)
	partner := flags(slave.AdPartnerOperPortState)

	return actor.isOperational() && partner.isOperational()
}
