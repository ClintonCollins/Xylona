package node

import (
	"time"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
)

func (n *Node) startStatusEventBridge() {
	if n == nil || n.events == nil {
		return
	}

	if n.supervisor == nil {
		return
	}
	n.supervisor.SetStatusEventHook(func(statusEvent eventbus.StatusChangedEvent) {
		n.events.Publish(Event{
			Type:               EventTypeProcessStatus,
			ProcessID:          statusEvent.ServerID,
			Status:             statusEvent.NewStatus,
			OldStatus:          statusEvent.OldStatus,
			ExecutionID:        statusEvent.ExecutionID,
			TransitionSequence: statusEvent.TransitionSequence,
			IntentionalStop:    statusEvent.IntentionalStop,
			ExitCode:           statusEvent.ExitCode,
			ExitCodeKnown:      statusEvent.ExitCodeKnown,
			Timestamp:          time.Now(),
		})
	})
}
