package dhcp4_spoofer

import (
	"net"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/deeGraYve/packet"
	"github.com/deeGraYve/packet/fastlog"
)

func testRequestPacket(mt packet.DHCP4MessageType, chAddr net.HardwareAddr, cIAddr netip.Addr, xId []byte, broadcast bool, options packet.DHCP4Options) packet.DHCP4 {
	p := make(packet.DHCP4, 1024)
	return packet.EncodeDHCP4(p, packet.DHCP4BootRequest, mt, chAddr, cIAddr, packet.IPv4zero, xId, broadcast, options, options[packet.DHCP4OptionParameterRequestList])
}

func TestDHCPHandler_handleDiscover(t *testing.T) {
	options := packet.DHCP4Options{}
	options[packet.DHCP4OptionCode(packet.DHCP4OptionParameterRequestList)] = []byte{byte(packet.DHCP4OptionDomainNameServer)}

	Logger.SetLevel(fastlog.LevelError)
	os.Remove(testDHCPFilename)
	tc := setupTestHandler()
	defer tc.Close()

	tests := []struct {
		name          string
		packet        packet.DHCP4
		wantResponse  bool
		tableLen      int
		responseCount int
		srcAddr       packet.Addr
		dstAddr       packet.Addr
	}{
		{name: "discover-mac1", wantResponse: true, responseCount: 1,
			packet: testRequestPacket(packet.DHCP4Discover, mac1, ip1, []byte{0x01}, false, options), tableLen: 1,
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac1, IP: ip1, Port: packet.DHCP4ServerPort}},
		{name: "discover-mac1", wantResponse: true, responseCount: 2,
			packet: testRequestPacket(packet.DHCP4Discover, mac1, ip1, []byte{0x01}, false, options), tableLen: 1,
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac1, IP: ip1, Port: packet.DHCP4ServerPort}},
		{name: "discover-mac1", wantResponse: true, responseCount: 3,
			packet: testRequestPacket(packet.DHCP4Discover, mac1, ip1, []byte{0x02}, false, options), tableLen: 1,
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac1, IP: ip1, Port: packet.DHCP4ServerPort}},
		{name: "discover-mac1", wantResponse: true, responseCount: 4,
			packet: testRequestPacket(packet.DHCP4Discover, mac1, ip1, []byte{0x03}, false, options), tableLen: 1,
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac1, IP: ip1, Port: packet.DHCP4ServerPort}},
		{name: "discover-mac2", wantResponse: true, responseCount: 5,
			packet: testRequestPacket(packet.DHCP4Discover, mac2, ip2, []byte{0x01}, false, options), tableLen: 2,
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac2, IP: ip2, Port: packet.DHCP4ServerPort}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			ether := packet.Ether(make([]byte, packet.EthMaxSize))
			ether = packet.EncodeEther(ether, syscall.ETH_P_IP, tt.srcAddr.MAC, tt.dstAddr.MAC)
			ip4 := packet.EncodeIP4(ether.Payload(), 50, tt.srcAddr.IP, tt.dstAddr.IP)
			udp := packet.EncodeUDP(ip4.Payload(), tt.srcAddr.Port, tt.dstAddr.Port)
			udp, _ = udp.AppendPayload(tt.packet)
			ip4 = ip4.SetPayload(udp, syscall.IPPROTO_UDP)
			if ether, err = ether.SetPayload(ip4); err != nil {
				t.Fatal("error processing packet", err)
				return
			}
			frame, err := tc.session.Parse(ether)
			if err != nil {
				panic(err)
			}
			err = tc.h.ProcessPacket(frame)
			if err != nil {
				t.Errorf("DHCPHandler.handleDiscover() error sending packet error=%s", err)
				return
			}
			select {
			case p := <-tc.notifyReply:
				dhcp := packet.DHCP4(packet.UDP(packet.IP4(packet.Ether(p).Payload()).Payload()).Payload())
				options := dhcp.ParseOptions()
				if options[packet.DHCP4OptionSubnetMask] == nil || options[packet.DHCP4OptionRouter] == nil || options[packet.DHCP4OptionDomainNameServer] == nil {
					t.Fatalf("DHCPHandler.handleDiscover() missing options =%v", err)
				}
			case <-time.After(time.Millisecond * 10):
				t.Fatal("failed to receive reply")
			}

			if n := len(tc.h.table); n != tt.tableLen {
				tc.h.printTable()
				t.Errorf("DHCPHandler.handleDiscover() invalid lease table len=%d want=%d", n, tt.tableLen)
			}
			tc.Lock()
			if tt.responseCount != tc.count {
				t.Errorf("DHCPHandler.handleDiscover() invalid response count=%d want=%d", tc.count, tt.responseCount)
			}
			tc.Unlock()
		})
	}
	checkLeaseTable(t, tc, 0, 2, 0)
}

func TestDHCPHandler_handleDiscoverCaptured(t *testing.T) {
	options := packet.DHCP4Options{}
	options[packet.DHCP4OptionCode(packet.DHCP4OptionParameterRequestList)] = []byte{byte(packet.DHCP4OptionDomainNameServer)}

	// packet.DebugIP4 = true
	Logger.SetLevel(fastlog.LevelError)
	os.Remove(testDHCPFilename)
	tc := setupTestHandler()
	defer tc.Close()

	t.Run("mac1 normal", func(t *testing.T) {
		hostName := "host1"
		addr := packet.Addr{MAC: mac1}
		p := newDHCP4DiscoverFrame(addr, hostName, nil)
		frame, err := tc.session.Parse(p)
		if err != nil {
			panic(err)
		}
		err = tc.h.ProcessPacket(frame)
		if lease := tc.h.findByMAC(addr.MAC); lease == nil || lease.IPOffer != netip.MustParseAddr("192.168.0.1") || lease.Name != hostName || lease.State != StateDiscover ||
			lease.subnet != tc.h.net1 {
			tc.h.PrintTable()
			t.Error("invalid discover state lease", lease)
		}
	})

	t.Run("mac1 capture", func(t *testing.T) {
		hostName := "host1"
		addr := packet.Addr{MAC: mac1}
		tc.session.Capture(addr.MAC)
		p := newDHCP4DiscoverFrame(addr, hostName, nil)
		frame, err := tc.session.Parse(p)
		if err != nil {
			panic(err)
		}
		err = tc.h.ProcessPacket(frame)
		if lease := tc.h.findByMAC(addr.MAC); lease == nil || lease.IPOffer != netip.MustParseAddr("192.168.0.130") || lease.Name != hostName || lease.State != StateDiscover ||
			lease.subnet != tc.h.net2 {
			tc.h.PrintTable()
			t.Error("invalid discover state lease", lease)
		}
	})
}

func TestDHCPHandler_exhaust(t *testing.T) {
	options := packet.DHCP4Options{}
	options[packet.DHCP4OptionCode(packet.DHCP4OptionParameterRequestList)] = []byte{byte(packet.DHCP4OptionDomainNameServer)}

	packet.Logger.SetLevel(fastlog.LevelError)
	Logger.SetLevel(fastlog.LevelError)
	os.Remove(testDHCPFilename)
	tc := setupTestHandler()
	defer tc.Close()

	tests := []struct {
		name          string
		packet        packet.DHCP4
		wantResponse  bool
		tableLen      int
		responseCount int
		srcAddr       packet.Addr
		dstAddr       packet.Addr
	}{
		{name: "discover-mac1", wantResponse: true, responseCount: 260,
			packet: testRequestPacket(packet.DHCP4Discover, mac1, ip1, []byte{0x01}, false, options), tableLen: 256, // maximum unique macs
			srcAddr: packet.Addr{MAC: routerMAC, IP: routerIP4, Port: packet.DHCP4ClientPort},
			dstAddr: packet.Addr{MAC: mac1, IP: ip1, Port: packet.DHCP4ServerPort}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 260; i++ {
				mac := mac1
				mac[5] = byte(i)
				var err error
				ether := packet.Ether(make([]byte, packet.EthMaxSize))
				ether = packet.EncodeEther(ether, syscall.ETH_P_IP, tt.srcAddr.MAC, tt.dstAddr.MAC)
				ip4 := packet.EncodeIP4(ether.Payload(), 50, tt.srcAddr.IP, tt.dstAddr.IP)
				udp := packet.EncodeUDP(ip4.Payload(), tt.srcAddr.Port, tt.dstAddr.Port)
				dhcp := packet.EncodeDHCP4(udp.Payload(), packet.DHCP4BootRequest, packet.DHCP4Discover, mac, packet.IPv4zero, packet.IPv4zero, []byte{0x01}, false, options, options[packet.DHCP4OptionParameterRequestList])
				udp = udp.SetPayload(dhcp)
				ip4 = ip4.SetPayload(udp, syscall.IPPROTO_UDP)
				if ether, err = ether.SetPayload(ip4); err != nil {
					t.Fatal("error processing packet", err)
					return
				}
				frame, err := tc.session.Parse(ether)
				if err != nil {
					panic(err)
				}
				err = tc.h.ProcessPacket(frame)
				if err != nil {
					t.Errorf("DHCPHandler.handleDiscover() error sending packet error=%s", err)
					return
				}
				select {
				case <-tc.notifyReply:
				case <-time.After(time.Millisecond * 10):
					t.Fatal("failed to receive reply")
				}
			}

			if n := len(tc.h.table); n != tt.tableLen {
				tc.h.printTable()
				t.Errorf("DHCPHandler.handleDiscover() invalid lease table len=%d want=%d", n, tt.tableLen)
			}
			tc.Lock()
			if tt.responseCount != tc.count {
				t.Errorf("DHCPHandler.handleDiscover() invalid response count=%d want=%d", tc.count, tt.responseCount)
			}
			tc.Unlock()
		})
	}
	checkLeaseTable(t, tc, 0, 256, 0)
}
