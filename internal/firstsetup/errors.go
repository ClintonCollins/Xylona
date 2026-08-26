package firstsetup

import "errors"

var (
	// ErrEncryptionKeyMissingExistingDatabase reports that a database already
	// exists and ENCRYPTION_KEY_BASE64 is missing. Generating a new key would
	// make encrypted control-plane secrets unrecoverable.
	ErrEncryptionKeyMissingExistingDatabase = errors.New("ENCRYPTION_KEY_BASE64 is missing and the database already exists; restore the matched recovery key")
	// ErrAlreadyInstalled reports that first-run setup has already created a superuser.
	ErrAlreadyInstalled = errors.New("first-run setup is already complete")
	// ErrSetupTokenInvalid reports that a setup token was missing or did not match.
	ErrSetupTokenInvalid = errors.New("setup token is invalid")
	// ErrEnvPathRequired reports that generated secrets cannot be persisted.
	ErrEnvPathRequired = errors.New("env path is required to persist generated secrets")
)
