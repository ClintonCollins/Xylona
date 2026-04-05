package rpc

import (
	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

var allAlertTopics = alerts.AllFederationAlertTopics

func serializeAlertEvent(topic string, msg any) (*xylona.FederationAlertEvent, bool) {
	return alerts.SerializeFederationAlertEvent(topic, msg)
}
