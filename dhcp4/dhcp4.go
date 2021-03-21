package dhcp4

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/irai/packet"
	log "github.com/sirupsen/logrus"
)

const (
	rebinding uint8 = iota
	selecting
	renewing
	rebooting
)

var (
	// Debug module variable to enable/disable debug & trace messages
	Debug bool
)

// Mode type for operational mode: Primary or Secondary server
type Mode int32

const (
	// ModePrimaryServer sets the server to operate as the single DHCP on the LAN
	ModePrimaryServer Mode = iota + 1
	// ModeSecondaryServer sets the server to operate as a secondary DHCP on the LAN; will attack the primary
	ModeSecondaryServer
	// ModeSecondaryServerNice sets the server to operate nice; i.e. will attack captured entries only
	ModeSecondaryServerNice
)

// CLoudFlare family
// https://developers.cloudflare.com/1.1.1.1/1.1.1.1-for-families
var (
	CloudFlareDNS1       = net.IPv4(1, 1, 1, 2) // malware
	CloudFlareDNS2       = net.IPv4(1, 0, 0, 2) // malware
	CloudFlareFamilyDNS1 = net.IPv4(1, 1, 1, 3) // malware and adult sites
	CloudFlareFamilyDNS2 = net.IPv4(1, 0, 0, 3) // malware and adult sites

	// OpenDNS
	OpenDNS1 = net.IPv4(208, 67, 222, 123)
	OpenDNS2 = net.IPv4(208, 67, 220, 123)
)

// Config contains configuration overrides
type Config struct {
	ClientConn net.PacketConn
}

var _ packet.PacketProcessor = &Handler{}

// Start implements PacketProcessor interface
func (h *Handler) Start() error {
	return nil
}

// Stop implements PacketProcessor interface
func (h *Handler) Stop() error { return nil }

// MinuteTicker implements packet processor interface
func (h *Handler) MinuteTicker(now time.Time) error {
	h.Lock()
	defer h.Unlock()
	h.freeLeases(now)
	return nil
}

func configChanged(config SubnetConfig, current SubnetConfig) bool {
	if !config.LAN.IP.Equal(current.LAN.IP) ||
		!config.DefaultGW.Equal(current.DefaultGW) ||
		!config.DNSServer.Equal(current.DNSServer) ||
		!config.DHCPServer.Equal(current.DHCPServer) ||
		(config.Duration != 0 && config.Duration != current.Duration) ||
		(config.FirstIP != nil && !config.FirstIP.Equal(current.FirstIP)) ||
		(config.LastIP != nil && !config.LastIP.Equal(current.LastIP)) {
		log.Infof("dhcp4: config parameters changed  config=%+v", config)
		log.Infof("dhcp4: config parameters changed current=%+v", current)
		return true
	}
	return false
}

// Handler is the main dhcp4 handler
type Handler struct {
	engine *packet.Handler // engine handler
	// clientConn net.PacketConn  // Listen DHCP client port
	mode      Mode        // if true, force decline and release packets to homeDHCPServer
	filename  string      // leases filename
	closed    bool        // indicates that detach function was called
	closeChan chan bool   // channel to close underlying goroutines
	Table     leaseTable  // lease table
	net1      *dhcpSubnet // home LAN
	net2      *dhcpSubnet // netfilter LAN
	sync.Mutex
}

// Attach return a dhcp handler with two internal subnets.
// func New(home SubnetConfig, netfilter SubnetConfig, filename string) (handler *DHCPHandler, err error) {
func Attach(engine *packet.Handler, netfilterIP net.IPNet, dnsServer net.IP, filename string) (handler *Handler, err error) {
	return Config{}.Attach(engine, netfilterIP, dnsServer, filename)
}

// Attach accepts a configuration structure and return a dhcp handler
func (config Config) Attach(engine *packet.Handler, netfilterIP net.IPNet, dnsServer net.IP, filename string) (h *Handler, err error) {

	// validate networks
	if !engine.NICInfo.HomeLAN4.Contains(netfilterIP.IP) || netfilterIP.Contains(engine.NICInfo.HomeLAN4.IP) {
		return nil, packet.ErrInvalidIP
	}

	h = &Handler{Table: map[string]*Lease{}}
	// handler.captureTable = make(map[string]bool)
	h.filename = filename
	h.mode = ModeSecondaryServerNice
	h.closeChan = make(chan bool) // go routines listen on this for closure

	if dnsServer == nil {
		dnsServer = engine.NICInfo.RouterIP4.IP
	}
	// Segment network
	homeSubnet := SubnetConfig{
		LAN:        engine.NICInfo.HomeLAN4,
		DefaultGW:  engine.NICInfo.RouterIP4.IP.To4(),
		DHCPServer: engine.NICInfo.HostIP4.IP.To4(),
		DNSServer:  dnsServer.To4(),
		// FirstIP:    net.ParseIP("192.168.0.10"),
		// LastIP:     net.ParseIP("192.168.0.127"),
	}
	netfilterSubnet := SubnetConfig{
		LAN:        net.IPNet{IP: netfilterIP.IP.Mask(netfilterIP.Mask), Mask: netfilterIP.Mask},
		DefaultGW:  netfilterIP.IP.To4(),
		DHCPServer: engine.NICInfo.HostIP4.IP,
		DNSServer:  CloudFlareFamilyDNS1,
		// FirstIP:    net.ParseIP("192.168.0.10"),
		// LastIP:     net.ParseIP("192.168.0.127"),
	}

	// Reset leases if error or config has changed
	h.net1, h.net2, h.Table, err = loadConfig(h.filename)
	if err != nil || h.net1 == nil || h.net2 == nil || h.Table == nil ||
		configChanged(homeSubnet, h.net1.SubnetConfig) || configChanged(netfilterSubnet, h.net2.SubnetConfig) {
		fmt.Printf("dhcp4: invalid or missing config file=%s. resetting...\n", h.filename)
		h.Table = make(map[string]*Lease)

		// net1 is home LAN
		h.net1, err = newSubnet(homeSubnet)
		if err != nil {
			return nil, fmt.Errorf("home config : %w", err)
		}
		h.net1.Stage = packet.StageNormal

		// net2 is netfilter LAN
		h.net2, err = newSubnet(netfilterSubnet)
		if err != nil {
			return nil, fmt.Errorf("netfilter config : %w", err)
		}
		h.net2.Stage = packet.StageRedirected
	}

	// Add static and classless route options
	h.net2.appendRouteOptions(h.net1.DefaultGW, h.net1.LAN.Mask, h.net2.DefaultGW)

	// Client port 68: used by dhcp client to listen for dhcp packets
	// Accept incoming both broadcast and localaddr packets
	/**
	h.clientConn = config.ClientConn
	if h.clientConn == nil {
		h.clientConn, err = net.ListenPacket("udp4", ":68")
		if err != nil {
			return nil, fmt.Errorf("port 68 listen error: %w ", err)
		}
	}
	*/

	if debugging() {
		log.WithFields(log.Fields{"netfilterLAN": h.net2.LAN.String(), "netfilterGW": h.net2.DefaultGW, "firstIP": h.net2.FirstIP,
			"lastIP": h.net2.LastIP}).Debug("dhcp4: Server Config")
	}

	h.engine = engine
	engine.HandlerDHCP4 = h

	h.saveConfig(h.filename)
	return h, nil
}

// Detach implements the PacketProcessor interface
func (h *Handler) Detach() error {
	h.engine.HandlerDHCP4 = packet.PacketNOOP{}
	h.closed = true
	close(h.closeChan)
	// if h.clientConn != nil {
	// h.clientConn.Close() // kill client goroutine
	// }
	return nil
}

// Mode return the disrupt flag
// if true we are sending fake decline, release and discover packets
func (h *Handler) Mode() Mode {
	return h.mode
}

// SetMode set to true to disrupt the home lan server
// with fake decline, release and discover packets
func (h *Handler) SetMode(mode Mode) {
	h.mode = mode
}

// PrintTable is a helper function to print the table to stdout
func (h *Handler) PrintTable() {
	h.Lock()
	defer h.Unlock()
	h.printTable()
}

func (h *Handler) printTable() {
	for _, v := range h.Table {
		fmt.Printf("dhcp4 : %v\n", v)
	}
}

// StartHunt will start the process to capture the client MAC
func (h *Handler) StartHunt(ip net.IP) error {
	h.engine.RLock()
	host := h.engine.FindIPNoLock(ip)
	if host == nil {
		h.engine.RUnlock()
		return packet.ErrInvalidIP
	}
	h.engine.RUnlock()

	if Debug {
		fmt.Printf("dhcp4: start hunt ip=%s\n", ip)
	}

	h.Lock()
	defer h.Unlock()

	if lease := h.findByIP(ip); lease != nil && lease.subnet.Stage != packet.StageRedirected {
		// Fake a dhcp release so router will force the client to discover when it attempts to reconnect
		if h.mode == ModeSecondaryServer || h.mode == ModeSecondaryServerNice {
			if Debug {
				fmt.Printf("dhcp4: send dhcp release to server clientID=%s ip=%s\n", lease.ClientID, ip)
			}
			h.forceRelease(lease.ClientID, h.net1.DefaultGW, lease.Addr.MAC, lease.Addr.IP, nil)
		}
		// h.engine.SetIP4Offer(host, net.IPv4zero)
	}
	return nil
}

// StopHunt will end the capture process
func (h *Handler) StopHunt(ip net.IP) error {
	// func (h *HandlerNew) Release(mac net.HardwareAddr) error {
	if Debug {
		fmt.Printf("dhcp4: stop hunt ip%s\n", ip)
	}

	// h.mutex.Lock()
	// defer h.mutex.Unlock()
	// if e := h.net2.findMAC(mac); e != nil {
	// freeLease(e)
	// }
	return nil
}

// HuntStage returns StageHunt if mac and ip are valid DHCP entry in the capture state.
// Otherwise returns false.
func (h *Handler) HuntStage(addr packet.Addr) packet.HuntStage {
	h.Lock()
	defer h.Unlock()

	lease := h.findByIP(addr.IP)
	fmt.Println("DEBUG log dhcp4 HuntStage ", lease)

	if lease != nil && lease.State == StateAllocated {
		return lease.subnet.Stage
	}
	return packet.StageNormal
}

// ProcessPacket implements PacketProcessor interface
func (h *Handler) ProcessPacket(host *packet.Host, b []byte) (*packet.Host, error) {
	ether := packet.Ether(b)
	ip4 := packet.IP4(ether.Payload())
	if !ip4.IsValid() {
		return host, packet.ErrInvalidIP
	}
	udp := packet.UDP(ip4.Payload())
	if !udp.IsValid() || len(udp.Payload()) < 240 {
		return host, packet.ErrInvalidIP
	}

	dhcpFrame := DHCP4(udp.Payload())
	if !dhcpFrame.IsValid() {
		return host, packet.ErrParseMessage
	}
	if Debug {
		fmt.Printf("ether: %s\n", ether)
		fmt.Printf("ip4  : %s\n", ip4)
		fmt.Printf("udp  : %s\n", udp)
		fmt.Printf("dhcp4: %s\n", dhcpFrame)
	}

	if udp.DstPort() == packet.DHCP4ClientPort {
		err := h.processClientPacket(host, dhcpFrame)
		return host, err
	}

	options := dhcpFrame.ParseOptions()
	var reqType MessageType
	if t := options[OptionDHCPMessageType]; len(t) != 1 {
		log.Warn("dhcp4: skiping dhcp packet with len not 1")
		return host, packet.ErrParseMessage
	} else {
		reqType = MessageType(t[0])
		if reqType < Discover || reqType > Inform {
			log.Warn("dhcp4: skiping dhcp packet invalid type ", reqType)
			return host, packet.ErrParseMessage
		}
	}

	// retrieve the sender IP address
	// ipStr , portStr, err := net.SplitHostPort(addr.String())

	// if res := h.processDHCP(req, reqType, options, ip4.Src()); res != nil {
	var response DHCP4

	h.Lock()

	switch reqType {
	case Discover:
		response = h.handleDiscover(dhcpFrame, options)

	case Request:
		// var senderIP net.IP
		// if tmp, ok := options[OptionDefaultFingerServer]; ok {
		// senderIP = net.IP(tmp)
		// }
		host, response = h.handleRequest(host, dhcpFrame, options, ip4.Src())

	case Decline:
		response = h.handleDecline(dhcpFrame, options)

	case Release:
		response = h.handleRelease(dhcpFrame, options)

	case Offer:
		log.Error("dhcp4: got dhcp offer")

	default:
		log.Warnf("dhcp4: message type not supported %v", reqType)
	}

	h.Unlock()

	if response != nil {
		// If IP not available, broadcast

		var dstAddr packet.Addr
		if ip4.Src().Equal(net.IPv4zero) || dhcpFrame.Broadcast() {
			dstAddr = packet.Addr{MAC: packet.EthBroadcast, IP: net.IPv4bcast, Port: packet.DHCP4ClientPort}
		} else {
			dstAddr = packet.Addr{MAC: ether.Src(), IP: ip4.Src(), Port: packet.DHCP4ClientPort}
		}

		if debugging() {
			log.Trace("dhcp4: send reply to ", dstAddr)
		}

		srcAddr := packet.Addr{MAC: h.engine.NICInfo.HostMAC, IP: h.engine.NICInfo.HostIP4.IP, Port: packet.DHCP4ServerPort}
		if err := sendDHCP4Packet(h.engine.Conn(), srcAddr, dstAddr, response); err != nil {
			fmt.Printf("dhcp4: failed sending packet error=%s", err)
			return host, err
		}
	}
	return host, nil
}

func getClientID(p DHCP4, options Options) []byte {
	clientID, ok := options[OptionClientIdentifier]
	if !ok {
		clientID = p.CHAddr()
	}
	return clientID
}
