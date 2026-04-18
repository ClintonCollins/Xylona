package node

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
)

func (n *Node) startStatusEventBridge() {
	if n == nil || n.events == nil {
		return
	}

	baseCtx := n.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	eb := eventbus.Get()
	statusChanged := eb.SubscribeReliable(eventbus.TopicGameServerStatusChanged)

	go func() {
		defer eb.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusChanged)

		for {
			select {
			case <-baseCtx.Done():
				return
			case data, ok := <-statusChanged:
				if !ok {
					return
				}

				statusEvent, ok := data.(eventbus.StatusChangedEvent)
				if !ok {
					log.Error().Msg("node: failed to cast status event")
					continue
				}

				n.events.Publish(Event{
					Type:      EventTypeProcessStatus,
					ProcessID: statusEvent.ServerID,
					Status:    statusEvent.NewStatus,
					Timestamp: time.Now(),
				})
			}
		}
	}()
}
