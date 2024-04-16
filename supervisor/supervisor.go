package supervisor

import (
	"context"
	"io"
	"os/exec"
	"runtime"
	"sync"
)

type Runtime string

const (
	RuntimeLinux   Runtime = "linux"
	RuntimeWindows Runtime = "windows"
	RuntimeDarwin  Runtime = "darwin"
	RuntimeUnknown Runtime = "unknown"
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
	currentCMD          *exec.Cmd
	outputListeners     map[string]chan string
	outputListenersLock *sync.RWMutex
	stdInWriter         io.Writer
	combinedOutput      io.Reader
	outBuffer           string
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	callbackFunc        func(job *Command)
	*sync.RWMutex
}

func (c *Command) AddPersistentOutputStreamListener(id string, outChan chan string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	c.outputListeners[id] = outChan
}

func (c *Command) RemovePersistentOutputStreamListener(id string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	delete(c.outputListeners, id)
}

func (c *Command) AddOutputListener(id string, outChan chan string) {
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
