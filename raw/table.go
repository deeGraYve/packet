package raw

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// HostTable manages host entries
type HostTable struct {
	Table map[string]*Host
	sync.Mutex
}

// Host holds a host identification
type Host struct {
	MAC        net.HardwareAddr
	IP         net.IP
	Online     bool
	IPV6Router bool
	LastSeen   time.Time
	ICMP4      interface{}
	DHCP4      interface{}
	ICMP6      interface{}
	DHCP6      interface{}
	MDNS       interface{}
	NBNS       interface{}
	ARP        interface{}
}

// New returns a HostTable handler
func New() *HostTable {
	return &HostTable{Table: make(map[string]*Host, 64)}
}

func (h *HostTable) Len() int {
	h.Lock()
	defer h.Unlock()

	return len(h.Table)
}

// PrintTable logs ICMP6 tables to standard out
func (h *HostTable) PrintTable() {
	h.Lock()
	defer h.Unlock()

	if len(h.Table) > 0 {
		fmt.Printf("icmp6 hosts table len=%v\n", len(h.Table))
		for _, v := range h.Table {
			fmt.Printf("mac=%s ip=%v online=%v \n", v.MAC, v.IP, v.Online)
		}
	}
}

func (h *HostTable) FindOrCreateHost(mac net.HardwareAddr, ip net.IP) (host *Host, found bool) {
	h.Lock()
	defer h.Unlock()

	return h.findOrCreateHost(mac, ip)
}

func (h *HostTable) findOrCreateHost(mac net.HardwareAddr, ip net.IP) (host *Host, found bool) {
	if host, ok := h.Table[string(ip)]; ok {
		host.LastSeen = time.Now()
		host.Online = true
		return host, true
	}
	host = &Host{MAC: CopyMAC(mac), IP: CopyIP(ip), LastSeen: time.Now(), Online: true}
	h.Table[string(host.IP)] = host
	return host, false
}

func (h *HostTable) FindIP(ip net.IP) *Host {
	h.Lock()
	defer h.Unlock()

	return h.Table[string(ip)]
}