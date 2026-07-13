// Package supervisor manages game server process lifecycles and runtime
// metrics.
package supervisor

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	pty "github.com/aymanbagabas/go-pty"
	"github.com/ziutek/telnet"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// Runtime identifies the host OS runtime used for process supervision.
type Runtime string

// Supported host runtimes for the process supervisor.
const (
	RuntimeLinux   Runtime = "linux"
	RuntimeWindows Runtime = "windows"
	RuntimeDarwin  Runtime = "darwin"
	RuntimeUnknown Runtime = "unknown"
)

var (
	// ErrCommandDoesNotExist is returned when a command lookup misses.
	ErrCommandDoesNotExist = errors.New("command does not exist")
	// ErrCommandAlreadyRunning is returned when a command is already active.
	ErrCommandAlreadyRunning = errors.New("command is already running")
	// ErrNoCommandProvided is returned when a start request has no executable.
	ErrNoCommandProvided = errors.New("no command provided")
	// ErrTelnetCredentialsRequired is returned when telnet input is selected
	// without connection settings.
	ErrTelnetCredentialsRequired = errors.New("telnet credentials required")
	// ErrTelnetPortRequired is returned when telnet input has no usable port.
	ErrTelnetPortRequired = errors.New("telnet port required")
	// ErrConsoleInputUnavailable is returned while a configured console input
	// transport has not attached or has disconnected. Callers may retry it.
	ErrConsoleInputUnavailable = errors.New("console input is temporarily unavailable")
)

var (
	// CurrentRuntime is the runtime detected from the current host OS.
	CurrentRuntime = setRuntime()
)

func setRuntime() Runtime {
	switch runtime.GOOS {
	case "linux":
		return RuntimeLinux
	case "windows":
		return RuntimeWindows
	case "darwin":
		return RuntimeDarwin
	default:
		return RuntimeUnknown
	}
}

// Instance tracks running commands within a Xylona supervisor.
type Instance struct {
	ctx             context.Context
	runningCommands map[string]*Command
	statusEventHook func(eventbus.StatusChangedEvent)
	RWMutex         *sync.RWMutex
}

// Command represents a managed process or internal task execution.
type Command struct {
	ID                            string
	executionID                   string
	User                          string
	BaseCommand                   string
	Args                          []string
	InternalCommand               bool
	gameServerName                string
	nodeID                        string
	internalCommandStdOut         io.Writer
	internalCommandStdErr         io.Writer
	internalGameServer            *models.GameServer
	gameID                        *string
	launchEnv                     map[string]string
	unixStartedAt                 int64
	status                        xylona.Status
	serviceID                     string
	currentCMD                    *exec.Cmd
	currentPTYCMD                 *pty.Cmd
	currentPTY                    pty.Pty
	outputListeners               map[string]chan *xylona.Message
	outputListenersLock           *sync.RWMutex
	statusListeners               map[string]chan *xylona.GameServerStatusUpdate
	statusListenersLock           *sync.RWMutex
	inputMethod                   InputMethod
	stdInWriter                   io.Writer
	combinedOutput                io.Reader
	stdout                        io.Reader
	stderr                        io.Reader
	telnetConn                    *telnet.Conn
	outBuffer                     string
	outputSequence                uint64
	preserveBufferedOutputOnReuse bool
	instanceCtx                   context.Context
	processCtx                    context.Context
	processCtxCancel              context.CancelFunc
	processGeneration             uint64
	executionMutex                *sync.Mutex
	toggleOutputType              chan struct{}
	telnetOutputActive            atomic.Bool
	stopTimeout                   time.Duration
	runAfterStartup               func(job *Command)
	intentionalStop               atomic.Bool
	previousStatus                xylona.Status
	transitionSequence            uint64
	lastExitCode                  int
	exitCodeKnown                 bool
	statusEventHook               func(eventbus.StatusChangedEvent)
	// Metrics fields (transient, not persisted to DB)
	cpuPercent      float64
	cpuCores        int32
	memoryRSS       uint64  // working set (WorkingSetSize on Windows)
	memoryVMS       uint64  // private committed memory (PagefileUsage on Windows)
	memoryPercent   float32 // % of total system RAM
	numThreads      int32
	diskUsageBytes  uint64
	workingDir      string
	ioReadRate      float64 // I/O read bytes/sec (disk + network)
	ioWriteRate     float64 // I/O write bytes/sec (disk + network)
	lastIORead      uint64  // previous cumulative read bytes
	lastIOWrite     uint64  // previous cumulative write bytes
	lastIOPollTime  time.Time
	connectionCount int32 // active TCP/UDP connections
	RWMutex         *sync.RWMutex
}

// SetStatusEventHook installs the direct lifecycle sink used by the owning
// node's replayable event emitter. Existing tracked commands are updated too.
func (i *Instance) SetStatusEventHook(hook func(eventbus.StatusChangedEvent)) {
	i.Lock()
	defer i.Unlock()
	i.statusEventHook = hook
	for _, command := range i.runningCommands {
		command.Lock()
		command.statusEventHook = hook
		command.Unlock()
	}
}

// LifecycleSnapshot is the authoritative lifecycle metadata retained for a
// command's current or most recently completed execution.
type LifecycleSnapshot struct {
	ExecutionID        string
	PreviousStatus     xylona.Status
	TransitionSequence uint64
	IntentionalStop    bool
	ExitCode           int
	ExitCodeKnown      bool
}

// Lock acquires the instance mutex.
func (i *Instance) Lock() {
	i.RWMutex.Lock()
}

// Unlock releases the instance mutex.
func (i *Instance) Unlock() {
	i.RWMutex.Unlock()
}

// RLock acquires the instance read mutex.
func (i *Instance) RLock() {
	i.RWMutex.RLock()
}

// RUnlock releases the instance read mutex.
func (i *Instance) RUnlock() {
	i.RWMutex.RUnlock()
}

// Lock acquires the command mutex.
func (c *Command) Lock() {
	c.RWMutex.Lock()
}

// Unlock releases the command mutex.
func (c *Command) Unlock() {
	c.RWMutex.Unlock()
}

// RLock acquires the command read mutex.
func (c *Command) RLock() {
	c.RWMutex.RLock()
}

// RUnlock releases the command read mutex.
func (c *Command) RUnlock() {
	c.RWMutex.RUnlock()
}

// Status returns the command's current status.
func (c *Command) Status() xylona.Status {
	c.RLock()
	defer c.RUnlock()
	return c.status
}

// NodeID returns the node ID associated with this command.
func (c *Command) NodeID() string {
	c.RLock()
	defer c.RUnlock()
	return c.nodeID
}

// GameServerName returns the game server name associated with this command.
func (c *Command) GameServerName() string {
	c.RLock()
	defer c.RUnlock()
	return c.gameServerName
}

// ServiceID returns the service ID associated with the command.
func (c *Command) ServiceID() string {
	c.RLock()
	defer c.RUnlock()
	return c.serviceID
}

// UnixStartedAt returns the UNIX timestamp when the command started.
func (c *Command) UnixStartedAt() int64 {
	c.RLock()
	defer c.RUnlock()
	return c.unixStartedAt
}

// IntentionalStop reports whether Stop was explicitly called on this command.
func (c *Command) IntentionalStop() bool {
	return c.intentionalStop.Load()
}

// Lifecycle returns a consistent copy of the command's retained lifecycle
// metadata. Terminal metadata remains available after the process is offline.
func (c *Command) Lifecycle() LifecycleSnapshot {
	c.RLock()
	defer c.RUnlock()
	return LifecycleSnapshot{
		ExecutionID:        c.executionID,
		PreviousStatus:     c.previousStatus,
		TransitionSequence: c.transitionSequence,
		IntentionalStop:    c.intentionalStop.Load(),
		ExitCode:           c.lastExitCode,
		ExitCodeKnown:      c.exitCodeKnown,
	}
}

// Metrics returns the core metrics snapshot for the command's process tree.
// memoryRSS is the total working set; memoryVMS is private committed memory.
func (c *Command) Metrics() (cpuPercent float64, memoryRSS uint64, memoryVMS uint64, memoryPercent float32, cpuCores int32, numThreads int32, diskUsageBytes uint64, ioReadRate float64, ioWriteRate float64, connectionCount int32) {
	c.RLock()
	defer c.RUnlock()
	return c.cpuPercent, c.memoryRSS, c.memoryVMS, c.memoryPercent, c.cpuCores, c.numThreads, c.diskUsageBytes, c.ioReadRate, c.ioWriteRate, c.connectionCount
}

// WorkingDir returns the command's working directory.
func (c *Command) WorkingDir() string {
	c.RLock()
	defer c.RUnlock()
	return c.workingDir
}

// AddOutputListener registers an output listener for command console messages.
func (c *Command) AddOutputListener(id string, outChan chan *xylona.Message) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	existing, exists := c.outputListeners[id]
	if exists && existing != outChan {
		close(existing)
	}
	c.outputListeners[id] = outChan
}

// AddOutputListenerWithReplay atomically registers an output listener and
// returns the retained console buffer represented by the latest sequence.
// Output emitted after the snapshot is guaranteed to arrive on outChan.
func (c *Command) AddOutputListenerWithReplay(id string, outChan chan *xylona.Message) *xylona.Message {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	existing, exists := c.outputListeners[id]
	if exists && existing != outChan {
		close(existing)
	}
	c.outputListeners[id] = outChan
	return &xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: c.ID,
			Output:       c.outBuffer,
			Sequence:     c.outputSequence,
			ResetBuffer:  true,
		},
	}
}

// RemoveOutputListener removes an output listener by ID.
func (c *Command) RemoveOutputListener(id string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	listener, exists := c.outputListeners[id]
	if !exists {
		return
	}
	delete(c.outputListeners, id)
	close(listener)
}

// AddStatusListener registers a listener for command status updates.
func (c *Command) AddStatusListener(id string, ch chan *xylona.GameServerStatusUpdate) {
	c.statusListenersLock.Lock()
	defer c.statusListenersLock.Unlock()
	c.statusListeners[id] = ch
}

// RemoveStatusListener removes a status listener by ID.
func (c *Command) RemoveStatusListener(id string) {
	c.statusListenersLock.Lock()
	defer c.statusListenersLock.Unlock()
	delete(c.statusListeners, id)
}

// New creates a new process supervisor instance.
func New(ctx context.Context) (*Instance, error) {
	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}
	return inst, nil
}
