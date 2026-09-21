package nodeclient

import (
	"testing"

	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestValheimAccessListBounds(t *testing.T) {
	_, errDecode := valheimAccessListFromProto(&nodeprotov1.ValheimAccessList{Identities: make([]string, 4097)})
	if errDecode == nil {
		t.Fatal("oversized access list accepted")
	}
}
