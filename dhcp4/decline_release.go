package dhcp4

import (
	"bytes"
	"net"

	log "github.com/sirupsen/logrus"
)

// handleDecline will process a DHCP decline message from a client and free up the
// entry in the dhcp table if present.
//
// Because the client is declining the use of the IP address supplied by the server,
// the client broadcasts DHCPDECLINE messages.
//
// -------------------------------------------+
// |              |DECLINE      |RELEASE      |             |          |
// -------------------------------------------+
// |broad/unicast |broadcast    |unicast      |
// |server-ip     |MUST         |MUST         |
// |requested-ip  |MUST         |MUST NOT     |
// |ciaddr        |zero         |IP address   |
// -------------------------------------------+
func (h *Handler) handleDecline(p DHCP4, options Options) (d DHCP4) {

	reqIP := net.IP(options[OptionRequestedIPAddress]).To4()
	serverIP := net.IP(options[OptionServerIdentifier]).To4()
	clientID := getClientID(p, options)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	_, subnet := h.findSubnet(p.CHAddr())
	lease := subnet.findCliendID(clientID)

	if !subnet.DHCPServer.Equal(serverIP) ||
		lease == nil || !lease.IP.Equal(reqIP) || !bytes.Equal(lease.MAC, p.CHAddr()) {
		lxid := []byte{}
		if lease != nil {
			lxid = lease.XID
		}
		log.WithFields(log.Fields{"clientid": clientID, "mac": p.CHAddr().String(),
			"xid": p.XId(), "leasexid": lxid, "serverip": serverIP, "reqip": reqIP}).Infof("dhcp4: decline - ignore invalid packet")
		return nil
	}

	log.WithFields(log.Fields{"clientid": clientID, "mac": lease.MAC, "ip": lease.IP, "xid": p.XId()}).Info("dhcp4: decline")

	freeLease(lease)

	if debugging() {
		subnet.printSubnet()
	}

	return nil
}

// handleRelease will process a DHCP release message and free up the
// entry in the dhcp table if present.
//
// If the client no longer requires use of its assigned network address
// (e.g., the client is gracefully shut down), the client sends a
// DHCPRELEASE message to the server.  Note that the correct operation
// of DHCP does not depend on the transmission of DHCPRELEASE messages.
//
// The client unicasts DHCPRELEASE messages to the server.
func (h *Handler) handleRelease(p DHCP4, options Options) (d DHCP4) {

	reqIP := p.CIAddr()
	serverIP := net.IP(options[OptionServerIdentifier]).To4()
	clientID := getClientID(p, options)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	_, subnet := h.findSubnet(p.CHAddr())
	lease := subnet.findCliendID(clientID)

	if !subnet.DHCPServer.Equal(serverIP) || lease == nil || !lease.IP.Equal(reqIP) {
		log.WithFields(log.Fields{"clientid": clientID, "mac": p.CHAddr(), "serverip": serverIP, "reqip": reqIP, "lease": lease}).Infof("dhcp4: release - discard invalid packet")
		return nil
	}

	log.WithFields(log.Fields{"clientid": clientID, "mac": lease.MAC, "ip": lease.IP, "xid": p.XId()}).Info("dhcp4: release")

	freeLease(lease)

	if debugging() {
		subnet.printSubnet()
	}

	return nil
}