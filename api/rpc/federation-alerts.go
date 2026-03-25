package rpc

import (
	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

var alertTopicToProtoType = alerts.AlertTopicToProtoType

var alertProtoTypeToTopic = alerts.AlertProtoTypeToTopic

var allAlertTopics = alerts.AllFederationAlertTopics

func isFederatedEvent(msg any) bool {
	return alerts.IsFederatedEvent(msg)
}

func serializeAlertEvent(topic string, msg any) (*xylona.FederationAlertEvent, bool) {
	return alerts.SerializeFederationAlertEvent(topic, msg)
}

func republishFederationAlertEvent(bus *eventbus.EventBus, evt *xylona.FederationAlertEvent) {
	alerts.RepublishFederationAlertEvent(bus, evt)
}
