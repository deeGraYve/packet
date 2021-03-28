package packet

import (
	"net"
	"time"
)

type Notification struct {
	Addr     Addr
	Online   bool
	DHCPName string
	MDNSName string
}

// purge is called each minute by the minute goroutine
func (h *Handler) purge(now time.Time, probeDur time.Duration, offlineDur time.Duration, purgeDur time.Duration) error {

	probeCutoff := now.Add(probeDur * -1)     // Mark offline entries last updated before this time
	offlineCutoff := now.Add(offlineDur * -1) // Mark offline entries last updated before this time
	deleteCutoff := now.Add(purgeDur * -1)    // Delete entries that have not responded in last hour

	purge := make([]net.IP, 0, 16)
	offline := make([]*Host, 0, 16)

	h.mutex.RLock()
	for _, e := range h.LANHosts.Table {
		e.Row.RLock()

		// Delete from table if the device is offline and was not seen for the last hour
		if !e.Online && e.LastSeen.Before(deleteCutoff) {
			purge = append(purge, e.IP)
			e.Row.RUnlock()
			continue
		}

		// Probe if device not seen recently
		if e.Online && e.LastSeen.Before(probeCutoff) {
			h.HandlerARP.CheckAddr(Addr{MAC: e.MACEntry.MAC, IP: e.IP})
		}

		// Set offline if no updates since the offline deadline
		if e.Online && e.LastSeen.Before(offlineCutoff) {
			offline = append(offline, e)
		}
		e.Row.RUnlock()
	}
	h.mutex.RUnlock()

	for _, host := range offline {
		h.lockAndSetOffline(host) // will lock/unlock row
	}

	// delete after loop because this will change the table
	if len(purge) > 0 {
		for _, v := range purge {
			h.deleteHostWithLock(v)
		}
	}

	return nil
}
