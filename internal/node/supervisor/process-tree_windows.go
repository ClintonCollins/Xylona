//go:build windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	pty "github.com/aymanbagabas/go-pty"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

const (
	windowsProcessStillActive   = 259
	windowsConsoleSignalPIDEnv  = "XYLONA_WINDOWS_CONSOLE_SIGNAL_PID"
	windowsConsoleHelperModeEnv = "XYLONA_WINDOWS_CONSOLE_HELPER_MODE"
	windowsConsoleSignalTimeout = 5 * time.Second
	windowsConsoleInputMaxBytes = 64 << 10
	windowsConsoleKeyEvent      = 0x0001
	windowsVirtualKeyReturn     = 0x0D
	windowsMapVirtualKeyToScan  = 0
	windowsLeftAltPressed       = 0x0002
	windowsLeftCtrlPressed      = 0x0008
	windowsShiftPressed         = 0x0010
)

var (
	kernel32DLL               = windows.NewLazySystemDLL("kernel32.dll")
	user32DLL                 = windows.NewLazySystemDLL("user32.dll")
	attachConsoleProc         = kernel32DLL.NewProc("AttachConsole")
	freeConsoleProc           = kernel32DLL.NewProc("FreeConsole")
	setConsoleCtrlHandlerProc = kernel32DLL.NewProc("SetConsoleCtrlHandler")
	writeConsoleInputProc     = kernel32DLL.NewProc("WriteConsoleInputW")
	vkKeyScanProc             = user32DLL.NewProc("VkKeyScanW")
	mapVirtualKeyProc         = user32DLL.NewProc("MapVirtualKeyW")
)

func init() {
	if strings.TrimSpace(os.Getenv(windowsConsoleSignalPIDEnv)) == "" {
		return
	}
	handled, exitCode := RunConsoleSignalHelper()
	if handled {
		os.Exit(exitCode)
	}
}

type windowsKeyEventRecord struct {
	keyDown        int32
	repeatCount    uint16
	virtualKeyCode uint16
	virtualScan    uint16
	unicodeChar    uint16
	controlState   uint32
}

type windowsInputRecord struct {
	eventType uint16
	_         uint16
	keyEvent  windowsKeyEventRecord
}

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
		HideWindow:    true,
	}
}

func prepareProcessInvocation(baseCommand string, args []string) (string, []string, error) {
	extension := strings.ToLower(filepath.Ext(baseCommand))
	if extension != ".bat" && extension != ".cmd" {
		return baseCommand, append([]string(nil), args...), nil
	}

	commandArgs := make([]string, 0, len(args)+5)
	commandArgs = append(commandArgs, "/d", "/v:off", "/c", "call")
	preparedBaseCommand, errBaseCommand := prepareWindowsBatchArgument(baseCommand)
	if errBaseCommand != nil {
		return "", nil, fmt.Errorf("prepare batch command: %w", errBaseCommand)
	}
	commandArgs = append(commandArgs, preparedBaseCommand)
	for _, arg := range args {
		preparedArg, errArg := prepareWindowsBatchArgument(arg)
		if errArg != nil {
			return "", nil, fmt.Errorf("prepare batch argument: %w", errArg)
		}
		commandArgs = append(commandArgs, preparedArg)
	}

	return "cmd.exe", commandArgs, nil
}

func prepareWindowsBatchArgument(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n\"%!") {
		return "", errors.New("batch argument contains unsupported characters")
	}
	if strings.ContainsAny(value, " \t") {
		return value, nil
	}
	replacer := strings.NewReplacer(
		"^", "^^",
		"&", "^&",
		"|", "^|",
		"<", "^<",
		">", "^>",
		"(", "^(",
		")", "^)",
	)
	return replacer.Replace(value), nil
}

func requiresPseudoTerminal(serviceID string) bool {
	return serviceID == "terraria"
}

func drainPseudoTerminalBeforeClose() bool {
	return false
}

func preparePseudoTerminalCommand(
	_ context.Context,
	terminal pty.Pty,
	baseCommand string,
	args []string,
) *pty.Cmd {
	return terminal.Command(baseCommand, args...)
}

func watchPseudoTerminalCancellation(ctx context.Context, command *pty.Cmd) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			errTerminate := terminateProcessTree(command.Process)
			if errTerminate != nil && !errors.Is(errTerminate, os.ErrProcessDone) {
				log.Debug().Err(errTerminate).Msg("Failed to terminate canceled pseudo-terminal process")
			}
		case <-done:
		}
	}()
	return func() {
		close(done)
	}
}

func interruptProcessTree(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}
	pid := process.Pid
	if pid <= 0 || int64(pid) > int64(^uint32(0)) {
		return fmt.Errorf("invalid process ID %d", pid)
	}
	output, errRun := runWindowsConsoleHelper(pid, "signal", nil)
	if errRun != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = errRun.Error()
		}
		return fmt.Errorf("send CTRL+C to process console %d: %s", pid, message)
	}
	return nil
}

func mirrorProcessConsoleInput(cmd *exec.Cmd, input string) (bool, error) {
	if input == "" {
		return true, nil
	}
	if cmd == nil || cmd.Process == nil {
		return true, os.ErrProcessDone
	}
	pid := cmd.Process.Pid
	if pid <= 0 || int64(pid) > int64(^uint32(0)) {
		return true, fmt.Errorf("invalid process ID %d", pid)
	}
	output, errRun := runWindowsConsoleHelper(pid, "input", strings.NewReader(input+"\r"))
	if errRun != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = errRun.Error()
		}
		return true, fmt.Errorf("write command to process console %d: %s", pid, message)
	}
	return true, nil
}

func runWindowsConsoleHelper(pid int, mode string, input io.Reader) ([]byte, error) {
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		return nil, fmt.Errorf("resolve console helper executable: %w", errExecutable)
	}
	ctx, cancel := context.WithTimeout(context.Background(), windowsConsoleSignalTimeout)
	defer cancel()
	helper := exec.CommandContext(ctx, executable)
	helper.Env = append(
		os.Environ(),
		windowsConsoleSignalPIDEnv+"="+strconv.Itoa(pid),
		windowsConsoleHelperModeEnv+"="+mode,
	)
	helper.Stdin = input
	helper.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	output, errRun := helper.CombinedOutput()
	if errRun != nil {
		return output, fmt.Errorf("run Windows console helper: %w", errRun)
	}
	return output, nil
}

// RunConsoleSignalHelper handles private helper modes used to attach to a
// managed process's hidden console and deliver CTRL+C or keyboard input.
func RunConsoleSignalHelper() (bool, int) {
	pidValue := strings.TrimSpace(os.Getenv(windowsConsoleSignalPIDEnv))
	if pidValue == "" {
		return false, 0
	}
	pid, errPID := strconv.ParseUint(pidValue, 10, 32)
	if errPID != nil || pid == 0 {
		_, errWrite := fmt.Fprintf(os.Stderr, "invalid console signal process ID %q\n", pidValue)
		if errWrite != nil {
			return true, 2
		}
		return true, 2
	}
	mode := strings.TrimSpace(os.Getenv(windowsConsoleHelperModeEnv))
	if mode == "" {
		mode = "signal"
	}
	var errHelper error
	switch mode {
	case "signal":
		errHelper = sendWindowsConsoleInterrupt(uint32(pid))
	case "input":
		input, errRead := io.ReadAll(io.LimitReader(os.Stdin, windowsConsoleInputMaxBytes+1))
		switch {
		case errRead != nil:
			errHelper = fmt.Errorf("read console helper input: %w", errRead)
		case len(input) > windowsConsoleInputMaxBytes:
			errHelper = fmt.Errorf("console helper input exceeds %d bytes", windowsConsoleInputMaxBytes)
		default:
			errHelper = writeWindowsConsoleInput(uint32(pid), string(input))
		}
	default:
		errHelper = fmt.Errorf("unsupported console helper mode %q", mode)
	}
	if errHelper != nil {
		_, errWrite := fmt.Fprintln(os.Stderr, errHelper)
		if errWrite != nil {
			return true, 1
		}
		return true, 1
	}
	return true, 0
}

func writeWindowsConsoleInput(processID uint32, input string) error {
	if input == "" {
		return nil
	}
	errAttach := attachWindowsConsole(processID)
	if errAttach != nil {
		return errAttach
	}

	consoleName, errConsoleName := windows.UTF16PtrFromString("CONIN$")
	if errConsoleName != nil {
		errFree := callWindowsConsoleProc(freeConsoleProc)
		return errors.Join(
			fmt.Errorf("encode console input device name: %w", errConsoleName),
			wrapWindowsConsoleError("free target process console", errFree),
		)
	}
	consoleInput, errOpen := windows.CreateFile(
		consoleName,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if errOpen != nil {
		errFree := callWindowsConsoleProc(freeConsoleProc)
		return errors.Join(
			fmt.Errorf("open target console input: %w", errOpen),
			wrapWindowsConsoleError("free target process console", errFree),
		)
	}
	records, errRecords := windowsConsoleInputRecords(input)
	if errRecords != nil {
		errClose := windows.CloseHandle(consoleInput)
		errFree := callWindowsConsoleProc(freeConsoleProc)
		return errors.Join(
			errRecords,
			wrapWindowsConsoleError("close target console input", errClose),
			wrapWindowsConsoleError("free target process console", errFree),
		)
	}
	var recordsWritten uint32
	recordCount := helpers.ClampUint32FromInt(len(records))
	result, _, errWrite := writeConsoleInputProc.Call(
		uintptr(consoleInput),
		uintptr(unsafe.Pointer(&records[0])),
		uintptr(recordCount),
		uintptr(unsafe.Pointer(&recordsWritten)),
	)
	if result == 0 && (errWrite == nil || errors.Is(errWrite, syscall.Errno(0))) {
		errWrite = syscall.EINVAL
	}
	if result != 0 && recordsWritten != recordCount {
		errWrite = fmt.Errorf("wrote %d of %d console input records", recordsWritten, len(records))
	}
	errClose := windows.CloseHandle(consoleInput)
	errFree := callWindowsConsoleProc(freeConsoleProc)
	if result != 0 && recordsWritten == recordCount {
		errWrite = nil
	}
	return errors.Join(
		wrapWindowsConsoleError("write target console input", errWrite),
		wrapWindowsConsoleError("close target console input", errClose),
		wrapWindowsConsoleError("free target process console", errFree),
	)
}

func windowsConsoleInputRecords(input string) ([]windowsInputRecord, error) {
	codeUnits := utf16.Encode([]rune(input))
	records := make([]windowsInputRecord, 0, len(codeUnits)*2)
	for _, codeUnit := range codeUnits {
		virtualKey, virtualScan, controlState, errVirtualKey := windowsVirtualKeyForCodeUnit(codeUnit)
		if errVirtualKey != nil {
			return nil, errVirtualKey
		}
		keyEvent := windowsKeyEventRecord{
			keyDown:        1,
			repeatCount:    1,
			virtualKeyCode: virtualKey,
			virtualScan:    virtualScan,
			unicodeChar:    codeUnit,
			controlState:   controlState,
		}
		records = append(records, windowsInputRecord{eventType: windowsConsoleKeyEvent, keyEvent: keyEvent})
		keyEvent.keyDown = 0
		records = append(records, windowsInputRecord{eventType: windowsConsoleKeyEvent, keyEvent: keyEvent})
	}
	return records, nil
}

func windowsVirtualKeyForCodeUnit(codeUnit uint16) (uint16, uint16, uint32, error) {
	if codeUnit == '\r' {
		scan, _, errMap := mapVirtualKeyProc.Call(windowsVirtualKeyReturn, windowsMapVirtualKeyToScan)
		if errMap != nil && !errors.Is(errMap, syscall.Errno(0)) {
			return 0, 0, 0, fmt.Errorf("map return key to scan code: %w", errMap)
		}
		scanCode, errScanCode := windowsUint16Result(scan, "return key scan code")
		if errScanCode != nil {
			return 0, 0, 0, errScanCode
		}
		return windowsVirtualKeyReturn, scanCode, 0, nil
	}
	keyScan, _, errKeyScan := vkKeyScanProc.Call(uintptr(codeUnit))
	if errKeyScan != nil && !errors.Is(errKeyScan, syscall.Errno(0)) {
		return 0, 0, 0, fmt.Errorf("map console character %d to virtual key: %w", codeUnit, errKeyScan)
	}
	if keyScan == math.MaxUint16 {
		return 0, 0, 0, nil
	}
	virtualKey := uint16(keyScan & 0xFF)
	shiftState := uint8((keyScan >> 8) & 0xFF)
	controlState := uint32(0)
	if shiftState&1 != 0 {
		controlState |= windowsShiftPressed
	}
	if shiftState&2 != 0 {
		controlState |= windowsLeftCtrlPressed
	}
	if shiftState&4 != 0 {
		controlState |= windowsLeftAltPressed
	}
	scan, _, errMap := mapVirtualKeyProc.Call(uintptr(virtualKey), windowsMapVirtualKeyToScan)
	if errMap != nil && !errors.Is(errMap, syscall.Errno(0)) {
		return 0, 0, 0, fmt.Errorf("map virtual key %d to scan code: %w", virtualKey, errMap)
	}
	scanCode, errScanCode := windowsUint16Result(scan, "virtual key scan code")
	if errScanCode != nil {
		return 0, 0, 0, errScanCode
	}
	return virtualKey, scanCode, controlState, nil
}

func windowsUint16Result(value uintptr, label string) (uint16, error) {
	if value > math.MaxUint16 {
		return 0, fmt.Errorf("%s %d exceeds uint16", label, value)
	}
	return uint16(value), nil
}

func sendWindowsConsoleInterrupt(processID uint32) error {
	errAttach := attachWindowsConsole(processID)
	if errAttach != nil {
		return errAttach
	}
	errSignal := windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0)
	if errSignal == nil {
		time.Sleep(250 * time.Millisecond)
	}
	errFree := callWindowsConsoleProc(freeConsoleProc)
	return errors.Join(
		wrapWindowsConsoleError("generate CTRL+C event", errSignal),
		wrapWindowsConsoleError("free target process console", errFree),
	)
}

func attachWindowsConsole(processID uint32) error {
	// Helpers normally start without a console. FreeConsole is deliberately
	// best-effort so this also works when the executable inherited one.
	freeResult, _, errInitialFree := freeConsoleProc.Call()
	if freeResult == 0 && !errors.Is(errInitialFree, windows.ERROR_INVALID_HANDLE) {
		return fmt.Errorf("detach signal helper from inherited console: %w", errInitialFree)
	}
	errAttach := callWindowsConsoleProc(attachConsoleProc, uintptr(processID))
	if errAttach != nil {
		return fmt.Errorf("attach to process console %d: %w", processID, errAttach)
	}
	errIgnore := callWindowsConsoleProc(setConsoleCtrlHandlerProc, 0, 1)
	if errIgnore != nil {
		errFree := callWindowsConsoleProc(freeConsoleProc)
		return errors.Join(
			fmt.Errorf("ignore CTRL+C in signal helper: %w", errIgnore),
			wrapWindowsConsoleError("free target process console", errFree),
		)
	}
	return nil
}

func callWindowsConsoleProc(proc *windows.LazyProc, args ...uintptr) error {
	result, _, errCall := proc.Call(args...)
	if result != 0 {
		return nil
	}
	if errCall != nil && !errors.Is(errCall, syscall.Errno(0)) {
		return fmt.Errorf("call Windows console API: %w", errCall)
	}
	return syscall.EINVAL
}

func wrapWindowsConsoleError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}

	pid := process.Pid
	running, errRunning := windowsProcessRunning(pid)
	if errRunning == nil && !running {
		return os.ErrProcessDone
	}
	// taskkill /T walks the descendant tree before terminating the root process.
	//nolint:gosec,noctx // PID comes from os.Process and taskkill has no context-aware API.
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	errTaskkill := taskkill.Run()
	if errTaskkill == nil {
		return nil
	}

	errKill := process.Kill()
	if errors.Is(errKill, os.ErrProcessDone) || errors.Is(errKill, syscall.EINVAL) {
		return os.ErrProcessDone
	}
	if errKill != nil {
		return errors.Join(
			fmt.Errorf("terminate process tree %d: %w", pid, errTaskkill),
			fmt.Errorf("terminate root process %d: %w", pid, errKill),
		)
	}
	return fmt.Errorf("terminate process tree %d: %w", pid, errTaskkill)
}

func windowsProcessRunning(pid int) (bool, error) {
	if pid <= 0 || int64(pid) > int64(^uint32(0)) {
		return false, fmt.Errorf("invalid process ID %d", pid)
	}
	// The bounds check above guarantees that this conversion cannot truncate.
	processID := uint32(pid)
	handle, errOpen := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if errors.Is(errOpen, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if errOpen != nil {
		return false, fmt.Errorf("open process %d: %w", pid, errOpen)
	}
	var exitCode uint32
	errExitCode := windows.GetExitCodeProcess(handle, &exitCode)
	errClose := windows.CloseHandle(handle)
	if errExitCode != nil || errClose != nil {
		return false, errors.Join(errExitCode, errClose)
	}
	return exitCode == windowsProcessStillActive, nil
}
