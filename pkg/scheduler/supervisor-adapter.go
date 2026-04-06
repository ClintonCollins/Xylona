package scheduler

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// supervisorAdapter wraps supervisor.Instance to satisfy the SupervisorAccessor
// interface without creating a direct dependency on the concrete type in the
// scheduler's API.
type supervisorAdapter struct {
	inst *supervisor.Instance
}

// NewSupervisorAdapter wraps a supervisor.Instance to implement SupervisorAccessor.
func NewSupervisorAdapter(inst *supervisor.Instance) SupervisorAccessor {
	return &supervisorAdapter{inst: inst}
}

func (a *supervisorAdapter) GetCommandByID(commandID string) (SupervisorCommand, error) {
	cmd, err := a.inst.GetCommandByID(commandID)
	if err != nil {
		return nil, fmt.Errorf("get command %s: %w", commandID, err)
	}
	return &commandAdapter{cmd: cmd}, nil
}

// commandAdapter wraps supervisor.Command to satisfy the SupervisorCommand interface.
type commandAdapter struct {
	cmd *supervisor.Command
}

func (c *commandAdapter) Status() xylona.Status {
	return c.cmd.Status()
}

func (c *commandAdapter) SendInput(input string) error {
	errSend := c.cmd.SendInput(input)
	if errSend != nil {
		return fmt.Errorf("send input: %w", errSend)
	}
	return nil
}
