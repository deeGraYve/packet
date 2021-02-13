package arp

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
)

// ARP Operation types
const (
	OperationRequest = 1
	OperationReply   = 2
)

type ARP []byte

// arpLen length is header + 2 * MACs + 2 IPs
// in theory MAC and IP len can vary
const arpLen = 8 + 2*6 + 2*4

func (b ARP) IsValid() bool {
	// lenght is header + 2 * MACs + 2 IPs
	if len(b) < arpLen {
		return false
	}
	if b.HType() != 1 {
		return false
	}
	if b.Proto() != syscall.ETH_P_IP {
		return false
	}
	if b.HLen() != 6 {
		return false
	}
	if b.PLen() != 4 {
		return false
	}

	return true
}

func (b ARP) HType() uint16 {
	return binary.BigEndian.Uint16(b[0:2])
}

func (b ARP) Proto() uint16 {
	return binary.BigEndian.Uint16(b[2:4])
}

func (b ARP) HLen() uint8 {
	return b[4]
}

func (b ARP) PLen() uint8 {
	return b[5]
}

func (b ARP) Operation() uint16 {
	return binary.BigEndian.Uint16(b[6:8])
}

func (b ARP) SrcMAC() net.HardwareAddr {
	return net.HardwareAddr(b[8:14])
}

func (b ARP) SrcIP() net.IP {
	return net.IP(b[14:18])
}

func (b ARP) DstMAC() net.HardwareAddr {
	return net.HardwareAddr(b[18:24])
}

func (b ARP) DstIP() net.IP {
	return net.IP(b[24:28])
}

func (b ARP) String() string {
	return fmt.Sprintf("operation=%d proto=%d srcMAC=%s srcIP=%s dstMAC=%s dstIP=%s", b.Operation(), b.Proto(), b.SrcMAC(), b.SrcIP(), b.DstMAC(), b.DstIP())
}

func ARPMarshalBinary(b []byte, operation uint16, srcMAC net.HardwareAddr, srcIP net.IP, dstMAC net.HardwareAddr, dstIP net.IP) ([]byte, error) {
	if b == nil {
		b = make([]byte, arpLen)
	}
	if len(b) < arpLen {
		return nil, fmt.Errorf("invalid len")
	}
	if len(srcIP) != 4 || len(dstIP) != 4 {
		return nil, fmt.Errorf("invalid len ip4 in src or dst srcLen=%d dstLen=%d", len(srcIP), len(dstIP))
	}

	binary.BigEndian.PutUint16(b[0:2], 1) // Ethernet is 1
	binary.BigEndian.PutUint16(b[2:4], syscall.ETH_P_IP)
	b[4] = 6 // mac len - fixed
	b[5] = 4 // ip len - fixed
	binary.BigEndian.PutUint16(b[6:8], operation)
	copy(b[8:8+6], srcMAC)
	copy(b[14:14+4], srcIP)
	copy(b[18:18+6], dstMAC)
	copy(b[24:24+4], dstIP)

	return b[:arpLen], nil
}
