package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// RuntimeSecurityContext captures runtime-specific launch hardening state that
// should be shared between startup validation and per-server launches.
type RuntimeSecurityContext struct {
	DBFilePath  string
	CurrentUser string
	Elevated    bool
}

// RuntimeSecurityAssessmentInput contains the state needed to evaluate runtime
// layout checks and same-user warnings.
type RuntimeSecurityAssessmentInput struct {
	DBFilePath  string
	Servers     []*models.GameServer
	CurrentUser string
	Elevated    bool
}

// RuntimeSecurityAssessment reports blocking layout errors and non-blocking
// warnings for the hardened standard-user runtime model.
type RuntimeSecurityAssessment struct {
	BlockingErrors []string
	Warnings       []string
}

// DetectRuntimeSecurityContext captures the current process identity state used
// by runtime validation and launch warnings.
func DetectRuntimeSecurityContext(dbFilePath string) RuntimeSecurityContext {
	return RuntimeSecurityContext{
		DBFilePath:  strings.TrimSpace(dbFilePath),
		CurrentUser: currentRuntimeUserName(),
		Elevated:    currentProcessIsElevated(),
	}
}

// AssessRuntimeSecurity evaluates runtime layout and same-user trust model
// warnings for the provided servers.
func AssessRuntimeSecurity(input RuntimeSecurityAssessmentInput) RuntimeSecurityAssessment {
	assessment := RuntimeSecurityAssessment{}

	assessment.addWarning(sameUserRuntimeWarning(input.CurrentUser))
	if input.Elevated {
		assessment.addWarning(`Elevated runtime warning: Xylona is running with root or Administrator-equivalent privileges, so a compromised managed server could reach beyond the hardened same-user boundary.`)
	}

	dbFilePath := strings.TrimSpace(input.DBFilePath)
	if dbFilePath == `` {
		return assessment
	}

	resolvedDBFilePath, errResolveDBPath := resolvePathForComparison(dbFilePath)
	if errResolveDBPath != nil {
		assessment.addBlockingError(fmt.Sprintf(`Blocking layout error: unable to resolve DB_FILE_PATH %q for safety checks: %v`, dbFilePath, errResolveDBPath))
		return assessment
	}

	resolvedDefaultInstallRoot := resolveManagedRoot(DefaultInstallPath)
	resolvedDefaultBackupRoot := resolveManagedRoot(DefaultBackupDirectory)

	for _, gameServer := range input.Servers {
		if gameServer == nil {
			continue
		}

		serverLabel := runtimeSecurityServerLabel(gameServer)
		serverDirectory := strings.TrimSpace(gameServer.Directory)
		if serverDirectory != `` {
			resolvedServerDirectory, errResolveServerDirectory := resolvePathForComparison(serverDirectory)
			if errResolveServerDirectory != nil {
				assessment.addBlockingError(fmt.Sprintf(`Blocking layout error: unable to resolve game server directory for %s: %v`, serverLabel, errResolveServerDirectory))
			} else {
				if pathWithinOrEqual(resolvedServerDirectory, resolvedDBFilePath) {
					assessment.addBlockingError(fmt.Sprintf(`Blocking layout error: DB_FILE_PATH resolves inside the game server directory for %s. Move the SQLite database outside managed server directories before starting or updating this server.`, serverLabel))
				}
				if resolvedDefaultInstallRoot != `` && !pathWithinOrEqual(resolvedDefaultInstallRoot, resolvedServerDirectory) {
					assessment.addWarning(fmt.Sprintf(`Custom layout warning: %s uses a server directory outside the default managed root. This layout is allowed, but Xylona cannot treat it as part of the safer default layout.`, serverLabel))
				}
			}
		}

		backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
		if backupDirectory == `` {
			continue
		}

		resolvedBackupDirectory, errResolveBackupDirectory := resolvePathForComparison(backupDirectory)
		if errResolveBackupDirectory != nil {
			assessment.addBlockingError(fmt.Sprintf(`Blocking layout error: unable to resolve backup directory for %s: %v`, serverLabel, errResolveBackupDirectory))
			continue
		}
		if pathWithinOrEqual(resolvedBackupDirectory, resolvedDBFilePath) {
			assessment.addBlockingError(fmt.Sprintf(`Blocking layout error: DB_FILE_PATH resolves inside the configured backup directory for %s. Move the SQLite database outside managed backup roots before starting or updating this server.`, serverLabel))
		}
		if backupDirectoryUsesDefaultLayout(gameServer, resolvedBackupDirectory, resolvedDefaultBackupRoot) {
			continue
		}
		assessment.addWarning(fmt.Sprintf(`Custom layout warning: %s uses a backup directory outside the default managed layout. This layout is allowed, but it keeps more trust on same-user filesystem boundaries.`, serverLabel))
	}

	return assessment
}

// BlockingError collapses all blocking errors into a single error for launch
// and startup enforcement.
func (assessment *RuntimeSecurityAssessment) BlockingError() error {
	if len(assessment.BlockingErrors) == 0 {
		return nil
	}
	return errors.New(strings.Join(assessment.BlockingErrors, ` `))
}

func (assessment *RuntimeSecurityAssessment) addBlockingError(message string) {
	if slices.Contains(assessment.BlockingErrors, message) {
		return
	}
	assessment.BlockingErrors = append(assessment.BlockingErrors, message)
}

func (assessment *RuntimeSecurityAssessment) addWarning(message string) {
	if slices.Contains(assessment.Warnings, message) {
		return
	}
	assessment.Warnings = append(assessment.Warnings, message)
}

func sameUserRuntimeWarning(currentUser string) string {
	userDescriptor := `the current OS user`
	trimmedUser := strings.TrimSpace(currentUser)
	if trimmedUser != `` {
		userDescriptor = fmt.Sprintf(`the current OS user %q`, trimmedUser)
	}

	return fmt.Sprintf(`Same-user warning: Xylona and managed servers still run as %s. Child processes can still access sibling server files, backups, and user-profile data when host permissions allow it.`, userDescriptor)
}

func resolveManagedRoot(resolvePath func() (string, error)) string {
	root, errResolveRoot := resolvePath()
	if errResolveRoot != nil {
		return ``
	}

	resolvedRoot, errResolveComparison := resolvePathForComparison(root)
	if errResolveComparison != nil {
		return ``
	}

	return resolvedRoot
}

func backupDirectoryUsesDefaultLayout(gameServer *models.GameServer, resolvedBackupDirectory string, resolvedDefaultBackupRoot string) bool {
	if resolvedDefaultBackupRoot != `` && pathWithinOrEqual(resolvedDefaultBackupRoot, resolvedBackupDirectory) {
		return true
	}

	defaultServerBackupDirectory, errDefaultBackupDirectory := defaultBackupDirectoryForRuntimeSecurity(gameServer.Directory)
	if errDefaultBackupDirectory != nil {
		return false
	}

	resolvedDefaultServerBackupDirectory, errResolveDefaultBackupDirectory := resolvePathForComparison(defaultServerBackupDirectory)
	if errResolveDefaultBackupDirectory != nil {
		return false
	}

	return resolvedDefaultServerBackupDirectory == resolvedBackupDirectory
}

func defaultBackupDirectoryForRuntimeSecurity(serverDirectory string) (string, error) {
	trimmedDirectory := strings.TrimSpace(serverDirectory)
	if trimmedDirectory == `` {
		return DefaultBackupDirectory()
	}

	parentDirectory := filepath.Dir(filepath.Clean(trimmedDirectory))
	if parentDirectory == `.` || parentDirectory == `` {
		return DefaultBackupDirectory()
	}

	return filepath.Join(parentDirectory, `backups`), nil
}

func runtimeSecurityServerLabel(gameServer *models.GameServer) string {
	serverName := strings.TrimSpace(gameServer.Name)
	if serverName != `` {
		return fmt.Sprintf(`server %q`, serverName)
	}

	serverID := strings.TrimSpace(gameServer.ID)
	if serverID != `` {
		return fmt.Sprintf(`server %q`, serverID)
	}

	return `a managed server`
}

func currentRuntimeUserName() string {
	currentUser := strings.TrimSpace(os.Getenv(`USER`))
	if currentUser != `` {
		return currentUser
	}

	return strings.TrimSpace(os.Getenv(`USERNAME`))
}
