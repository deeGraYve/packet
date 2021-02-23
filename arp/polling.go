package arp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"log"

	"github.com/irai/packet/raw"
)

// pollingLoop detect new IPs on the network
// Send ARP request to all 255 IP addresses first time then send ARP request every so many minutes.
func (c *Handler) scanLoop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval).C
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker:
			if err := c.ScanNetwork(ctx, c.config.HomeLAN); err != nil {
				return err
			}
		}
	}
}

// Probe known macs more often in case they left the network.
func (c *Handler) probeOnlineLoop(ctx context.Context, interval time.Duration) error {
	dur := time.Second * 30
	if interval <= dur {
		dur = interval / 2
	}
	ticker := time.NewTicker(dur).C
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker:
			refreshCutoff := time.Now().Add(interval * -1)

			c.RLock()
			for _, entry := range c.table.macTable {
				if entry.State == StateVirtualHost || !entry.Online {
					continue
				}
				if entry.LastUpdated.Before(refreshCutoff) {
					for _, v := range entry.IPs() {
						if Debug {
							log.Printf("arp ip=%s online? mac=%s", v, entry.MAC)
						}
						if err := c.request(c.config.HostMAC, c.config.HostIP, entry.MAC, v); err != nil {
							log.Printf("Error ARP request mac=%s ip=%s: %s ", entry.MAC, v, err)
						}
					}
				}
			}
			c.RUnlock()

		}
	}
}

func (c *Handler) purgeLoop(ctx context.Context, offline time.Duration, purge time.Duration) error {

	dur := time.Minute * 1
	if offline <= dur {
		dur = offline / 2
	}
	ticker := time.NewTicker(dur).C
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker:

			now := time.Now()
			offlineCutoff := now.Add(offline * -1) // Mark offline entries last updated before this time
			deleteCutoff := now.Add(purge * -1)    // Delete entries that have not responded in last hour
			macs := make([]net.HardwareAddr, 0, 16)

			c.Lock()
			for _, e := range c.table.macTable {

				fmt.Println("DEBUG running purge", e, offlineCutoff, e.LastUpdated)
				// Delete from ARP table if the device was not seen for the last hour
				// This will delete Virtual hosts too
				if e.LastUpdated.Before(deleteCutoff) {
					macs = append(macs, e.MAC)
					continue
				}

				// Set offline if no updates since the offline deadline
				// Ignore virtual hosts; offline controlled by spoofing goroutine
				if e.State != StateVirtualHost && e.Online && e.LastUpdated.Before(offlineCutoff) {
					c.table.printTable()
					log.Printf("arp ip=%s offline cutoff reached mac=%s state=%s ips=%s", e.IP(), e.MAC, e.State, e.IPs())

					e.Online = false
					e.State = StateNormal // Stop hunt if in progress

					// Notify upstream the device changed to offline
					if c.notification != nil {
						c.notification <- *e
					}
				}
			}

			// delete after loop because this will change the ipTable map
			if len(macs) > 0 {
				c.table.printTable()
				for i := range macs {
					c.table.delete(macs[i])
				}
			}
			c.Unlock()
		}
	}
}

// ScanNetwork sends 256 arp requests to identify IPs on the lan
func (c *Handler) ScanNetwork(ctx context.Context, lan net.IPNet) error {

	// Copy underneath array so we can modify value.
	ip := raw.CopyIP(lan.IP)
	ip = ip.To4()
	if ip == nil {
		return raw.ErrInvalidIP4
	}

	if Debug {
		log.Printf("arp Discovering IP - sending 254 ARP requests - lan %v", lan)
	}
	for host := 1; host < 255; host++ {
		ip[3] = byte(host)

		// Don't scan router and host
		if bytes.Equal(ip, c.config.RouterIP) || bytes.Equal(ip, c.config.HostIP) {
			continue
		}

		err := c.request(c.config.HostMAC, c.config.HostIP, EthernetBroadcast, ip)
		if ctx.Err() == context.Canceled {
			return nil
		}
		if err != nil {
			if err1, ok := err.(net.Error); ok && err1.Temporary() {
				if Debug {
					log.Print("arp error in write socket is temporary - retry ", err1)
				}
				continue
			}

			if Debug {
				log.Print("arp request error ", err)
			}
			return err
		}
		time.Sleep(time.Millisecond * 25)
	}

	return nil
}
