package appservice

import (
	"errors"
	"os/user"
	"strings"
	"testing"
)

type fakeAccountLookup struct {
	current       *user.User
	usersByName   map[string]*user.User
	usersByID     map[string]*user.User
	groupsByID    map[string]*user.Group
	groupIDs      map[string][]string
	currentErr    error
	lookupErr     error
	lookupIDErr   error
	groupErr      error
	groupIDsError error
}

func (f fakeAccountLookup) Current() (*user.User, error) {
	return f.current, f.currentErr
}

func (f fakeAccountLookup) Lookup(username string) (*user.User, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.usersByName[username], nil
}

func (f fakeAccountLookup) LookupID(uid string) (*user.User, error) {
	if f.lookupIDErr != nil {
		return nil, f.lookupIDErr
	}
	return f.usersByID[uid], nil
}

func (f fakeAccountLookup) LookupGroupID(gid string) (*user.Group, error) {
	if f.groupErr != nil {
		return nil, f.groupErr
	}
	return f.groupsByID[gid], nil
}

func (f fakeAccountLookup) GroupIDs(account *user.User) ([]string, error) {
	if f.groupIDsError != nil {
		return nil, f.groupIDsError
	}
	return append([]string(nil), f.groupIDs[account.Username]...), nil
}

func TestResolveInstallAccount(t *testing.T) {
	alice := &user.User{Username: "alice", Uid: "1000", Gid: "1000"}
	bob := &user.User{Username: "bob", Uid: "1001", Gid: "1001"}
	root := &user.User{Username: "root", Uid: "0", Gid: "0"}
	baseLookup := fakeAccountLookup{
		current: alice,
		usersByName: map[string]*user.User{
			"alice": alice,
			"bob":   bob,
			"root":  root,
		},
		usersByID: map[string]*user.User{
			"0":    root,
			"1000": alice,
			"1001": bob,
		},
		groupsByID: map[string]*user.Group{
			"0":    {Name: "root", Gid: "0"},
			"1000": {Name: "alice", Gid: "1000"},
			"1001": {Name: "games", Gid: "1001"},
		},
		groupIDs: map[string][]string{
			"alice": {"1000", "2000"},
			"bob":   {"1001"},
			"root":  {"0"},
		},
	}

	cases := []struct {
		name          string
		requestedUser string
		sudoUID       string
		sudoUser      string
		lookup        fakeAccountLookup
		wantUser      string
		wantGroup     string
		wantWarning   bool
		wantError     string
	}{
		{
			name:          "explicit user overrides sudo metadata",
			requestedUser: "bob",
			sudoUID:       "bad",
			sudoUser:      "wrong",
			lookup:        baseLookup,
			wantUser:      "bob",
			wantGroup:     "games",
		},
		{
			name:      "sudo UID selects original invoking user",
			sudoUID:   "1000",
			sudoUser:  "alice",
			lookup:    baseLookup,
			wantUser:  "alice",
			wantGroup: "alice",
		},
		{
			name:      "current user is fallback",
			lookup:    baseLookup,
			wantUser:  "alice",
			wantGroup: "alice",
		},
		{
			name:        "direct root produces warning",
			lookup:      withCurrentAccount(baseLookup, root),
			wantUser:    "root",
			wantGroup:   "root",
			wantWarning: true,
		},
		{
			name:      "malformed sudo UID is rejected",
			sudoUID:   "not-a-number",
			sudoUser:  "alice",
			lookup:    baseLookup,
			wantError: "invalid SUDO_UID",
		},
		{
			name:      "sudo username mismatch is rejected",
			sudoUID:   "1000",
			sudoUser:  "bob",
			lookup:    baseLookup,
			wantError: "does not match",
		},
		{
			name:      "sudo username without UID is rejected",
			sudoUser:  "alice",
			lookup:    baseLookup,
			wantError: "without SUDO_UID",
		},
		{
			name:          "unknown explicit user is rejected",
			requestedUser: "missing",
			lookup: withLookupError(
				baseLookup,
				errors.New("unknown user"),
			),
			wantError: "unknown user",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			account, warning, errResolve := resolveInstallAccount(
				testCase.requestedUser,
				testCase.sudoUID,
				testCase.sudoUser,
				testCase.lookup,
			)
			if testCase.wantError != "" {
				if errResolve == nil || !strings.Contains(errResolve.Error(), testCase.wantError) {
					t.Fatalf("resolveInstallAccount() error = %v, want containing %q", errResolve, testCase.wantError)
				}
				return
			}
			if errResolve != nil {
				t.Fatalf("resolveInstallAccount() error = %v", errResolve)
			}
			if account.Username != testCase.wantUser || account.PrimaryGroup != testCase.wantGroup {
				t.Fatalf(
					"resolved account = %s:%s, want %s:%s",
					account.Username,
					account.PrimaryGroup,
					testCase.wantUser,
					testCase.wantGroup,
				)
			}
			if (warning != "") != testCase.wantWarning {
				t.Fatalf("warning = %q, want warning %t", warning, testCase.wantWarning)
			}
		})
	}
}

func withCurrentAccount(lookup fakeAccountLookup, current *user.User) fakeAccountLookup {
	lookup.current = current
	return lookup
}

func withLookupError(lookup fakeAccountLookup, errLookup error) fakeAccountLookup {
	lookup.lookupErr = errLookup
	return lookup
}
