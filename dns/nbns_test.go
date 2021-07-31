package dns

import (
	"fmt"
	"testing"

	"github.com/irai/packet"
	"github.com/irai/packet/fastlog"
)

func Test_encodeNBNSName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{name: "test FRED",
			args: args{name: "FRED"},
			want: " EGFCEFEECACACACACACACACACACACACA\x00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeNBNSName(tt.args.name); got != tt.want {
				t.Errorf("encodeNBNSName() =|%x|%v, want |%x|%v", got, len(got), tt.want, len(tt.want))
			}
		})
	}
}

func Test_decodeNBNSName(t *testing.T) {
	type args struct {
		buffer []byte
	}
	tests := []struct {
		name     string
		args     args
		wantName string
	}{
		{name: "test FRED",
			args:     args{buffer: []byte(" EGFCEFEECACACACACACACACACACACACA\x00")},
			wantName: "FRED",
		},
		{name: "test FRED LONG",
			args:     args{buffer: []byte(" EGFCEFEECACACACACACACACACACACACA.NETBIOS.COM\x00")},
			wantName: "FRED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, gotName, _ := decodeNBNSName(tt.args.buffer); gotName != tt.wantName {
				t.Errorf("decodeNBNSName() = %v, want %v", gotName, tt.wantName)
			}
		})
	}
}

//  sudo tcpdump -en -v -XX -t udp and port 137 or 138
// 34:e8:94:42:29:a9 > 02:42:15:e6:10:08, ethertype IPv4 (0x0800), length 271: 192.168.0.1.137 > 192.168.0.129.137: UDP, length 229
var nbnsFrame = []byte{
	0x02, 0x42, 0x15, 0xe6, 0x10, 0x08, 0x34, 0xe8, 0x94, 0x42, 0x29, 0xa9, 0x08, 0x00, 0x45, 0x00, //  .B....4..B)...E.
	0x01, 0x01, 0x00, 0x00, 0x40, 0x00, 0x40, 0x11, 0xb8, 0x19, 0xc0, 0xa8, 0x00, 0x01, 0xc0, 0xa8, //  ....@.@.........
	0x00, 0x81, 0x00, 0x89, 0x00, 0x89, 0x00, 0xed, 0x72, 0xc9, 0x01, 0x15, 0x84, 0x00, 0x00, 0x00, //  ........r.......
	0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x20, 0x43, 0x4b, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, //  .......CKAAAAAAA
	0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, //  AAAAAAAAAAAAAAAA
	0x41, 0x41, 0x41, 0x41, 0x41, 0x43, 0x41, 0x00, 0x00, 0x21, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, //  AAAAACA..!......
	0x00, 0xad, 0x07, 0x41, 0x52, 0x43, 0x48, 0x45, 0x52, 0x5f, 0x56, 0x52, 0x31, 0x36, 0x30, 0x30, //  ...ARCHER_VR1600
	0x56, 0x20, 0x00, 0x04, 0x00, 0x41, 0x52, 0x43, 0x48, 0x45, 0x52, 0x5f, 0x56, 0x52, 0x31, 0x36, //  V....ARCHER_VR16
	0x30, 0x30, 0x56, 0x20, 0x03, 0x04, 0x00, 0x41, 0x52, 0x43, 0x48, 0x45, 0x52, 0x5f, 0x56, 0x52, //  00V....ARCHER_VR
	0x31, 0x36, 0x30, 0x30, 0x56, 0x20, 0x20, 0x04, 0x00, 0x01, 0x02, 0x5f, 0x5f, 0x4d, 0x53, 0x42, //  1600V......__MSB
	0x52, 0x4f, 0x57, 0x53, 0x45, 0x5f, 0x5f, 0x02, 0x01, 0x84, 0x00, 0x57, 0x4f, 0x52, 0x4b, 0x47, //  ROWSE__....WORKG
	0x52, 0x4f, 0x55, 0x50, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x84, 0x00, 0x57, 0x4f, 0x52, //  ROUP.........WOR
	0x4b, 0x47, 0x52, 0x4f, 0x55, 0x50, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x1d, 0x04, 0x00, 0x57, //  KGROUP.........W
	0x4f, 0x52, 0x4b, 0x47, 0x52, 0x4f, 0x55, 0x50, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x1e, 0x84, //  ORKGROUP........
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  ................
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  ................
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  ...............
}

func Test_NBNS(t *testing.T) {
	session := packet.NewEmptySession()
	dnsHandler, _ := New(session)
	Debug = true

	ether := packet.Ether(nbnsFrame)
	fastlog.NewLine("test", "ether").Struct(ether).Write()
	ip := packet.IP4(ether.Payload())
	fastlog.NewLine("test", "ip").Struct(ip).Write()
	udp := packet.UDP(ip.Payload())
	fastlog.NewLine("test", "udp").Struct(udp).Write()
	p := DNS(udp.Payload())
	if p.IsValid() != nil {
		t.Fatal("invalid dns packet")
	}

	name, err := dnsHandler.ProcessNBNS(nil, nil, udp.Payload())
	// ipv4Host, _, err := dnsHandler.ProcessMDNS(nil, nil, udp.Payload())
	if err != nil {
		t.Error("unexpected error", err)
	}
	if name.Name != "ARCHER_VR1600V" {
		t.Errorf("unexpected name |%s|", name.Name)
	}
}

// 34:e8:94:42:29:a9 > ff:ff:ff:ff:ff:ff, ethertype IPv4 (0x0800), length 257: (tos 0x0, ttl 64, id 0, offset 0, flags [DF], proto UDP (17), length 243)
// 192.168.0.1.138 > 192.168.0.255.138: UDP, length 215
var frameNBNS138 = []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x34, 0xe8, 0x94, 0x42, 0x29, 0xa9, 0x08, 0x00, 0x45, 0x00, //  ......4..B)...E.
	0x00, 0xf3, 0x00, 0x00, 0x40, 0x00, 0x40, 0x11, 0xb7, 0xa9, 0xc0, 0xa8, 0x00, 0x01, 0xc0, 0xa8, //  ....@.@.........
	0x00, 0xff, 0x00, 0x8a, 0x00, 0x8a, 0x00, 0xdf, 0xa8, 0x15, 0x11, 0x0a, 0x77, 0x56, 0xc0, 0xa8, //  ............wV..
	0x00, 0x01, 0x00, 0x8a, 0x00, 0xc9, 0x00, 0x00, 0x20, 0x45, 0x42, 0x46, 0x43, 0x45, 0x44, 0x45, //  .........EBFCEDE
	0x49, 0x45, 0x46, 0x46, 0x43, 0x46, 0x50, 0x46, 0x47, 0x46, 0x43, 0x44, 0x42, 0x44, 0x47, 0x44, //  IEFFCFPFGFCDBDGD
	0x41, 0x44, 0x41, 0x46, 0x47, 0x43, 0x41, 0x41, 0x41, 0x00, 0x20, 0x46, 0x48, 0x45, 0x50, 0x46, //  ADAFGCAAA..FHEPF
	0x43, 0x45, 0x4c, 0x45, 0x48, 0x46, 0x43, 0x45, 0x50, 0x46, 0x46, 0x46, 0x41, 0x43, 0x41, 0x43, //  CELEHFCEPFFFACAC
	0x41, 0x43, 0x41, 0x43, 0x41, 0x43, 0x41, 0x43, 0x41, 0x42, 0x4f, 0x00, 0xff, 0x53, 0x4d, 0x42, //  ACACACACABO..SMB
	0x25, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  %...............
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x2f, //  .............../
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  ................
	0x00, 0x00, 0x00, 0x2f, 0x00, 0x56, 0x00, 0x03, 0x00, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00, 0x40, //  .../.V.........@
	0x00, 0x5c, 0x4d, 0x41, 0x49, 0x4c, 0x53, 0x4c, 0x4f, 0x54, 0x5c, 0x42, 0x52, 0x4f, 0x57, 0x53, //  .\MAILSLOT\BROWS
	0x45, 0x00, 0x0f, 0x03, 0x80, 0xfc, 0x0a, 0x00, 0x41, 0x52, 0x43, 0x48, 0x45, 0x52, 0x5f, 0x56, //  E.......ARCHER_V
	0x52, 0x31, 0x36, 0x30, 0x30, 0x56, 0x00, 0x00, 0x06, 0x01, 0x03, 0x9a, 0x84, 0x00, 0x0f, 0x01, //  R1600V..........
	0x55, 0xaa, 0x41, 0x72, 0x63, 0x68, 0x65, 0x72, 0x5f, 0x56, 0x52, 0x31, 0x36, 0x30, 0x30, 0x76, //  U.Archer_VR1600v
	0x00, //x
}

/***
current nbns frame sent by us
02:42:15:e6:10:08 > 8a:f3:08:1e:01:55, ethertype IPv4 (0x0800), length 92: (tos 0x0, ttl 64, id 6159, offset 0, flags [DF], proto UDP (17), length 78)
    192.168.0.129.137 > 192.168.0.4.137: UDP, length 50
        0x0000:  8af3 081e 0155 0242 15e6 1008 0800 4500  .....U.B......E.
        0x0010:  004e 180f 4000 4011 a0ba c0a8 0081 c0a8  .N..@.@.........
        0x0020:  0004 0089 0089 003a 8221 0125 0010 0001  .......:.!.%....
        0x0030:  0000 0000 0000 2043 4b43 4143 4143 4143  .......CKCACACAC
        0x0040:  4143 4143 4143 4143 4143 4143 4143 4143  ACACACACACACACAC
        0x0050:  4143 4143 4143 4100 0021 0001            ACACACA..!..
		***/

func TestDNSHandler_SendNBNSNodeStatus(t *testing.T) {
	session := packet.NewEmptySession()
	dnsHandler, _ := New(session)

	if err := dnsHandler.SendNBNSNodeStatus(); err != nil {
		// TODO: need to fix empty session conn to prevent error when writing test
		fmt.Println("TODO need to fix empty conn", err)
	}
}
