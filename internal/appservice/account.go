package appservice

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"
)

type accountLookup interface {
	Current() (*user.User, error)
	Lookup(username string) (*user.User, error)
	LookupID(uid string) (*user.User, error)
	LookupGroupID(gid string) (*user.Group, error)
	GroupIDs(account *user.User) ([]string, error)
}

func resolveInstallAccount(
	requestedUser string,
	sudoUID string,
	sudoUser string,
	lookup accountLookup,
) (Account, string, error) {
	requestedUser = strings.TrimSpace(requestedUser)
	sudoUID = strings.TrimSpace(sudoUID)
	sudoUser = strings.TrimSpace(sudoUser)

	var selected *user.User
	var errSelected error
	switch {
	case requestedUser != "":
		selected, errSelected = lookup.Lookup(requestedUser)
		if errSelected != nil {
			return Account{}, "", fmt.Errorf("look up requested service user %q: %w", requestedUser, errSelected)
		}
	case sudoUID != "":
		_, errUID := strconv.ParseUint(sudoUID, 10, 32)
		if errUID != nil {
			return Account{}, "", fmt.Errorf("invalid SUDO_UID %q; provide --user explicitly: %w", sudoUID, errUID)
		}
		selected, errSelected = lookup.LookupID(sudoUID)
		if errSelected != nil {
			return Account{}, "", fmt.Errorf("look up sudo-invoking UID %s; provide --user explicitly: %w", sudoUID, errSelected)
		}
		if sudoUser != "" && sudoUser != selected.Username {
			return Account{}, "", fmt.Errorf(
				"SUDO_USER %q does not match SUDO_UID %s (%s); provide --user explicitly",
				sudoUser,
				sudoUID,
				selected.Username,
			)
		}
	case sudoUser != "":
		return Account{}, "", errors.New("SUDO_USER is set without SUDO_UID; provide --user explicitly")
	default:
		selected, errSelected = lookup.Current()
		if errSelected != nil {
			return Account{}, "", fmt.Errorf("look up current service user: %w", errSelected)
		}
	}

	if selected == nil || strings.TrimSpace(selected.Username) == "" || strings.TrimSpace(selected.Uid) == "" {
		return Account{}, "", errors.New("resolved service user is incomplete")
	}
	group, errGroup := lookup.LookupGroupID(selected.Gid)
	if errGroup != nil {
		return Account{}, "", fmt.Errorf("look up primary group for service user %s: %w", selected.Username, errGroup)
	}
	groupIDs, errGroupIDs := lookup.GroupIDs(selected)
	if errGroupIDs != nil {
		return Account{}, "", fmt.Errorf("look up supplementary groups for service user %s: %w", selected.Username, errGroupIDs)
	}

	account := Account{
		Username:       selected.Username,
		UID:            selected.Uid,
		PrimaryGroup:   group.Name,
		PrimaryGroupID: selected.Gid,
		GroupIDs:       append([]string(nil), groupIDs...),
	}
	var warning string
	if account.UID == "0" {
		warning = "The Linux service will run as root. Use --user to select a less-privileged existing account."
	}
	return account, warning, nil
}
