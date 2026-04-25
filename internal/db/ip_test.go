package db

import (
	"context"
	"errors"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func seedNodeScopedIPFixture(t *testing.T, conn *Connection) {
	t.Helper()

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert local node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into local_settings (id, node_id) values (1, ?)`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errSettings)
	}
}

func TestUpsertIPAndGetAllIPs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-upsert.sqlite")
	seedNodeScopedIPFixture(t, conn)

	setter := &models.IPSetter{
		Address:            omit.From("192.168.1.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-local"),
	}

	ip, errUpsert := conn.UpsertIP(setter)
	if errUpsert != nil {
		t.Fatalf("UpsertIP() error = %v", errUpsert)
	}
	if ip == nil {
		t.Fatalf("UpsertIP() returned nil, want non-nil")
	}
	if ip.Address != "192.168.1.1" {
		t.Errorf("UpsertIP().Address = %q, want %q", ip.Address, "192.168.1.1")
	}
	if !ip.Usable {
		t.Errorf("UpsertIP().Usable = false, want true")
	}

	ips, errGetAll := conn.GetAllIPs()
	if errGetAll != nil {
		t.Fatalf("GetAllIPs() error = %v", errGetAll)
	}

	found := false
	for _, existingIP := range ips {
		if existingIP.Address == "192.168.1.1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetAllIPs() missing inserted IP %q", "192.168.1.1")
	}
}

func TestUpsertIPConflictDoesNothing(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-conflict.sqlite")
	seedNodeScopedIPFixture(t, conn)

	setter := &models.IPSetter{
		Address:            omit.From("10.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(true),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-local"),
	}

	_, errFirst := conn.UpsertIP(setter)
	if errFirst != nil {
		t.Fatalf("UpsertIP(first) error = %v", errFirst)
	}

	// Second upsert with same address should return nil (DoNothing on conflict).
	conflictSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.1"),
		Usable:             omit.From(false),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(true),
		NodeID:             omit.From("node-local"),
	}

	ip, errSecond := conn.UpsertIP(conflictSetter)
	if !errors.Is(errSecond, ErrIPConflict) {
		t.Fatalf("UpsertIP(conflict) error = %v, want ErrIPConflict", errSecond)
	}
	if ip != nil {
		t.Errorf("UpsertIP(conflict) = %v, want nil (DoNothing)", ip)
	}
}

func TestUpsertIPAllowsSameAddressOnDifferentNodes(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-multi-node.sqlite")
	seedNodeScopedIPFixture(t, conn)

	_, errNode := conn.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-alt"),
		Name:      omit.From("Alternate Node"),
		ListenURL: omit.From("http://node-alt.local:8081"),
		Enabled:   omit.From(true),
	})
	if errNode != nil {
		t.Fatalf("InsertNode() error = %v", errNode)
	}

	firstSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-local"),
	}

	_, errFirst := conn.UpsertIP(firstSetter)
	if errFirst != nil {
		t.Fatalf("UpsertIP(first) error = %v", errFirst)
	}

	secondSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-alt"),
	}

	second, errSecond := conn.UpsertIP(secondSetter)
	if errSecond != nil {
		t.Fatalf("UpsertIP(second) error = %v", errSecond)
	}
	if second == nil {
		t.Fatalf("UpsertIP(second) returned nil, want non-nil")
	}

	localIP, errLocal := conn.GetIPByNodeIDAndAddress("node-local", "10.0.0.1")
	if errLocal != nil {
		t.Fatalf("GetIPByNodeIDAndAddress(local) error = %v", errLocal)
	}
	remoteIP, errRemote := conn.GetIPByNodeIDAndAddress("node-alt", "10.0.0.1")
	if errRemote != nil {
		t.Fatalf("GetIPByNodeIDAndAddress(remote) error = %v", errRemote)
	}

	if localIP.NodeID != "node-local" {
		t.Errorf("local IP node_id = %q, want %q", localIP.NodeID, "node-local")
	}
	if remoteIP.NodeID != "node-alt" {
		t.Errorf("remote IP node_id = %q, want %q", remoteIP.NodeID, "node-alt")
	}
}

func TestRemoveAutomaticallyAddedIPs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-remove-auto.sqlite")
	seedNodeScopedIPFixture(t, conn)

	manualSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.2"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-local"),
	}
	_, errManual := conn.UpsertIP(manualSetter)
	if errManual != nil {
		t.Fatalf("UpsertIP(manual) error = %v", errManual)
	}

	autoSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.3"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(true),
		NodeID:             omit.From("node-local"),
	}
	_, errAuto := conn.UpsertIP(autoSetter)
	if errAuto != nil {
		t.Fatalf("UpsertIP(auto) error = %v", errAuto)
	}

	errRemove := conn.RemoveAutomaticallyAddedIPs()
	if errRemove != nil {
		t.Fatalf("RemoveAutomaticallyAddedIPs() error = %v", errRemove)
	}

	ips, errGetAll := conn.GetAllIPs()
	if errGetAll != nil {
		t.Fatalf("GetAllIPs() error = %v", errGetAll)
	}

	for _, ip := range ips {
		if ip.Address == "10.0.0.3" {
			t.Errorf("GetAllIPs() still contains automatically added IP %q", "10.0.0.3")
		}
	}

	foundManual := false
	for _, ip := range ips {
		if ip.Address == "10.0.0.2" {
			foundManual = true
			break
		}
	}
	if !foundManual {
		t.Errorf("GetAllIPs() missing manual IP %q after removal", "10.0.0.2")
	}
}

func TestGetAllIPsNoError(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-empty.sqlite")

	// With no IPs inserted, GetAllIPs should return without error.
	_, errGetAll := conn.GetAllIPs()
	if errGetAll != nil {
		t.Fatalf("GetAllIPs() error = %v", errGetAll)
	}
}
