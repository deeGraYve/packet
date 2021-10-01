package dns

import (
	"errors"
	"fmt"

	"github.com/irai/packet"
	"github.com/irai/packet/fastlog"
	"inet.af/netaddr"
)

func (h *DNSHandler) PrintDNSTable() {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	fmt.Printf("dns table len=%d\n", len(h.DNSTable))
	for _, v := range h.DNSTable {
		fastlog.NewLine(module, "entry").Struct(v).Write()
	}
}

func (h *DNSHandler) DNSExist(ip netaddr.IP) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for _, entry := range h.DNSTable {
		if _, found := entry.IP4Records[ip]; found {
			return true
		}
	}
	return false
}

func (h *DNSHandler) DNSFind(name string) DNSEntry {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	if e, found := h.DNSTable[name]; found {
		return e.copy()
	}
	return DNSEntry{}
}

func (h *DNSHandler) DNSLookupPTR(ip netaddr.IP) {
	if err := ReverseDNS(ip); errors.Is(err, packet.ErrNotFound) {
		h.mutex.Lock()
		defer h.mutex.Unlock()

		// cache IPs that do not have a PTR RR to prevent unnecessary lookups;
		// it is likely the same IP will be used again and again.
		// TODO: should we block unknown IPs?
		entry, found := h.DNSTable["ptrentryname"]
		if !found {
			entry = newDNSEntry()
			entry.Name = "ptrentryname"
			h.DNSTable["ptrentryname"] = entry
		}

		// IPv4?
		if ip.Is4() {
			_, found = entry.IP4Records[ip]
			if !found {
				if Debug {
					fmt.Printf("dns   : add ptr record not found for ip=%s\n", ip)
				}
				entry.IP4Records[ip] = IPResourceRecord{IP: ip}
			}
			return
		}

		// IPv6?
		_, found = entry.IP6Records[ip]
		if !found {
			if Debug {
				fmt.Printf("dns   : ptr record not found for ip=%s\n", ip)
			}
			entry.IP4Records[ip] = IPResourceRecord{IP: ip}
		}
	}
}
