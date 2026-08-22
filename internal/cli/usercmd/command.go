// Package usercmd implements the `xylona user` command family.
package usercmd

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/ClintonCollins/Xylona/internal/adminipc"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

type commandRunner interface {
	List(context.Context) ([]*usermgmt.User, error)
	GetByID(context.Context, string) (*usermgmt.User, error)
	GetByUsername(context.Context, string) (*usermgmt.User, error)
	Create(context.Context, usermgmt.CreateInput) (*usermgmt.User, error)
	Update(context.Context, usermgmt.UpdateInput) (*usermgmt.User, error)
	Delete(context.Context, string) error
}

type serviceRunner struct {
	service *usermgmt.Service
}

type connectRunner struct {
	client xylonaconnect.XylonaClient
}

type deleteJSONOutput struct {
	Deleted bool           `json:"deleted"`
	User    *usermgmt.User `json:"user"`
}

// Options configures the `xylona user` command family.
type Options struct {
	DefaultDBPath        string
	Migrate              func(*sql.DB) error
	ResolveDefaultDBPath func(context.Context) (string, error)
}

var (
	commandStdin             io.Reader = os.Stdin
	commandStdout            io.Writer = os.Stdout
	commandStderr            io.Writer = os.Stderr
	newRunnerFunc                      = newRunner
	guardOfflineMutationFunc           = func(ctx context.Context, dbPath string) (io.Closer, error) {
		return adminipc.GuardOfflineMutation(ctx, dbPath)
	}
	isTerminalFunc             = term.IsTerminal
	readPasswordFunc           = term.ReadPassword
	interactiveCreateInputFunc = promptForCreateInput
)

// Run executes a `xylona user` subcommand.
func Run(ctx context.Context, args []string, options Options) error {
	command := NewCommand(options)
	errRun := command.Run(ctx, append([]string{`xylona user`}, args...))
	if errRun != nil {
		return wrapCommandError(`run user cli`, errRun)
	}
	return nil
}

func runList(ctx context.Context, cmd *cli.Command, options Options) error {
	resolvedOptions, errResolve := resolveOptions(ctx, options)
	if errResolve != nil {
		return errResolve
	}

	modeFlags := modeFlagsFromCommand(cmd)
	runner, cleanup, errRunner := newRunnerFunc(ctx, modeFlags, resolvedOptions, false)
	if errRunner != nil {
		return errRunner
	}
	defer cleanup()

	users, errList := runner.List(ctx)
	if errList != nil {
		return wrapCommandError(`list users`, errList)
	}

	format := cmd.String(`format`)
	if strings.EqualFold(format, `json`) {
		return printJSON(users)
	}
	if !strings.EqualFold(format, `human`) {
		return fmt.Errorf(`unsupported format %q`, format)
	}

	return printUsersTable(users)
}

func runShow(ctx context.Context, cmd *cli.Command, options Options) error {
	resolvedOptions, errResolve := resolveOptions(ctx, options)
	if errResolve != nil {
		return errResolve
	}

	modeFlags := modeFlagsFromCommand(cmd)
	userID := cmd.String(`id`)
	username, errTarget := parseOptionalUsernameArg(cmd.Args().Slice(), userID)
	if errTarget != nil {
		return errTarget
	}

	runner, cleanup, errRunner := newRunnerFunc(ctx, modeFlags, resolvedOptions, false)
	if errRunner != nil {
		return errRunner
	}
	defer cleanup()

	user, errUser := getTargetUser(ctx, runner, userID, username)
	if errUser != nil {
		return errUser
	}

	format := cmd.String(`format`)
	if strings.EqualFold(format, `json`) {
		return printJSON(user)
	}
	if !strings.EqualFold(format, `human`) {
		return fmt.Errorf(`unsupported format %q`, format)
	}

	return printUserDetails(user)
}

func runCreate(ctx context.Context, cmd *cli.Command, options Options) error {
	if cmd.NArg() > 0 {
		return errors.New(`create does not accept positional arguments`)
	}

	resolvedOptions, errResolve := resolveOptions(ctx, options)
	if errResolve != nil {
		return errResolve
	}

	createInput, errCreateInput := resolveCreateInput(cmd)
	if errCreateInput != nil {
		return errCreateInput
	}

	modeFlags := modeFlagsFromCommand(cmd)
	runner, cleanup, errRunner := newRunnerFunc(ctx, modeFlags, resolvedOptions, isOfflineMutation(modeFlags))
	if errRunner != nil {
		return errRunner
	}
	defer cleanup()

	user, errCreate := runner.Create(ctx, createInput)
	if errCreate != nil {
		return wrapCommandError(`create user`, errCreate)
	}

	format := cmd.String(`format`)
	if strings.EqualFold(format, `json`) {
		return printJSON(user)
	}
	if !strings.EqualFold(format, `human`) {
		return fmt.Errorf(`unsupported format %q`, format)
	}

	return printUserDetails(user)
}

func resolveCreateInput(cmd *cli.Command) (usermgmt.CreateInput, error) {
	if cmd.Bool(`password-stdin`) {
		password, errPassword := readRequiredPassword(true, true)
		if errPassword != nil {
			return usermgmt.CreateInput{}, errPassword
		}
		return createInputFromCommand(cmd, password), nil
	}

	if !isTerminalFunc(stdinFileDescriptor()) {
		return usermgmt.CreateInput{}, errors.New(`interactive user creation requires a TTY; use --password-stdin for automation`)
	}

	return interactiveCreateInputFunc(cmd)
}

func createInputFromCommand(cmd *cli.Command, password string) usermgmt.CreateInput {
	return usermgmt.CreateInput{
		UserName:  cmd.String(`username`),
		Email:     cmd.String(`email`),
		FirstName: cmd.String(`first-name`),
		LastName:  cmd.String(`last-name`),
		Password:  password,
		SuperUser: cmd.Bool(`superuser`),
	}
}

func promptForCreateInput(cmd *cli.Command) (usermgmt.CreateInput, error) {
	input := createInputFromCommand(cmd, ``)

	userName, errUserName := promptRequiredLine(`Username`, input.UserName)
	if errUserName != nil {
		return usermgmt.CreateInput{}, errUserName
	}
	email, errEmail := promptRequiredLine(`Email`, input.Email)
	if errEmail != nil {
		return usermgmt.CreateInput{}, errEmail
	}
	superUser, errSuperUser := promptYesNo(`Grant superuser access?`, input.SuperUser)
	if errSuperUser != nil {
		return usermgmt.CreateInput{}, errSuperUser
	}
	firstName, errFirstName := promptOptionalLine(`First name`, input.FirstName)
	if errFirstName != nil {
		return usermgmt.CreateInput{}, errFirstName
	}
	lastName, errLastName := promptOptionalLine(`Last name`, input.LastName)
	if errLastName != nil {
		return usermgmt.CreateInput{}, errLastName
	}
	password, errPassword := promptForPassword(true)
	if errPassword != nil {
		return usermgmt.CreateInput{}, errPassword
	}
	confirmed, errConfirm := promptYesNo(`Create this user now?`, true)
	if errConfirm != nil {
		return usermgmt.CreateInput{}, errConfirm
	}
	if !confirmed {
		return usermgmt.CreateInput{}, errors.New(`user creation cancelled`)
	}

	return usermgmt.CreateInput{
		UserName:  userName,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Password:  password,
		SuperUser: superUser,
	}, nil
}

func promptRequiredLine(label string, current string) (string, error) {
	value, errPrompt := promptOptionalLine(label, current)
	if errPrompt != nil {
		return ``, errPrompt
	}
	if value == `` {
		if label == `Username` {
			return ``, usermgmt.ErrUserNameRequired
		}
		return ``, usermgmt.ErrEmailRequired
	}
	return value, nil
}

func promptOptionalLine(label string, current string) (string, error) {
	prompt := label
	if current != `` {
		prompt = fmt.Sprintf(`%s [%s]`, label, current)
	}
	_, errWrite := fmt.Fprintf(commandStdout, `%s: `, prompt)
	if errWrite != nil {
		return ``, fmt.Errorf(`write %s prompt: %w`, label, errWrite)
	}
	reader := bufio.NewReader(commandStdin)
	line, errRead := reader.ReadString('\n')
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return ``, fmt.Errorf(`read %s: %w`, label, errRead)
	}
	value := strings.TrimSpace(line)
	if value == `` {
		return strings.TrimSpace(current), nil
	}
	return value, nil
}

func promptYesNo(label string, defaultYes bool) (bool, error) {
	hint := `y/N`
	if defaultYes {
		hint = `Y/n`
	}
	_, errWrite := fmt.Fprintf(commandStdout, `%s [%s]: `, label, hint)
	if errWrite != nil {
		return false, fmt.Errorf(`write confirm prompt: %w`, errWrite)
	}
	reader := bufio.NewReader(commandStdin)
	line, errRead := reader.ReadString('\n')
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return false, fmt.Errorf(`read confirm: %w`, errRead)
	}
	value := strings.ToLower(strings.TrimSpace(line))
	if value == `` {
		return defaultYes, nil
	}
	return value == `y` || value == `yes`, nil
}

func runUpdate(ctx context.Context, cmd *cli.Command, options Options) error {
	resolvedOptions, errResolve := resolveOptions(ctx, options)
	if errResolve != nil {
		return errResolve
	}

	targetID := cmd.String(`id`)
	usernameTarget, errTarget := parseOptionalUsernameArg(cmd.Args().Slice(), targetID)
	if errTarget != nil {
		return errTarget
	}
	if cmd.Bool(`promote`) && cmd.Bool(`demote`) {
		return errors.New(`cannot use --promote and --demote together`)
	}
	if cmd.Bool(`password-prompt`) && cmd.Bool(`password-stdin`) {
		return errors.New(`choose either --password-prompt or --password-stdin`)
	}

	var password *string
	if cmd.Bool(`password-prompt`) || cmd.Bool(`password-stdin`) {
		passwordValue, errPassword := readRequiredPassword(cmd.Bool(`password-stdin`), false)
		if errPassword != nil {
			return errPassword
		}
		password = &passwordValue
	}

	var superUser *bool
	if cmd.Bool(`promote`) {
		value := true
		superUser = &value
	}
	if cmd.Bool(`demote`) {
		value := false
		superUser = &value
	}
	if !updateHasRequestedChanges(cmd, password, superUser) {
		return errors.New(`no changes requested; use --password-prompt, --password-stdin, or one of the profile update flags`)
	}

	modeFlags := modeFlagsFromCommand(cmd)
	runner, cleanup, errRunner := newRunnerFunc(ctx, modeFlags, resolvedOptions, isOfflineMutation(modeFlags))
	if errRunner != nil {
		return errRunner
	}
	defer cleanup()

	targetUser, errGetTarget := getTargetUser(ctx, runner, targetID, usernameTarget)
	if errGetTarget != nil {
		return errGetTarget
	}

	updatedUser, errUpdate := runner.Update(ctx, usermgmt.UpdateInput{
		ID:        targetUser.ID,
		UserName:  optionalStringFromCommand(cmd, `username`),
		Email:     optionalStringFromCommand(cmd, `email`),
		FirstName: optionalStringFromCommand(cmd, `first-name`),
		LastName:  optionalStringFromCommand(cmd, `last-name`),
		Password:  password,
		SuperUser: superUser,
	})
	if errUpdate != nil {
		return wrapCommandError(`update user`, errUpdate)
	}

	format := cmd.String(`format`)
	if strings.EqualFold(format, `json`) {
		return printJSON(updatedUser)
	}
	if !strings.EqualFold(format, `human`) {
		return fmt.Errorf(`unsupported format %q`, format)
	}

	return printUserDetails(updatedUser)
}

func updateHasRequestedChanges(cmd *cli.Command, password *string, superUser *bool) bool {
	if password != nil || superUser != nil {
		return true
	}

	return cmd.IsSet(`username`) ||
		cmd.IsSet(`email`) ||
		cmd.IsSet(`first-name`) ||
		cmd.IsSet(`last-name`)
}

func runDelete(ctx context.Context, cmd *cli.Command, options Options) error {
	resolvedOptions, errResolve := resolveOptions(ctx, options)
	if errResolve != nil {
		return errResolve
	}

	targetID := cmd.String(`id`)
	usernameTarget, errTarget := parseOptionalUsernameArg(cmd.Args().Slice(), targetID)
	if errTarget != nil {
		return errTarget
	}

	modeFlags := modeFlagsFromCommand(cmd)
	runner, cleanup, errRunner := newRunnerFunc(ctx, modeFlags, resolvedOptions, isOfflineMutation(modeFlags))
	if errRunner != nil {
		return errRunner
	}
	defer cleanup()

	targetUser, errGetTarget := getTargetUser(ctx, runner, targetID, usernameTarget)
	if errGetTarget != nil {
		return errGetTarget
	}

	if !cmd.Bool(`yes`) {
		errConfirm := confirmDelete(targetUser)
		if errConfirm != nil {
			return errConfirm
		}
	}

	errDelete := runner.Delete(ctx, targetUser.ID)
	if errDelete != nil {
		return wrapCommandError(`delete user`, errDelete)
	}

	format := cmd.String(`format`)
	if strings.EqualFold(format, `json`) {
		return printJSON(deleteJSONOutput{
			Deleted: true,
			User:    targetUser,
		})
	}
	if !strings.EqualFold(format, `human`) {
		return fmt.Errorf(`unsupported format %q`, format)
	}

	_, errPrint := fmt.Fprintf(commandStdout, "Deleted user %s (%s)\n", targetUser.UserName, targetUser.ID)
	if errPrint != nil {
		return wrapCommandError(`print delete result`, errPrint)
	}
	return nil
}

func (r *serviceRunner) List(_ context.Context) ([]*usermgmt.User, error) {
	users, errList := r.service.List()
	if errList != nil {
		return nil, wrapCommandError(`list offline users`, errList)
	}
	return users, nil
}

func (r *serviceRunner) GetByID(_ context.Context, userID string) (*usermgmt.User, error) {
	user, errGet := r.service.GetByID(userID)
	if errGet != nil {
		return nil, wrapCommandError(`get offline user by id`, errGet)
	}
	return user, nil
}

func (r *serviceRunner) GetByUsername(_ context.Context, username string) (*usermgmt.User, error) {
	user, errGet := r.service.GetByUsername(username)
	if errGet != nil {
		return nil, wrapCommandError(`get offline user by username`, errGet)
	}
	return user, nil
}

func (r *serviceRunner) Create(_ context.Context, input usermgmt.CreateInput) (*usermgmt.User, error) {
	user, errCreate := r.service.Create(input)
	if errCreate != nil {
		return nil, wrapCommandError(`create offline user`, errCreate)
	}
	return user, nil
}

func (r *serviceRunner) Update(_ context.Context, input usermgmt.UpdateInput) (*usermgmt.User, error) {
	user, errUpdate := r.service.Update(input)
	if errUpdate != nil {
		return nil, wrapCommandError(`update offline user`, errUpdate)
	}
	return user, nil
}

func (r *serviceRunner) Delete(_ context.Context, userID string) error {
	errDelete := r.service.Delete(usermgmt.DeleteInput{ID: userID})
	if errDelete != nil {
		return wrapCommandError(`delete offline user`, errDelete)
	}
	return nil
}

type modeFlags struct {
	offline bool
	dbPath  string
}

func modeFlagsFromCommand(cmd *cli.Command) *modeFlags {
	return &modeFlags{
		offline: cmd.Bool(`offline`),
		dbPath:  cmd.String(`db`),
	}
}

func newRunner(ctx context.Context, flags *modeFlags, options Options, mutating bool) (commandRunner, func(), error) {
	dbPath := options.DefaultDBPath
	if strings.TrimSpace(flags.dbPath) != `` {
		dbPath = flags.dbPath
	}

	if !isOfflineMode(flags) {
		client, errClient := adminipc.NewClient(dbPath)
		if errClient != nil {
			return nil, nil, wrapCommandError(`create live admin client`, errClient)
		}
		return &connectRunner{client: client}, func() {}, nil
	}

	needsGuard := mutating || options.Migrate != nil
	var lock io.Closer
	if needsGuard {
		acquiredLock, errGuard := guardOfflineMutationFunc(ctx, dbPath)
		if errGuard != nil {
			return nil, nil, wrapCommandError(`guard offline mutation`, errGuard)
		}
		lock = acquiredLock
	}

	conn, _, errConn := db.OpenOfflineUserConnection(ctx, dbPath)
	if errConn != nil {
		if lock != nil {
			_ = lock.Close()
		}
		return nil, nil, wrapCommandError(`open offline user database`, errConn)
	}
	if options.Migrate != nil {
		errMigrate := options.Migrate(conn.SQLDb)
		if errMigrate != nil {
			if lock != nil {
				_ = lock.Close()
			}
			_ = conn.SQLDb.Close()
			return nil, nil, wrapCommandError(`run offline migrations`, errMigrate)
		}
	}

	return &serviceRunner{service: usermgmt.NewService(conn)}, func() {
		if lock != nil {
			_ = lock.Close()
		}
		_ = conn.SQLDb.Close()
	}, nil
}

func getTargetUser(ctx context.Context, runner commandRunner, userID string, username string) (*usermgmt.User, error) {
	if strings.TrimSpace(userID) != `` {
		user, errGet := runner.GetByID(ctx, userID)
		if errGet != nil {
			return nil, wrapCommandError(`get target user by id`, errGet)
		}
		return user, nil
	}
	user, errGet := runner.GetByUsername(ctx, username)
	if errGet != nil {
		return nil, wrapCommandError(`get target user by username`, errGet)
	}
	return user, nil
}

func (r *connectRunner) List(ctx context.Context) ([]*usermgmt.User, error) {
	response, errList := r.client.ListUsers(ctx, connect.NewRequest(&xylona.ListUsersRequest{}))
	if errList != nil {
		return nil, wrapCommandError(`list live users`, errList)
	}

	users := make([]*usermgmt.User, len(response.Msg.GetUsers()))
	for i, user := range response.Msg.GetUsers() {
		users[i] = protoUserToUser(user)
	}

	return users, nil
}

func (r *connectRunner) GetByID(ctx context.Context, userID string) (*usermgmt.User, error) {
	response, errGet := r.client.GetUser(ctx, connect.NewRequest(&xylona.GetUserDetailsRequest{Id: userID}))
	if errGet != nil {
		return nil, wrapCommandError(`get live user by id`, errGet)
	}

	return protoUserToUser(response.Msg.GetUser()), nil
}

func (r *connectRunner) GetByUsername(ctx context.Context, username string) (*usermgmt.User, error) {
	users, errList := r.List(ctx)
	if errList != nil {
		return nil, wrapCommandError(`list live users for username lookup`, errList)
	}

	for _, user := range users {
		if user.UserName == username {
			return user, nil
		}
	}

	return nil, usermgmt.ErrUserNotFound
}

func (r *connectRunner) Create(ctx context.Context, input usermgmt.CreateInput) (*usermgmt.User, error) {
	response, errCreate := r.client.CreateUser(ctx, connect.NewRequest(&xylona.CreateUserRequest{
		UserName:  input.UserName,
		Email:     input.Email,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Password:  input.Password,
		SuperUser: input.SuperUser,
	}))
	if errCreate != nil {
		return nil, wrapCommandError(`create live user`, errCreate)
	}

	return protoUserToUser(response.Msg.GetUser()), nil
}

func (r *connectRunner) Update(ctx context.Context, input usermgmt.UpdateInput) (*usermgmt.User, error) {
	currentUser, errCurrent := r.GetByID(ctx, input.ID)
	if errCurrent != nil {
		return nil, wrapCommandError(`load current live user`, errCurrent)
	}

	request := &xylona.UpdateUserRequest{
		Id:        currentUser.ID,
		UserName:  currentUser.UserName,
		Email:     currentUser.Email,
		FirstName: currentUser.FirstName,
		LastName:  currentUser.LastName,
		SuperUser: currentUser.SuperUser,
	}
	if input.UserName != nil {
		request.UserName = *input.UserName
	}
	if input.Email != nil {
		request.Email = *input.Email
	}
	if input.FirstName != nil {
		request.FirstName = *input.FirstName
	}
	if input.LastName != nil {
		request.LastName = *input.LastName
	}
	if input.SuperUser != nil {
		request.SuperUser = *input.SuperUser
	}
	if input.Password != nil {
		request.Password = *input.Password
	}

	response, errUpdate := r.client.UpdateUser(ctx, connect.NewRequest(request))
	if errUpdate != nil {
		return nil, wrapCommandError(`update live user`, errUpdate)
	}

	return protoUserToUser(response.Msg.GetUser()), nil
}

func (r *connectRunner) Delete(ctx context.Context, userID string) error {
	_, errDelete := r.client.DeleteUser(ctx, connect.NewRequest(&xylona.DeleteUserRequest{Id: userID}))
	if errDelete != nil {
		return wrapCommandError(`delete live user`, errDelete)
	}
	return nil
}

func isOfflineMode(flags *modeFlags) bool {
	return flags.offline || strings.TrimSpace(flags.dbPath) != ``
}

func isOfflineMutation(flags *modeFlags) bool {
	return isOfflineMode(flags)
}

func protoUserToUser(user *xylona.User) *usermgmt.User {
	if user == nil {
		return nil
	}

	var lastLoginAt *time.Time
	if user.GetLastLogin() != nil {
		lastLogin := user.GetLastLogin().AsTime()
		lastLoginAt = &lastLogin
	}

	return &usermgmt.User{
		ID:          user.GetId(),
		UserName:    user.GetUserName(),
		Email:       user.GetEmail(),
		FirstName:   user.GetFirstName(),
		LastName:    user.GetLastName(),
		SuperUser:   user.GetSuperUser(),
		LastLoginAt: lastLoginAt,
		CreatedAt:   user.GetCreatedAt().AsTime(),
	}
}

func parseOptionalUsernameArg(args []string, userID string) (string, error) {
	if strings.TrimSpace(userID) != `` {
		if len(args) > 0 {
			return ``, errors.New(`do not pass a username argument with --id`)
		}
		return ``, nil
	}
	if len(args) != 1 {
		return ``, errors.New(`a username argument is required unless --id is provided`)
	}
	return strings.TrimSpace(args[0]), nil
}

func readRequiredPassword(fromStdin bool, confirm bool) (string, error) {
	if fromStdin {
		return readPasswordFromStdin()
	}
	return promptForPassword(confirm)
}

func readPasswordFromStdin() (string, error) {
	data, errRead := io.ReadAll(commandStdin)
	if errRead != nil {
		return ``, fmt.Errorf(`read password from stdin: %w`, errRead)
	}
	password := strings.TrimRight(string(data), "\r\n")
	if strings.TrimSpace(password) == `` {
		return ``, usermgmt.ErrPasswordEmpty
	}
	return password, nil
}

func promptForPassword(confirm bool) (string, error) {
	stdinFD := stdinFileDescriptor()
	if !isTerminalFunc(stdinFD) {
		return ``, errors.New(`interactive password entry requires a TTY; use --password-stdin for automation`)
	}

	fmt.Fprint(commandStderr, `Password: `)
	passwordBytes, errPassword := readPasswordFunc(stdinFD)
	fmt.Fprintln(commandStderr)
	if errPassword != nil {
		return ``, fmt.Errorf(`read password: %w`, errPassword)
	}

	password := string(passwordBytes)
	if strings.TrimSpace(password) == `` {
		return ``, usermgmt.ErrPasswordEmpty
	}

	if !confirm {
		return password, nil
	}

	fmt.Fprint(commandStderr, `Confirm password: `)
	confirmBytes, errConfirm := readPasswordFunc(stdinFD)
	fmt.Fprintln(commandStderr)
	if errConfirm != nil {
		return ``, fmt.Errorf(`read password confirmation: %w`, errConfirm)
	}

	if password != string(confirmBytes) {
		return ``, errors.New(`passwords do not match`)
	}

	return password, nil
}

func confirmDelete(user *usermgmt.User) error {
	if !isTerminalFunc(stdinFileDescriptor()) {
		return errors.New(`delete confirmation requires a TTY; rerun with --yes for automation`)
	}

	_, errPrompt := fmt.Fprintf(commandStderr, "Delete user %s (%s)? [y/N]: ", user.UserName, user.ID)
	if errPrompt != nil {
		return wrapCommandError(`prompt for delete confirmation`, errPrompt)
	}

	reader := bufio.NewReader(commandStdin)
	line, errRead := reader.ReadString('\n')
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return fmt.Errorf(`read delete confirmation: %w`, errRead)
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != `y` && answer != `yes` {
		return errors.New(`delete cancelled`)
	}

	return nil
}

func printUsersTable(users []*usermgmt.User) error {
	writer := tabwriter.NewWriter(commandStdout, 0, 2, 2, ' ', 0)
	_, errHeader := fmt.Fprintln(writer, "ID\tUSERNAME\tEMAIL\tSUPERUSER\tLAST_LOGIN\tCREATED_AT")
	if errHeader != nil {
		return wrapCommandError(`write users table header`, errHeader)
	}

	for _, user := range users {
		lastLogin := `never`
		if user.LastLoginAt != nil {
			lastLogin = user.LastLoginAt.UTC().Format(time.RFC3339)
		}
		_, errRow := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%t\t%s\t%s\n",
			user.ID,
			user.UserName,
			user.Email,
			user.SuperUser,
			lastLogin,
			user.CreatedAt.UTC().Format(time.RFC3339),
		)
		if errRow != nil {
			return wrapCommandError(`write users table row`, errRow)
		}
	}

	errFlush := writer.Flush()
	if errFlush != nil {
		return wrapCommandError(`flush users table`, errFlush)
	}
	return nil
}

func printUserDetails(user *usermgmt.User) error {
	lastLogin := `never`
	if user.LastLoginAt != nil {
		lastLogin = user.LastLoginAt.UTC().Format(time.RFC3339)
	}
	updatedAt := ``
	if !user.UpdatedAt.IsZero() {
		updatedAt = user.UpdatedAt.UTC().Format(time.RFC3339)
	}

	_, errPrint := fmt.Fprintf(
		commandStdout,
		"id: %s\nusername: %s\nemail: %s\nfirst_name: %s\nlast_name: %s\nsuperuser: %t\nlast_login: %s\ncreated_at: %s\nupdated_at: %s\n",
		user.ID,
		user.UserName,
		user.Email,
		user.FirstName,
		user.LastName,
		user.SuperUser,
		lastLogin,
		user.CreatedAt.UTC().Format(time.RFC3339),
		updatedAt,
	)
	if errPrint != nil {
		return wrapCommandError(`print user details`, errPrint)
	}
	return nil
}

func printJSON(payload any) error {
	buffer := bytes.Buffer{}
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent(``, `  `)
	errEncode := encoder.Encode(payload)
	if errEncode != nil {
		return fmt.Errorf(`encode json output: %w`, errEncode)
	}

	_, errWrite := io.Copy(commandStdout, &buffer)
	if errWrite != nil {
		return wrapCommandError(`write json output`, errWrite)
	}
	return nil
}

func optionalStringFromCommand(cmd *cli.Command, name string) *string {
	if !cmd.IsSet(name) {
		return nil
	}

	value := cmd.String(name)
	return &value
}

// NewCommand builds the `xylona user` command family for embedding into a root CLI.
func NewCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      `user`,
		Usage:     `Manage local users`,
		UsageText: `xylona user <command> [command options]`,
		Writer:    commandStdout,
		ErrWriter: commandStderr,
		Action: func(_ context.Context, cmd *cli.Command) error {
			errHelp := cli.ShowSubcommandHelp(cmd)
			if errHelp != nil {
				return wrapCommandError(`show user help`, errHelp)
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:      `list`,
				Usage:     `List users`,
				UsageText: `xylona user list [command options]`,
				Flags: append(modeCommandFlags(), []cli.Flag{
					&cli.StringFlag{
						Name:  `format`,
						Value: `human`,
						Usage: `Output format: human or json`,
					},
				}...),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runList(ctx, cmd, options)
				},
			},
			{
				Name:      `show`,
				Usage:     `Show a user`,
				UsageText: `xylona user show [command options] <username>`,
				Flags: append(modeCommandFlags(), []cli.Flag{
					&cli.StringFlag{
						Name:  `id`,
						Usage: `Show a user by ID instead of username`,
					},
					&cli.StringFlag{
						Name:  `format`,
						Value: `human`,
						Usage: `Output format: human or json`,
					},
				}...),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runShow(ctx, cmd, options)
				},
			},
			{
				Name:      `create`,
				Usage:     `Create a user`,
				UsageText: `xylona user create [command options]`,
				Flags: append(modeCommandFlags(), []cli.Flag{
					&cli.StringFlag{
						Name:  `username`,
						Usage: `Username`,
					},
					&cli.StringFlag{
						Name:  `email`,
						Usage: `Email`,
					},
					&cli.StringFlag{
						Name:  `first-name`,
						Usage: `First name`,
					},
					&cli.StringFlag{
						Name:  `last-name`,
						Usage: `Last name`,
					},
					&cli.BoolFlag{
						Name:  `superuser`,
						Usage: `Create the user as a superuser`,
					},
					&cli.BoolFlag{
						Name:  `password-stdin`,
						Usage: `Read the password from stdin`,
					},
					&cli.StringFlag{
						Name:  `format`,
						Value: `human`,
						Usage: `Output format: human or json`,
					},
				}...),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runCreate(ctx, cmd, options)
				},
			},
			{
				Name:      `update`,
				Usage:     `Update a user`,
				UsageText: `xylona user update [command options] <username>`,
				Flags: append(modeCommandFlags(), []cli.Flag{
					&cli.StringFlag{
						Name:  `id`,
						Usage: `Update a user by ID instead of username`,
					},
					&cli.StringFlag{
						Name:  `format`,
						Value: `human`,
						Usage: `Output format: human or json`,
					},
					&cli.BoolFlag{
						Name:  `password-stdin`,
						Usage: `Read the new password from stdin`,
					},
					&cli.BoolFlag{
						Name:  `password-prompt`,
						Usage: `Prompt for a new password interactively`,
					},
					&cli.BoolFlag{
						Name:  `promote`,
						Usage: `Promote the user to superuser`,
					},
					&cli.BoolFlag{
						Name:  `demote`,
						Usage: `Demote the user from superuser`,
					},
					&cli.StringFlag{
						Name:  `username`,
						Usage: `New username`,
					},
					&cli.StringFlag{
						Name:  `email`,
						Usage: `New email`,
					},
					&cli.StringFlag{
						Name:  `first-name`,
						Usage: `New first name`,
					},
					&cli.StringFlag{
						Name:  `last-name`,
						Usage: `New last name`,
					},
				}...),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runUpdate(ctx, cmd, options)
				},
			},
			{
				Name:      `delete`,
				Usage:     `Delete a user`,
				UsageText: `xylona user delete [command options] <username>`,
				Flags: append(modeCommandFlags(), []cli.Flag{
					&cli.StringFlag{
						Name:  `id`,
						Usage: `Delete a user by ID instead of username`,
					},
					&cli.BoolFlag{
						Name:  `yes`,
						Usage: `Skip the confirmation prompt`,
					},
					&cli.StringFlag{
						Name:  `format`,
						Value: `human`,
						Usage: `Output format: human or json`,
					},
				}...),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runDelete(ctx, cmd, options)
				},
			},
		},
	}
}

func modeCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  `offline`,
			Usage: `Use direct database access instead of the live local admin transport`,
		},
		&cli.StringFlag{
			Name:  `db`,
			Usage: `Use direct database access for the given SQLite file`,
		},
	}
}

func resolveOptions(ctx context.Context, options Options) (Options, error) {
	resolvedOptions := options
	if strings.TrimSpace(resolvedOptions.DefaultDBPath) != `` {
		return resolvedOptions, nil
	}
	if resolvedOptions.ResolveDefaultDBPath == nil {
		return resolvedOptions, nil
	}

	defaultDBPath, errResolve := resolvedOptions.ResolveDefaultDBPath(ctx)
	if errResolve != nil {
		return Options{}, wrapCommandError(`resolve default database path`, errResolve)
	}
	resolvedOptions.DefaultDBPath = defaultDBPath

	return resolvedOptions, nil
}

func wrapCommandError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(`usercmd: %s: %w`, action, err)
}

func stdinFileDescriptor() int {
	return int(os.Stdin.Fd())
}
