package supervisor

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"sync"

	"github.com/ziutek/telnet"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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
	ID                  string
	User                string
	FullCommandAndArgs  string
	UnixStartedAt       int64
	Status              xylona.Status
	ServiceID           string
	currentCMD          *exec.Cmd
	outputListeners     map[string]chan xylona.Message
	outputListenersLock *sync.RWMutex
	inputMethod         InputMethod
	stdInWriter         io.Writer
	combinedOutput      io.Reader
	telnetConn          *telnet.Conn
	outBuffer           string
	instanceCtx         context.Context
	processCtx          context.Context
	processCtxCancel    context.CancelFunc
	toggleOutputType    chan struct{}
	callbackFunc        func(job *Command)
	runAfterStartup     func(job *Command)
	*sync.RWMutex
}

func (c *Command) AddOutputListener(id string, outChan chan xylona.Message) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	c.outputListeners[id] = outChan
}

func (c *Command) RemoveOutputListener(id string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	delete(c.outputListeners, id)
}

func New(ctx context.Context) (*Instance, error) {
	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}
	return inst, nil
}
