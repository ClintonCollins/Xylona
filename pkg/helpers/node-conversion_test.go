package helpers

import (
	"testing"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestNodeProtoToModel(t *testing.T) {
	tests := []struct {
		name          string
		input         *xylona.Node
		wantName      string
		wantListenURL string
	}{
		{
			name: "trims name and listen URL",
			input: &xylona.Node{
				Id:      "remote-1",
				Name:    "  Remote Node  ",
				BaseUrl: "  https://panel.example.com  ",
			},
			wantName:      "Remote Node",
			wantListenURL: "https://panel.example.com",
		},
		{
			name: "empty listen URL is preserved",
			input: &xylona.Node{
				Id:      "local-1",
				Name:    "localhost",
				BaseUrl: "",
			},
			wantName:      "localhost",
			wantListenURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NodeProtoToModel(tt.input)

			if got.Name != tt.wantName {
				t.Errorf("NodeProtoToModel().Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.ListenURL != tt.wantListenURL {
				t.Errorf("NodeProtoToModel().ListenURL = %q, want %q", got.ListenURL, tt.wantListenURL)
			}
			if !got.Enabled {
				t.Errorf("NodeProtoToModel().Enabled = false, want true")
			}
		})
	}
}

func TestNodeModelToSetter(t *testing.T) {
	input := &models.Node{
		ID:        "remote-1",
		Name:      "Remote",
		ListenURL: "https://panel.example.com",
		Enabled:   true,
	}

	setter := NodeModelToSetter(input)

	gotListenURL, okListenURL := setter.ListenURL.Get()
	if !okListenURL {
		t.Fatalf("NodeModelToSetter().ListenURL should be set")
	}
	if gotListenURL != "https://panel.example.com" {
		t.Errorf("NodeModelToSetter().ListenURL = %q, want %q", gotListenURL, "https://panel.example.com")
	}

	gotName, okName := setter.Name.Get()
	if !okName {
		t.Fatalf("NodeModelToSetter().Name should be set")
	}
	if gotName != "Remote" {
		t.Errorf("NodeModelToSetter().Name = %q, want %q", gotName, "Remote")
	}
}
