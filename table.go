package packet

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"inet.af/netaddr"
)

// HostTable manages host entries
type HostTable struct {
	Table map[netaddr.IP]*Host
}

// HuntStage holds the host hunt stage
type HuntStage byte

// possible hunt stages
const (
	StageNoChange   HuntStage = 0 // no change to stage - used as no op
	StageNormal     HuntStage = 1 // not captured
	StageHunt       HuntStage = 2 // host is not redirected
	StageRedirected HuntStage = 3 // host is not redirected
)

func (s HuntStage) String() string {
	if s == StageNormal {
		return "normal"
	}
	if s == StageRedirected {
		return "redirected"
	}
	if s == StageHunt {
		return "hunt"
	}
	return "noop"
}

// DHCP4Store keeps dhcp4 specific settings
type DHCP4Store struct {
	HuntStage HuntStage // DHCP4 hunt stage
	Name      string    // DHCP4 host name
	IPOffer   net.IP    // DCHCP discover offer
}

func (e DHCP4Store) String() string {
	return fmt.Sprintf("dhcp4stage=%s name=%v ipoffer=%v", e.HuntStage, e.Name, e.IPOffer)
}

// ARPStore keeps arp specific settings
type ARPStore struct {
	HuntStage HuntStage // ARP hunt stage
}

// ICMP4Store keeps icmp4 specific settings
type ICMP4Store struct {
	HuntStage HuntStage // ARP hunt stage
}
type ICMP6Store struct {
	HuntStage HuntStage // ARP hunt stage
	Router    bool      // ICMP6 specific
}

func (e ICMP6Store) String() string {
	return fmt.Sprintf("icmp6stage=%s router=%v", e.HuntStage, e.Router)
}

// Host holds a pointer to the host record. The pointer is always valid and will be garbage collected
// when no longer in use. Host pointers are shared with all plugins.
//
// CAUTION:
// Host has a RWMutex used to sync access to the record. This must be read locked to access fields or write locked for updating
// When locking the engine, you must lock the engine first then row lock to avoid deadlocks
type Host struct {
	IP         net.IP       // either IP6 or ip4
	MACEntry   *MACEntry    // pointer to mac entry
	Online     bool         // keep host online / offline state
	huntStage  HuntStage    // keep host overall huntStage
	LastSeen   time.Time    // keep last packet time
	icmp4Store ICMP4Store   // ICMP4 private store
	arpStore   ARPStore     // ARP private store
	icmp6Store ICMP6Store   // ICMP6 private store
	dhcp4Store DHCP4Store   // DHCP4 private store
	Row        sync.RWMutex // Row level mutex
}

func (e *Host) String() string {
	return fmt.Sprintf("mac=%s ip=%v online=%v capture=%v stage4=%s lastSeen=%s", e.MACEntry.MAC, e.IP, e.Online, e.MACEntry.Captured, e.huntStage, time.Since(e.LastSeen))
}

func (host *Host) SetDHCP4StoreNoLock(h *Handler, store DHCP4Store) {
	host.MACEntry.IP4Offer = store.IPOffer
	if host.dhcp4Store.Name != store.Name {
		host.dhcp4Store.Name = store.Name
	}

	// DHCP stage overides all other stages
	if store.HuntStage != StageNoChange {
		host.dhcp4Store.HuntStage = store.HuntStage
		h.transitionHuntStage(host, store.HuntStage, StageNoChange)
	}
}

func (host *Host) GetDHCP4StoreNoLock() (store DHCP4Store) {
	store.IPOffer = host.MACEntry.IP4Offer
	store.HuntStage = host.dhcp4Store.HuntStage
	store.Name = host.dhcp4Store.Name
	return store
}

func (host *Host) SetICMP6StoreNoLock(store ICMP6Store) {
	host.icmp6Store.Router = store.Router
	host.icmp6Store.HuntStage = store.HuntStage
}

func (host *Host) GetICMP6StoreNoLock() (store ICMP6Store) {
	return host.icmp6Store
}

func (host *Host) GetARPStore() (store ARPStore) {
	return host.arpStore
}
func (host *Host) SetARPStore(store ARPStore) {
	host.arpStore = store
}

// newHostTable returns a HostTable handler
func newHostTable() HostTable {
	return HostTable{Table: make(map[netaddr.IP]*Host, 64)}
}

// PrintTable print table to standard out
func (h *Handler) printHostTable() {
	count := 0
	for _, v := range h.MACTable.Table {
		for _, host := range v.HostList {
			fmt.Println("host :", host)
			count++
		}
	}
	if count != len(h.LANHosts.Table) { // validate our logic - DELETE and replace with test in future
		panic(fmt.Sprintf("host table differ in lenght hosts=%d machosts=%d  ", len(h.LANHosts.Table), count))
	}

}

// FindOrCreateHost will create a new host entry or return existing
//
// The funcion copies both the mac and the ip; it is safe to call this with a frame.IP(), frame.MAC()
func (h *Handler) FindOrCreateHost(mac net.HardwareAddr, ip net.IP) (host *Host, found bool) {

	//optimise the common path
	ipNew, _ := netaddr.FromStdIP(ip)
	h.mutex.RLock()
	if host, ok := h.LANHosts.Table[ipNew]; ok && bytes.Equal(host.MACEntry.MAC, mac) {
		h.mutex.RUnlock()
		return host, true
	}
	h.mutex.RUnlock()

	// lock for writing
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.findOrCreateHost(mac, ip)
}

// findOrCreateHost find the host using the frame IP (avoid copy if not needed)
//
// Must have engine lock before calling
func (h *Handler) findOrCreateHost(mac net.HardwareAddr, ip net.IP) (host *Host, found bool) {

	// using netaddr IP
	ipNew, _ := netaddr.FromStdIP(ip)
	now := time.Now()
	if host, ok := h.LANHosts.Table[ipNew]; ok {
		host.Row.Lock() // lock the row
		if !bytes.Equal(host.MACEntry.MAC, mac) {
			fmt.Println("packet: error mac address differ - duplicated IP???", host.MACEntry.MAC, mac, ipNew)
			h.printHostTable()
			// link host to new macEntry
			mac := CopyMAC(mac)
			host.MACEntry.unlink(host) // remove IP from existing mac
			macEntry := h.MACTable.findOrCreate(mac)
			macEntry.link(host)
			host.MACEntry = macEntry
			host.huntStage = StageNormal
		}
		host.LastSeen = now
		host.MACEntry.LastSeen = now
		host.Row.Unlock()
		return host, true
	}
	mac = CopyMAC(mac) // copy from frame
	macEntry := h.MACTable.findOrCreate(mac)
	host = &Host{IP: CopyIP(ip), MACEntry: macEntry, Online: false} // set Online to false to trigger Online transition
	host.LastSeen = now
	host.huntStage = StageNormal
	host.MACEntry.LastSeen = now
	// host.MACEntry.updateIP(host.IP)
	h.LANHosts.Table[ipNew] = host

	// link host to macEntry
	macEntry.HostList = append(macEntry.HostList, host)
	return host, false
}

func (h *Handler) deleteHostWithLock(ip net.IP) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if host := h.findIP(ip); host != nil {
		host.MACEntry.unlink(host)
		newIP, _ := netaddr.FromStdIP(ip)
		delete(h.LANHosts.Table, newIP)
	}
}

// FindIP returns the host entry for IP or nil othewise
func (h *Handler) FindIP(ip net.IP) *Host {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	newIP, _ := netaddr.FromStdIP(ip)
	return h.LANHosts.Table[newIP]
}

// MustFindIP returns the host for IP or panic if IP is not found
func (h *Handler) MustFindIP(ip net.IP) *Host {
	if host := h.FindIP(ip); host != nil {
		return host
	}
	panic(fmt.Sprintf("MustFindIP not found ip=%s", ip))
}

// findIP finds the host for IP wihout locking the engine
// Engine must be locked prior to calling this function
func (h *Handler) findIP(ip net.IP) *Host {
	newIP, _ := netaddr.FromStdIP(ip)
	return h.LANHosts.Table[newIP]
}

// FindByMAC return a list of IP addresses for mac
func (h *Handler) FindByMAC(mac net.HardwareAddr) (list []Addr) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	for _, v := range h.LANHosts.Table {
		if bytes.Equal(v.MACEntry.MAC, mac) {
			list = append(list, Addr{MAC: v.MACEntry.MAC, IP: v.IP})
		}
	}
	return list
}

// GetTable returns a copy of the current table
func (h *Handler) GetTable() (list []Host) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	list = make([]Host, len(h.LANHosts.Table))
	for _, v := range h.LANHosts.Table {
		list = append(list, *v)
	}
	return list
}
