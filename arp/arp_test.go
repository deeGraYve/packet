package arp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/irai/packet"
)

var (
	zeroMAC = net.HardwareAddr{0, 0, 0, 0, 0, 0}

	hostMAC   = net.HardwareAddr{0x00, 0xff, 0x03, 0x04, 0x05, 0x01} // key first byte zero for unicast mac
	hostIP    = net.ParseIP("192.168.0.129").To4()
	homeLAN   = net.IPNet{IP: net.IPv4(192, 168, 0, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}
	routerMAC = net.HardwareAddr{0x00, 0xff, 0x03, 0x04, 0x05, 0x11} // key first byte zero for unicast mac
	routerIP  = net.ParseIP("192.168.0.11").To4()
	ip1       = net.ParseIP("192.168.0.1").To4()
	ip2       = net.ParseIP("192.168.0.2").To4()
	ip3       = net.ParseIP("192.168.0.3").To4()
	ip4       = net.ParseIP("192.168.0.4").To4()
	ip5       = net.ParseIP("192.168.0.5").To4()
	mac1      = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x01}
	mac2      = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x02}
	mac3      = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x03}
	mac4      = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x04}
	mac5      = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x05}
	localIP   = net.IPv4(169, 254, 0, 10).To4()
	localIP2  = net.IPv4(169, 254, 0, 11).To4()
)

type notificationCounter struct {
	onlineCounter  int
	offlineCounter int
}

type testContext struct {
	inConn        net.PacketConn
	outConn       net.PacketConn
	arp           *Handler
	session       *packet.Session
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	countResponse int
}

func setupTestHandler(t *testing.T) *testContext {

	var err error

	tc := testContext{}
	tc.ctx, tc.cancel = context.WithCancel(context.Background())
	tc.session = packet.NewEmptySession()

	// fake conn
	tc.inConn, tc.outConn = packet.TestNewBufferedConn()
	go readResponse(tc.ctx, &tc) // MUST read the out conn to avoid blocking the sender
	tc.session.Conn = tc.inConn

	// fake nicinfo
	tc.session.NICInfo = &packet.NICInfo{
		HostMAC:   hostMAC,
		HostIP4:   net.IPNet{IP: hostIP, Mask: net.IPv4Mask(255, 255, 255, 0)},
		RouterIP4: net.IPNet{IP: routerIP, Mask: net.IPv4Mask(255, 255, 255, 0)},
		HomeLAN4:  homeLAN,
	}

	if tc.arp, err = New(tc.session); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 10) // time for all goroutine to start
	return &tc
}

func (tc *testContext) Close() {
	time.Sleep(time.Millisecond * 20) // wait for all packets to finish
	if Debug {
		fmt.Println("teminating context")
	}
	tc.cancel()
	tc.wg.Wait()
}

func readResponse(ctx context.Context, tc *testContext) error {
	buffer := make([]byte, 2000)
	for {
		buf := buffer[:]
		n, _, err := tc.outConn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != context.Canceled {
				panic(err)
			}
		}
		if ctx.Err() == context.Canceled {
			return nil
		}

		buf = buf[:n]
		ether := packet.Ether(buf)
		if !ether.IsValid() {
			s := fmt.Sprintf("error ether client packet %s", ether)
			panic(s)
		}

		// used for debuging - disable to avoid verbose logging
		if false {
			fmt.Printf("test  : got test response=%s\n", ether)
		}

		if ether.EtherType() != syscall.ETH_P_ARP {
			panic("invalid ether type")
		}

		arpFrame := ARP(ether.Payload())
		if !arpFrame.IsValid() {
			panic("invalid arp packet")
		}
		tc.countResponse++

		// tmp := make([]byte, len(buf))
		// copy(tmp, buf)

	}
}

func Test_Handler_BasicTest(t *testing.T) {
	Debug = true
	packet.Debug = true
	tc := setupTestHandler(t)
	defer tc.Close()

	tests := []struct {
		name       string
		ether      packet.Ether
		arp        ARP
		wantErr    error
		wantLen    int
		wantResult bool
	}{
		{name: "replymac2",
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac2, routerMAC),
			arp:     newPacket(OperationReply, mac2, ip2, routerMAC, routerIP),
			wantErr: nil, wantLen: 1, wantResult: true},
		{name: "replymac3",
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac3, routerMAC),
			arp:     newPacket(OperationReply, mac3, ip3, routerMAC, routerIP),
			wantErr: nil, wantLen: 2, wantResult: true},
		{name: "replymac4",
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac4, routerMAC),
			arp:     newPacket(OperationReply, mac4, ip4, routerMAC, routerIP),
			wantErr: nil, wantLen: 3, wantResult: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ether, err := tt.ether.AppendPayload(tt.arp)
			if err != nil {
				panic(err)
			}

			_, result, err := tc.arp.ProcessPacket(nil, ether, ether.Payload())
			if err != tt.wantErr {
				t.Errorf("Test_Requests:%s error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if result.Update {
				tc.session.FindOrCreateHost(result.Addr.MAC, result.Addr.IP)
			}

			if len(tc.arp.session.GetHosts()) != tt.wantLen {
				t.Errorf("Test_Requests:%s table len = %v, wantLen %v", tt.name, len(tc.arp.session.GetHosts()), tt.wantLen)
				tc.arp.session.PrintTable()
			}
		})
	}

}
