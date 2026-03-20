package supervisor

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/ziutek/telnet"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type Runtime string

const (
	RuntimeLinux   Runtime = "linux"
	RuntimeWindows Runtime = "windows"
	RuntimeDarwin  Runtime = "darwin"
	RuntimeUnknown Runtime = "unknown"
)

var (
	ErrCommandDoesNotExist = errors.New("command does not exist")
)

var (
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

type Instance struct {
	ctx             context.Context
	runningCommands map[string]*Command
	*sync.RWMutex
}

type Command struct {
	ID                    string
	User                  string
	FullCommandAndArgs    string
	InternalCommand       bool
	internalCommandStdOut io.Writer
	internalCommandStdErr io.Writer
	internalGameServer    *models.GameServer
	gameID                *string
	unixStartedAt         int64
	status                xylona.Status
	serviceID             string
	currentCMD            *exec.Cmd
	outputListeners       map[string]chan *xylona.Message
	outputListenersLock   *sync.RWMutex
	statusListeners       map[string]chan *xylona.GameServerStatusUpdate
	statusListenersLock   *sync.RWMutex
	inputMethod           InputMethod
	stdInWriter           io.Writer
	combinedOutput        io.Reader
	stdout                io.Reader
	stderr                io.Reader
	telnetConn            *telnet.Conn
	outBuffer             string
	instanceCtx           context.Context
	processCtx            context.Context
	processCtxCancel      context.CancelFunc
	toggleOutputType      chan struct{}
	runAfterStartup       func(job *Command)
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
	*sync.RWMutex
}

func (c *Command) Status() xylona.Status {
	c.RLock()
	defer c.RUnlock()
	return c.status
}

func (c *Command) ServiceID() string {
	c.RLock()
	defer c.RUnlock()
	return c.serviceID
}

func (c *Command) UnixStartedAt() int64 {
	c.RLock()
	defer c.RUnlock()
	return c.unixStartedAt
}

// Metrics returns the core metrics snapshot for the command's process tree.
// memoryRSS is the total working set; memoryVMS is private committed memory.
func (c *Command) Metrics() (cpuPercent float64, memoryRSS uint64, memoryVMS uint64, memoryPercent float32, cpuCores int32, numThreads int32, diskUsageBytes uint64, ioReadRate float64, ioWriteRate float64, connectionCount int32) {
	c.RLock()
	defer c.RUnlock()
	return c.cpuPercent, c.memoryRSS, c.memoryVMS, c.memoryPercent, c.cpuCores, c.numThreads, c.diskUsageBytes, c.ioReadRate, c.ioWriteRate, c.connectionCount
}

func (c *Command) WorkingDir() string {
	c.RLock()
	defer c.RUnlock()
	return c.workingDir
}

func (c *Command) AddOutputListener(id string, outChan chan *xylona.Message) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	c.outputListeners[id] = outChan
}

func (c *Command) RemoveOutputListener(id string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	delete(c.outputListeners, id)
}

func (c *Command) AddStatusListener(id string, ch chan *xylona.GameServerStatusUpdate) {
	c.statusListenersLock.Lock()
	defer c.statusListenersLock.Unlock()
	c.statusListeners[id] = ch
}

func (c *Command) RemoveStatusListener(id string) {
	c.statusListenersLock.Lock()
	defer c.statusListenersLock.Unlock()
	delete(c.statusListeners, id)
}

func New(ctx context.Context) (*Instance, error) {
	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}
	return inst, nil
}
