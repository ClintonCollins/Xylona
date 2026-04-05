package rpc

import (
	"math"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestClampSearchTotalCount(t *testing.T) {
	tests := []struct {
		name      string
		totalHits int
		want      int32
	}{
		{name: "unknown total preserves sentinel", totalHits: modproviders.UnknownTotalHits, want: modproviders.UnknownTotalHits},
		{name: "other negative clamps to zero", totalHits: -2, want: 0},
		{name: "zero stays zero", totalHits: 0, want: 0},
		{name: "small value passes through", totalHits: 42, want: 42},
		{name: "max int32 passes through", totalHits: math.MaxInt32, want: math.MaxInt32},
		{name: "overflow clamps", totalHits: math.MaxInt32 + 1, want: math.MaxInt32},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampSearchTotalCount(tc.totalHits); got != tc.want {
				t.Fatalf("clampSearchTotalCount(%d) = %d, want %d", tc.totalHits, got, tc.want)
			}
		})
	}
}

func TestSearchModsRequiresServerPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.SearchModsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-other")

	_, errSearch := fixture.service.SearchMods(t.Context(), req)
	if errSearch == nil {
		t.Fatal("SearchMods() error = nil, want permission error")
	}
	if connect.CodeOf(errSearch) != connect.CodePermissionDenied {
		t.Fatalf("SearchMods() code = %v, want %v", connect.CodeOf(errSearch), connect.CodePermissionDenied)
	}
}
