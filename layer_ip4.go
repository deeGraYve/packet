package packet

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/deeGraYve/packet/fastlog"
)

const (
	Version      = 4  // protocol version
	HeaderLen    = 20 // header length without extension headers
	UDPHeaderLen = 8
)

// IP4 provide decoding and encoding of IPv4 packets.
// see: ipv4.ParseHeader in https://raw.githubusercontent.com/golang/net/master/ipv4/header.go
type IP4 []byte

func (p IP4) IHL() int                { return int(p[0]&0x0f) << 2 } // IP header length
func (p IP4) Version() int            { return int(p[0] >> 4) }
func (p IP4) Protocol() uint8         { return p[9] }
func (p IP4) TOS() int                { return int(p[1]) }
func (p IP4) ID() int                 { return int(binary.BigEndian.Uint16(p[4:6])) }
func (p IP4) Flags() uint8            { return uint8(p[6]) & 0b11100000 } // first 3 bits
func (p IP4) FlagDontFragment() bool  { return (uint8(p[6]) & 0b01000000) != 0 }
func (p IP4) FlagMoreFragments() bool { return (uint8(p[6]) & 0b00100000) != 0 }
func (p IP4) Fragment() uint16        { return ((uint16(p[6]) & 0b00011111) << 8) & uint16(p[7]) }
func (p IP4) TTL() int                { return int(p[8]) }
func (p IP4) Checksum() int           { return int(binary.BigEndian.Uint16(p[10:12])) }
func (p IP4) Src() netip.Addr         { return netip.AddrFrom4(*((*[4]byte)(p[12:16]))) }
func (p IP4) Dst() netip.Addr         { return netip.AddrFrom4(*((*[4]byte)(p[16:20]))) }
func (p IP4) TotalLen() int           { return int(binary.BigEndian.Uint16(p[2:4])) } // total packet size including header and payload
func (p IP4) Payload() []byte         { return p[p.IHL():p.TotalLen()] }
func (p IP4) String() string {
	return Logger.Msg("").Struct(p).ToString()
}

func (p IP4) IsValid() error {
	if n := len(p); n >= 20 && n >= p.IHL() && n >= p.TotalLen() {
		return nil
	}
	if n := len(p); n < 20 || n < p.IHL() {
		return fmt.Errorf("ipv4 header too short len=%d: %w", n, ErrFrameLen)
	}
	return fmt.Errorf("ipv4 len=%d not equal header totallen=%d: %w", len(p), p.TotalLen(), ErrFrameLen)
}

// FastLog implements fastlog interface
func (p IP4) FastLog(line *fastlog.Line) *fastlog.Line {
	line.Int("version", p.Version())
	line.IP("src", p.Src())
	line.IP("dst", p.Dst())
	line.Uint8("proto", p.Protocol())
	line.Int("ttl", p.TTL())
	line.Int("tos", p.TOS())
	line.Uint8Hex("flags", p.Flags())
	if tmp := p.Fragment(); tmp != 0 {
		line.Int("fragment", int(tmp))
	}
	line.Int("totallen", p.TotalLen())
	return line
}

func EncodeIP4(p []byte, ttl byte, src netip.Addr, dst netip.Addr) IP4 {
	options := []byte{}
	var hdrLen = HeaderLen + len(options) // len includes options
	const fragOffset = 0
	const flags = 0
	const totalLen = HeaderLen + 0 // 0 payload
	const id = 0
	const protocol = 0 // invalid
	const checksum = 0

	flagsAndFragOff := (fragOffset & 0x1fff) | int(flags<<13)
	if !src.Is4() {
		src = IPv4zero
	}
	if !dst.Is4() {
		dst = IPv4zero
	}

	p[0] = byte(Version<<4 | (hdrLen >> 2 & 0x0f))
	p[1] = byte(0xc0) // DSCP CS6)
	binary.BigEndian.PutUint16(p[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(p[4:6], uint16(id))
	binary.BigEndian.PutUint16(p[6:8], uint16(flagsAndFragOff))
	p[8] = byte(ttl)
	p[9] = byte(protocol)
	binary.BigEndian.PutUint16(p[10:12], uint16(checksum))
	s := src.As4()
	copy(p[12:16], s[:])
	s = dst.As4()
	copy(p[16:20], s[:])
	return p[:HeaderLen]
}

func (p IP4) SetPayload(b []byte, protocol byte) IP4 {
	p[9] = protocol
	totalLen := uint16(HeaderLen + len(b))
	binary.BigEndian.PutUint16(p[2:4], totalLen)
	checksum := p.CalculateChecksum()
	p[11] = byte(checksum >> 8)
	p[10] = byte(checksum)
	return p[:totalLen]
}

func (p IP4) AppendPayload(b []byte, protocol byte) (IP4, error) {
	if cap(p)-len(p) < len(b) {
		return nil, ErrPayloadTooBig
	}
	p = p[:len(p)+len(b)] // change slice in case slice is less than required
	totalLen := uint16(HeaderLen + len(b))
	binary.BigEndian.PutUint16(p[2:4], totalLen)
	copy(p.Payload(), b)
	p[9] = protocol
	checksum := p.CalculateChecksum()
	p[11] = byte(checksum >> 8)
	p[10] = byte(checksum)
	return p, nil
}

func (p IP4) CalculateChecksum() uint16 {
	psh := make([]byte, 20)
	copy(psh[0:10], p[0:10])           // first 10 bytes
	copy(psh[10:10+8], p[10+2:10+2+8]) // skip checksum filed in pos 10
	return Checksum(psh)
}

// Checksum calculate IP4, ICMP6 checksum
// In network format already
func Checksum(b []byte) uint16 {
	csumcv := len(b) - 1 // checksum coverage
	s := uint32(0)
	for i := 0; i < csumcv; i += 2 {
		s += uint32(b[i+1])<<8 | uint32(b[i])
	}
	if csumcv&1 == 0 {
		s += uint32(b[csumcv])
	}
	s = s>>16 + s&0xffff
	s = s + s>>16
	return ^uint16(s)
}

// UDP provides decoding and encoding of udp frames
type UDP []byte

func (p UDP) String() string {
	return Logger.Msg("").Struct(p).ToString()
}

// FastLog implements fastlog interface
func (p UDP) FastLog(line *fastlog.Line) *fastlog.Line {
	line.Uint16("srcport", p.SrcPort())
	line.Uint16("dstport", p.DstPort())
	line.Int("len", int(p.Len()))
	line.Int("payloadlen", len(p.Payload()))
	return line
}

func (p UDP) SrcPort() uint16  { return binary.BigEndian.Uint16(p[0:2]) }
func (p UDP) DstPort() uint16  { return binary.BigEndian.Uint16(p[2:4]) }
func (p UDP) Len() uint16      { return binary.BigEndian.Uint16(p[4:6]) }
func (p UDP) Checksum() uint16 { return binary.BigEndian.Uint16(p[6:8]) }
func (p UDP) Payload() []byte  { return p[8:] }
func (p UDP) HeaderLen() int   { return 8 }

func (p UDP) IsValid() error {
	if len(p) >= 8 { // 8 bytes UDP header
		return nil
	}
	return fmt.Errorf("invalid udp len=%d: %w", len(p), ErrFrameLen)
}

func EncodeUDP(p []byte, srcPort uint16, dstPort uint16) UDP {
	if cap(p) < UDPHeaderLen {
		return nil
	}
	p = p[:UDPHeaderLen] // change slice in case slice is less than header
	binary.BigEndian.PutUint16(p[0:2], srcPort)
	binary.BigEndian.PutUint16(p[2:4], dstPort)
	binary.BigEndian.PutUint16(p[4:6], 0) // len zero - no payload
	// TODO: udp checksum is mandatory for IPv6 - must do this
	binary.BigEndian.PutUint16(p[6:8], 0) // no checksum
	return UDP(p)
}

func (p UDP) SetPayload(b []byte) UDP {
	binary.BigEndian.PutUint16(p[4:6], UDPHeaderLen+uint16(len(b)))
	binary.BigEndian.PutUint16(p[6:8], 0) // no checksum
	return p[:len(p)+len(b)]
}

func (p UDP) AppendPayload(b []byte) (UDP, error) {
	if cap(p)-len(p) < len(b) {
		return nil, ErrPayloadTooBig
	}
	p = p[:len(p)+len(b)] // change slice in case slice is less total
	copy(p.Payload(), b)
	binary.BigEndian.PutUint16(p[4:6], UDPHeaderLen+uint16(len(b)))
	binary.BigEndian.PutUint16(p[6:8], 0) // no checksum
	return p, nil
}

// TCP provides decoding of tcp frames
type TCP []byte

func (p TCP) IsValid() error {
	if len(p) >= 20 {
		return nil
	}
	return fmt.Errorf("invalid tcp len=%d: %w", len(p), ErrFrameLen)
}

func (p TCP) SrcPort() uint16  { return binary.BigEndian.Uint16(p[0:2]) }
func (p TCP) DstPort() uint16  { return binary.BigEndian.Uint16(p[2:4]) }
func (p TCP) Seq() uint32      { return binary.BigEndian.Uint32(p[4:8]) }
func (p TCP) Ack() uint32      { return binary.BigEndian.Uint32(p[8:12]) }
func (p TCP) HeaderLen() int   { return int(p[12] >> 4) }
func (p TCP) NS() bool         { return p[12]&0x01 != 0 }
func (p TCP) FIN() bool        { return p[13]&0x01 != 0 }
func (p TCP) SYN() bool        { return p[13]&0x02 != 0 }
func (p TCP) RST() bool        { return p[13]&0x04 != 0 }
func (p TCP) PSH() bool        { return p[13]&0x08 != 0 }
func (p TCP) ACK() bool        { return p[13]&0x10 != 0 }
func (p TCP) URG() bool        { return p[13]&0x20 != 0 }
func (p TCP) ECE() bool        { return p[13]&0x40 != 0 }
func (p TCP) CWR() bool        { return p[13]&0x80 != 0 }
func (p TCP) Window() uint16   { return binary.BigEndian.Uint16(p[14:16]) }
func (p TCP) Checksum() uint16 { return binary.BigEndian.Uint16(p[16:18]) }
func (p TCP) Urgent() uint16   { return binary.BigEndian.Uint16(p[18:20]) }
func (p TCP) Payload() []byte  { return p[p[12]>>4:] }
