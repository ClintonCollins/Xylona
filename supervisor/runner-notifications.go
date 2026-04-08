package supervisor

import (
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (c *Command) closeJobNotification() {
	c.sendJobNotification(MessageStoppedServer)
	oldStatus := c.Status()
	c.sendJobStatusNotification(oldStatus, xylona.Status_OFFLINE)
}

func (c *Command) sendJobStatusNotification(oldStatus, newStatus xylona.Status) {
	c.handleOutputListeners(&xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId: c.ID,
			Status:       newStatus,
		},
	})
	c.handleStatusListeners(newStatus)

	// Publish status change to the event bus for alert evaluation.
	// Skip no-op transitions (e.g., OFFLINE→OFFLINE from concurrent shutdown paths).
	if oldStatus != newStatus {
		eb := eventbus.Get()
		eb.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
			ServerID:     c.ID,
			ServerNodeID: c.nodeID,
			OldStatus:    oldStatus.String(),
			NewStatus:    newStatus.String(),
		})
	}
}

func (c *Command) handleStatusListeners(status xylona.Status) {
	update := &xylona.GameServerStatusUpdate{
		GameServerId: c.ID,
		Status:       status,
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
