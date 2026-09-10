package supervisor

import (
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (c *Command) closeJobNotification() {
	c.sendJobNotification(MessageStoppedServer)
}

func (c *Command) sendJobStatusNotification(oldStatus, newStatus xylona.Status) {
	c.sendJobStatusNotificationDetails(oldStatus, newStatus, 0, false)
}

// sendJobStatusNotificationWithExit is like sendJobStatusNotification but lets
// the supervisor runner propagate the process exit code when the transition
// is a shutdown (NewStatus=OFFLINE). Auto-restart uses this to tell a clean
// stop from a crash.
func (c *Command) sendJobStatusNotificationWithExit(oldStatus xylona.Status, exitCode int) {
	c.sendJobStatusNotificationDetails(oldStatus, xylona.Status_OFFLINE, exitCode, true)
}

func (c *Command) sendJobStatusNotificationWithExitDetails(oldStatus xylona.Status, exitCode int, exitCodeKnown bool) {
	c.sendJobStatusNotificationDetails(oldStatus, xylona.Status_OFFLINE, exitCode, exitCodeKnown)
}

func (c *Command) sendJobStatusNotificationDetails(
	oldStatus xylona.Status,
	newStatus xylona.Status,
	exitCode int,
	exitCodeKnown bool,
) {
	c.RLock()
	suppressStatusEvents := c.suppressStatusEvents
	c.RUnlock()
	if suppressStatusEvents {
		return
	}

	gameServerName := c.GameServerName()
	nodeID := c.NodeID()

	// Status updates are delivered via two dedicated channels:
	//  1. handleStatusListeners — per-server subscribers
	//  2. eventbus publish below — WebSocket broadcastGameServerStatus (all clients)
	// They are NOT sent through handleOutputListeners because that channel is
	// reserved for console text. Sending status frames through it would cause
	// duplicate delivery to console-subscribed clients.
	c.handleStatusListeners(newStatus)

	// Publish status change to the event bus for alert evaluation.
	// Skip no-op transitions (e.g., OFFLINE→OFFLINE from concurrent shutdown paths).
	if oldStatus != newStatus {
		c.Lock()
		c.previousStatus = oldStatus
		c.transitionSequence++
		sequence := c.transitionSequence
		executionID := c.executionID
		c.lastExitCode = exitCode
		c.exitCodeKnown = exitCodeKnown
		c.Unlock()
		statusEvent := eventbus.StatusChangedEvent{
			ServerID:           c.ID,
			ServerName:         gameServerName,
			ServerNodeID:       nodeID,
			OldStatus:          oldStatus.String(),
			NewStatus:          newStatus.String(),
			ExecutionID:        executionID,
			TransitionSequence: sequence,
			IntentionalStop:    c.IntentionalStop(),
			ExitCode:           exitCode,
			ExitCodeKnown:      exitCodeKnown,
		}
		eb := eventbus.Get()
		eb.Publish(eventbus.TopicGameServerStatusChanged, statusEvent)
		c.RLock()
		statusEventHook := c.statusEventHook
		c.RUnlock()
		if statusEventHook != nil {
			statusEventHook(statusEvent)
		}
	}
}

func (c *Command) handleStatusListeners(status xylona.Status) {
	gameServerName := c.GameServerName()
	update := &xylona.GameServerStatusUpdate{
		GameServerId:   c.ID,
		Status:         status,
		GameServerName: gameServerName,
	}
	listenerIDsToRemove := make([]string, 0)
	c.statusListenersLock.RLock()
	for id, listener := range c.statusListeners {
		select {
		case listener <- update:
		default:
			listenerIDsToRemove = append(listenerIDsToRemove, id)
		}
	}
	c.statusListenersLock.RUnlock()
	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing slow status listener")
		c.RemoveStatusListener(id)
	}
}
