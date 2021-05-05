package packet

import (
	"fmt"
	"testing"

	"github.com/irai/packet/model"
)

func setupTestHandler() *model.Session {
	h := &model.Session{HostTable: model.NewHostTable(), MACTable: model.NewMACTable()}
	return h
}

func TestHandler_findOrCreateHostDupIP(t *testing.T) {
	session := setupTestHandler()

	Debug = true

	// First create host for IP3 and IP2 and set online
	host1, _ := session.FindOrCreateHost(mac1, ip3)
	tc.Engine.lockAndSetOnline(host1, true)
	host1, _ = engine.findOrCreateHost(mac1, ip2)
	host1.dhcp4Store.Name = "mac1" // test that name will clear - this was a bug in past
	engine.lockAndSetOnline(host1, true)
	engine.lockAndSetOnline(host1, false)
	if err := engine.Capture(mac1); err != nil {
		t.Fatal(err)
	}
	if !host1.MACEntry.Captured {
		t.Fatal("host not capture")
	}

	// new mac, same IP
	host2, _ := engine.findOrCreateHost(mac1, ip2)
	if host2.MACEntry.Captured { // mac should not be captured
		t.Fatal("host not capture")
	}
	if host2.dhcp4Store.Name != "" {
		t.Fatal("invalid host name")
	}

	// there must be two macs
	if n := len(engine.MACTable.Table); n != 2 {
		engine.PrintTable()
		t.Fatal(fmt.Sprintf("invalid mac table len=%d", n))
	}

	// The must only be two hosts for IP2
	if n := len(engine.Engine.session.HostTable.Table); n != 2 {
		engine.PrintTable()
		t.Fatal(fmt.Sprintf("invalid host table len=%d ", n))
	}

	// second IPs
	host1, _ = engine.findOrCreateHost(mac2, ip2)
	if host1.MACEntry.Captured { // mac should not be captured
		t.Fatal("host not capture")
	}
}
