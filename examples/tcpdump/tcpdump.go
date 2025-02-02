package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"net/netip"

	"github.com/deeGraYve/packet"
)

// Simple tcpdump utility to dump network packets.
var (
	macStr = flag.String("mac", "", "filter by mac address")
	ipStr  = flag.String("ip", "", "filter by ip address")
	nic    = flag.String("i", "eth0", "nic interface")
)

func filterByMAC(mac net.HardwareAddr) func(packet.Frame) bool {
	return func(frame packet.Frame) bool {
		if bytes.Equal(frame.Ether().Src(), mac) || bytes.Equal(frame.Ether().Dst(), mac) {
			return false
		}
		return true
	}
}

func filterByIP(ip netip.Addr) func(packet.Frame) bool {
	if ip.Is4() {
		return func(frame packet.Frame) bool {
			ip4 := frame.IP4()
			if ip4 == nil { // skip if not IPv4 packet
				return true
			}
			if ip == ip4.Src() || ip == ip4.Dst() {
				return false
			}
			return true
		}
	}

	return func(frame packet.Frame) bool {
		ip6 := frame.IP6()
		if ip6 == nil { // skip if not IPv6 packet
			return true
		}
		if ip == ip6.Src() || ip == ip6.Dst() {
			return false
		}
		return true
	}
}

func log(frame packet.Frame) {
	fmt.Println("ether  :", frame.Ether())
	if udp := frame.UDP(); udp != nil {
		fmt.Printf("udp   : src=%s dst=%s\n", frame.SrcAddr, frame.DstAddr)
	}
	if tcp := frame.TCP(); tcp != nil {
		fmt.Printf("tcp    : src %s dst %s\n", frame.SrcAddr, frame.DstAddr)
	}
	fmt.Printf("payload: payloadID=%v payload=% x\n\n", frame.PayloadID, frame.Payload())
}

func main() {
	var err error
	flag.Parse()

	// add mac filter
	var filters []func(packet.Frame) bool
	if *macStr != "" {
		if mac, err := net.ParseMAC(*macStr); err == nil {
			filters = append(filters, filterByMAC(mac))
		} else {
			fmt.Println("invalid mac", *macStr, err)
			flag.PrintDefaults()
			return
		}
	}

	// add ip filter
	if *ipStr != "" {
		if ip, err := netip.ParseAddr(*ipStr); err != nil {
			filters = append(filters, filterByIP(ip))
		} else {
			fmt.Println("invalid ip", *ipStr, err)
			flag.PrintDefaults()
			return
		}
	}

	fmt.Println("setting up nic:", *nic)
	s, err := packet.NewSession(*nic)
	if err != nil {
		fmt.Printf("conn error: %s", err)
		return
	}
	defer s.Close()

	buffer := make([]byte, packet.EthMaxSize)
	for {
		n, _, err := s.ReadFrom(buffer)
		if err != nil {
			fmt.Println("error reading packet", err)
			return
		}

		frame, err := s.Parse(buffer[:n])
		if err != nil {
			fmt.Println("error parsing frame", err)
			continue
		}

		// skip non matching packets
		var skip bool
		for _, filter := range filters {
			if skip = filter(frame); skip {
				break
			}
		}
		if skip {
			continue
		}
		log(frame)
	}
}
