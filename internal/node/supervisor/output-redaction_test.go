package supervisor

import (
	"io"
	"strings"
	"sync"
	"testing"
)

func TestValheimOutputSecretRedaction(t *testing.T) {
	c := &Command{RWMutex: &sync.RWMutex{}, serviceID: "valheim", redactValues: []string{"private-secret"}}
	reader := io.MultiReader(strings.NewReader("before private-"), strings.NewReader("secret after"))
	output, errRead := io.ReadAll(c.redactedOutputReader(reader))
	if errRead != nil {
		t.Fatal(errRead)
	}
	if string(output) != "before [redacted] after" {
		t.Fatalf("unexpected sanitized output %q", output)
	}
}
