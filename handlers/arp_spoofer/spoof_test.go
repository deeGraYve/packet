package arp_spoofer

import (
	"syscall"
	"testing"
	"time"

	"github.com/deeGraYve/packet"
	"github.com/deeGraYve/packet/fastlog"
)

func Test_Probe_Reject(t *testing.T) {
	tc := setupTestHandler(t)
	defer tc.Close()

	Logger.SetLevel(fastlog.LevelDebug)
	packet.Logger.SetLevel(fastlog.LevelDebug)

	tests := []struct {
		name              string
		ether             packet.Ether
		arp               packet.ARP
		hunt              bool
		wantErr           error
		wantLen           int
		wantIPs           int
		wantCountResponse int
	}{
		{name: "replyMAC2",
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac2, routerMAC),
			arp:     newARPPacket(packet.ARPOperationReply, addr2, routerAddr),
			wantErr: nil, wantLen: 3, wantIPs: 1, wantCountResponse: 0, hunt: true},
		{name: "replyMAC3",
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac3, hostMAC),
			arp:     newARPPacket(packet.ARPOperationReply, addr3, hostAddr),
			wantErr: nil, wantLen: 4, wantIPs: 2, wantCountResponse: 1, hunt: true}, // MAC2 will start hunt and send single response
		{name: "probeMAC2", // probe does not add host but will send a probe reject if IP is not our DHCP IP
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac2, packet.EthernetBroadcast),
			arp:     newARPPacket(packet.ARPOperationRequest, packet.Addr{MAC: mac2, IP: packet.IPv4zero}, packet.Addr{MAC: zeroMAC, IP: ip2}),
			wantErr: nil, wantLen: 4, wantIPs: 2, wantCountResponse: 3, hunt: false},
		{name: "probeMAC3", // probe does not add host but will send a probe reject if IP is not our DHCP IP
			ether:   newEtherPacket(syscall.ETH_P_ARP, mac3, packet.EthernetBroadcast),
			arp:     newARPPacket(packet.ARPOperationRequest, packet.Addr{MAC: mac3, IP: packet.IPv4zero}, packet.Addr{MAC: zeroMAC, IP: ip3}),
			wantErr: nil, wantLen: 4, wantIPs: 2, wantCountResponse: 4, hunt: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ether, err := tt.ether.AppendPayload(tt.arp)
			if err != nil {
				panic(err)
			}
			frame, _ := tc.session.Parse(ether)

			// if probe frame, set a different DHCP IP to trigger probe reject
			if packet.ARP(frame.Payload()).SrcIP() == packet.IPv4zero {
				tc.session.SetDHCPv4IPOffer(frame.SrcAddr.MAC, ip5, packet.NameEntry{})
			}
			if err := tc.arp.ProcessPacket(frame); err != tt.wantErr {
				t.Errorf("Test_Requests:%s error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			time.Sleep(time.Millisecond * 50) // there is a delay of 10 msec for each packet in arp hunt - need 30msec to get all three

			tc.arp.arpMutex.Lock() // lock to test no dead locks remain - sanity check
			if len(tc.session.HostTable.Table) != tt.wantLen {
				tc.arp.arpMutex.Unlock()
				t.Errorf("Test_Requests:%s table len = %v, wantLen %v", tt.name, len(tc.session.HostTable.Table), tt.wantLen)
				return
			}
			tc.arp.arpMutex.Unlock()

			if len(tc.session.HostTable.Table) != tt.wantLen {
				t.Fatalf("Test_Requests:%s table len = %v, wantLen %v", tt.name, len(tc.session.HostTable.Table), tt.wantLen)
			}

			tc.Lock()
			if tc.countResponse != tt.wantCountResponse {
				t.Errorf("Test_Requests:%s invali response count=%v, want=%v", tt.name, tc.countResponse, tt.wantCountResponse)
			}
			tc.Unlock()
			if tt.hunt { // Hunt will send 1 packets for each mac
				tc.arp.StartHunt(packet.Addr{MAC: frame.Host.MACEntry.MAC, IP: frame.Host.MACEntry.IP4})
			}
		})
	}
}
