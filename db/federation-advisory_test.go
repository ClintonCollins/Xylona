package db

import (
	"testing"
)

func TestInsertAndListFederationAdvisories(t *testing.T) {
	conn := newRBACMigratedConnection(t, "advisory-insert.sqlite")

	advisory := FederationAdvisory{
		ID:                 "adv-001",
		Type:               "NODE_AUTO_PAIRED",
		Title:              "Node auto-paired",
		Message:            "us-west-2 was auto-paired via introduction from eu-central-1",
		SourceNodeID:       "source-id",
		SourceNodeName:     "eu-central-1",
		SubjectNodeID:      "subject-id",
		SubjectNodeName:    "us-west-2",
		SubjectNodeBaseURL: "https://us-west-2.example.com",
		Read:               false,
	}

	errInsert := conn.InsertFederationAdvisory(advisory)
	if errInsert != nil {
		t.Fatalf("InsertFederationAdvisory() error = %v", errInsert)
	}

	advisories, total, errList := conn.ListFederationAdvisories(false, 50, 0)
	if errList != nil {
		t.Fatalf("ListFederationAdvisories() error = %v", errList)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(advisories) != 1 {
		t.Fatalf("len(advisories) = %d, want 1", len(advisories))
	}
	if advisories[0].Type != "NODE_AUTO_PAIRED" {
		t.Errorf("Type = %q, want %q", advisories[0].Type, "NODE_AUTO_PAIRED")
	}
	if advisories[0].SubjectNodeBaseURL != "https://us-west-2.example.com" {
		t.Errorf("SubjectNodeBaseURL = %q, want %q", advisories[0].SubjectNodeBaseURL, "https://us-west-2.example.com")
	}
	if advisories[0].Read {
		t.Errorf("Read = true, want false")
	}
	if advisories[0].CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero, want non-zero")
	}
}

func TestListFederationAdvisories_UnreadFilter(t *testing.T) {
	conn := newRBACMigratedConnection(t, "advisory-unread-filter.sqlite")

	// Insert one read and one unread advisory.
	errInsert := conn.InsertFederationAdvisory(FederationAdvisory{
		ID: "adv-read", Type: "NODE_AUTO_PAIRED", Title: "Read", Message: "Already read", Read: true,
	})
	if errInsert != nil {
		t.Fatalf("InsertFederationAdvisory() error = %v", errInsert)
	}
	errInsert = conn.InsertFederationAdvisory(FederationAdvisory{
		ID: "adv-unread", Type: "NODE_DEPARTED", Title: "Unread", Message: "Not yet read", Read: false,
	})
	if errInsert != nil {
		t.Fatalf("InsertFederationAdvisory() error = %v", errInsert)
	}

	// unreadOnly=false should return both.
	all, totalAll, errAll := conn.ListFederationAdvisories(false, 50, 0)
	if errAll != nil {
		t.Fatalf("ListFederationAdvisories(false) error = %v", errAll)
	}
	if totalAll != 2 {
		t.Errorf("total (all) = %d, want 2", totalAll)
	}
	if len(all) != 2 {
		t.Errorf("len (all) = %d, want 2", len(all))
	}

	// unreadOnly=true should return only the unread one.
	unread, totalUnread, errUnread := conn.ListFederationAdvisories(true, 50, 0)
	if errUnread != nil {
		t.Fatalf("ListFederationAdvisories(true) error = %v", errUnread)
	}
	if totalUnread != 1 {
		t.Errorf("total (unread) = %d, want 1", totalUnread)
	}
	if len(unread) != 1 {
		t.Errorf("len (unread) = %d, want 1", len(unread))
	}
}

func TestListFederationAdvisories_Pagination(t *testing.T) {
	conn := newRBACMigratedConnection(t, "advisory-pagination.sqlite")

	for i := range 5 {
		errInsert := conn.InsertFederationAdvisory(FederationAdvisory{
			ID: "adv-" + string(rune('a'+i)), Type: "NODE_AUTO_PAIRED", Title: "Advisory", Message: "Message",
		})
		if errInsert != nil {
			t.Fatalf("InsertFederationAdvisory() error = %v", errInsert)
		}
	}

	page1, total, errPage := conn.ListFederationAdvisories(false, 2, 0)
	if errPage != nil {
		t.Fatalf("ListFederationAdvisories(limit=2, offset=0) error = %v", errPage)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Errorf("len(page1) = %d, want 2", len(page1))
	}

	page2, _, errPage2 := conn.ListFederationAdvisories(false, 2, 2)
	if errPage2 != nil {
		t.Fatalf("ListFederationAdvisories(limit=2, offset=2) error = %v", errPage2)
	}
	if len(page2) != 2 {
		t.Errorf("len(page2) = %d, want 2", len(page2))
	}
}

func TestMarkAdvisoriesReadAndCount(t *testing.T) {
	conn := newRBACMigratedConnection(t, "advisory-read.sqlite")

	for _, id := range []string{"adv-a", "adv-b"} {
		errInsert := conn.InsertFederationAdvisory(FederationAdvisory{
			ID: id, Type: "NODE_AUTO_PAIRED", Title: "Test", Message: "Test message",
		})
		if errInsert != nil {
			t.Fatalf("InsertFederationAdvisory(%s) error = %v", id, errInsert)
		}
	}

	count, errCount := conn.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 2 {
		t.Errorf("unread count = %d, want 2", count)
	}

	errMark := conn.MarkAdvisoriesRead([]string{"adv-a"})
	if errMark != nil {
		t.Fatalf("MarkAdvisoriesRead() error = %v", errMark)
	}

	count, errCount = conn.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 1 {
		t.Errorf("unread count after marking = %d, want 1", count)
	}
}

func TestMarkAllAdvisoriesRead(t *testing.T) {
	conn := newRBACMigratedConnection(t, "advisory-markall.sqlite")

	for _, id := range []string{"adv-x", "adv-y", "adv-z"} {
		errInsert := conn.InsertFederationAdvisory(FederationAdvisory{
			ID: id, Type: "NODE_DEPARTED", Title: "Test", Message: "Test",
		})
		if errInsert != nil {
			t.Fatalf("InsertFederationAdvisory(%s) error = %v", id, errInsert)
		}
	}

	errMark := conn.MarkAdvisoriesRead(nil)
	if errMark != nil {
		t.Fatalf("MarkAdvisoriesRead(nil) error = %v", errMark)
	}

	count, errCount := conn.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 0 {
		t.Errorf("unread count = %d, want 0", count)
	}

	// Also test with empty slice.
	errInsert := conn.InsertFederationAdvisory(FederationAdvisory{
		ID: "adv-new", Type: "NODE_AUTO_PAIRED", Title: "New", Message: "Fresh",
	})
	if errInsert != nil {
		t.Fatalf("InsertFederationAdvisory() error = %v", errInsert)
	}

	errMark = conn.MarkAdvisoriesRead([]string{})
	if errMark != nil {
		t.Fatalf("MarkAdvisoriesRead([]) error = %v", errMark)
	}

	count, errCount = conn.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 0 {
		t.Errorf("unread count after empty-slice mark-all = %d, want 0", count)
	}
}
