package rpc

import (
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestValheimOperationsAuthorizationAndAvailability(t *testing.T) {
	fixture, _, client := newPrivateReadGateFixture(t)
	_, errUpdate := fixture.conn.SQLDb.ExecContext(t.Context(), "UPDATE game_server SET game_id = ? WHERE id = ?", "valheim", "server-local-1")
	if errUpdate != nil {
		t.Fatal(errUpdate)
	}
	ids := []string{}
	for _, operation := range gameintegrations.OperationsForGame("valheim") {
		ids = append(ids, operation.ID)
	}
	client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{{GameID: "valheim", OperationIDs: ids}}
	for _, user := range []string{"user-owner", "user-other"} {
		if user == "user-other" {
			grantGameServerRole(t, fixture, user, "viewer")
		}
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, user)
		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatal(errList)
		}
		if user == "user-other" {
			if len(response.Msg.GetOperations()) != 0 {
				t.Fatal("viewer received access management")
			}
			continue
		}
		if len(response.Msg.GetOperations()) != 9 {
			t.Fatalf("operations = %d", len(response.Msg.GetOperations()))
		}
		for _, operation := range response.Msg.GetOperations() {
			_, action, _ := gameintegrations.ValheimAccessOperation(operation.GetId())
			if operation.GetAvailable() != (action == "list") {
				t.Fatalf("running availability: %+v", operation)
			}
		}
	}
	if len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
		t.Fatal("Valheim used native dashboard")
	}
	client.RuntimeCapabilitiesResult.ProtocolVersion = 13
	request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
	if errList != nil {
		t.Fatal(errList)
	}
	for _, operation := range response.Msg.GetOperations() {
		if operation.GetAvailable() {
			t.Fatal("old node enabled stored access")
		}
	}
}
