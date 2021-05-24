package icmp6

import (
	"bytes"
	"fmt"
	"net"
	"testing"

	"github.com/irai/packet"
)

/**
sudo tcpdump -v -XX -t icmp6
icmp6 redirect
*/
var icmp6Redirect = []byte{
	0x02, 0x42, 0xca, 0x78, 0x04, 0x50, 0x7e, 0xe8, 0x94, 0x42, 0x29, 0xaa, 0x86, 0xdd,
	0x60, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x3a, 0xff, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // `.....:.........
	0xce, 0x32, 0xe5, 0xff, 0xfe, 0x0e, 0x67, 0xf4, 0x20, 0x01, 0x44, 0x79, 0x1e, 0x00, 0x82, 0x01, // .2....g...Dy....
	0x00, 0x42, 0xca, 0xff, 0xfe, 0x78, 0x04, 0x50, 0x89, 0x00, 0xbb, 0xf4, 0x00, 0x00, 0x00, 0x00, // .B...x.P........
	0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7c, 0xe8, 0x94, 0xff, 0xfe, 0x42, 0x29, 0xaa, // ........|....B).
	0x20, 0x01, 0x44, 0x79, 0x1e, 0x00, 0x82, 0x02, 0x00, 0x42, 0x15, 0xff, 0xfe, 0xe6, 0x10, 0x08, // ..Dy.....B......
	0x02, 0x01, 0x7e, 0xe8, 0x94, 0x42, 0x29, 0xaa, 0x04, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ..~..B).........
	0x60, 0x01, 0x90, 0x03, 0x00, 0x40, 0x3a, 0x40, 0x20, 0x01, 0x44, 0x79, 0x1e, 0x00, 0x82, 0x01, // `....@:@..Dy....
	0x00, 0x42, 0xca, 0xff, 0xfe, 0x78, 0x04, 0x50, 0x20, 0x01, 0x44, 0x79, 0x1e, 0x00, 0x82, 0x02, // .B...x.P..Dy....
	0x00, 0x42, 0x15, 0xff, 0xfe, 0xe6, 0x10, 0x08, 0x81, 0x00, 0xb9, 0x7e, 0x01, 0x0a, 0x09, 0x3a, // .B.........~...:
	0x5a, 0x28, 0xab, 0x60, 0xc4, 0x02, 0x0a, 0x00, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, // Z(.`............
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, // ................
	0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, // .!"#$%&'()*+,-./
	0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, // 01234567
}

func Test_ICMP6Redirect(t *testing.T) {
	tc := setupTestHandler()
	defer tc.Close()

	ether := packet.Ether(icmp6Redirect)
	fmt.Println("ether", ether)
	ip6Frame := packet.IP6(ether.Payload())
	fmt.Println("ip6", ip6Frame)
	icmp6Frame := ICMP6(ip6Frame.Payload())
	fmt.Println("icmp6", icmp6Frame)
	redirect := ICMP6Redirect(icmp6Frame)
	if !redirect.IsValid() {
		t.Fatal("invalid len")
	}
	fmt.Println("redirect", redirect)

	targetIP := net.ParseIP("fe80::7ce8:94ff:fe42:29aa")
	targetMAC, _ := net.ParseMAC("7e:e8:94:42:29:aa")
	dstIP := net.ParseIP("2001:4479:1e00:8202:42:15ff:fee6:1008")
	if !redirect.TargetAddress().Equal(targetIP) ||
		!redirect.DstAddress().Equal(dstIP) ||
		!bytes.Equal(redirect.TargetLinkLayerAddr(), targetMAC) {
		t.Fatal("invalid fields ", redirect)
	}

	_, _, err := tc.h.ProcessPacket(nil, ether, ip6Frame.Payload())
	if err != nil {
		t.Fatal(err)
	}

}
