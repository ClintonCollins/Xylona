package db

import (
	"errors"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestUpsertIPAndGetAllIPs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-upsert.sqlite")

	setter := &models.IPSetter{
		Address:            omit.From("192.168.1.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
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

	setter := &models.IPSetter{
		Address:            omit.From("10.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(true),
		AutomaticallyAdded: omit.From(false),
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
	}

	ip, errSecond := conn.UpsertIP(conflictSetter)
	if !errors.Is(errSecond, ErrIPConflict) {
		t.Fatalf("UpsertIP(conflict) error = %v, want ErrIPConflict", errSecond)
	}
	if ip != nil {
		t.Errorf("UpsertIP(conflict) = %v, want nil (DoNothing)", ip)
	}
}

func TestRemoveAutomaticallyAddedIPs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ip-remove-auto.sqlite")

	manualSetter := &models.IPSetter{
		Address:            omit.From("10.0.0.2"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
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
