package startargs

import "testing"

func TestParseTemplate(t *testing.T) {
	t.Run("empty template returns nil", func(t *testing.T) {
		got, errParse := ParseTemplate("")
		if errParse != nil {
			t.Fatalf("ParseTemplate() error = %v", errParse)
		}
		if got != nil {
			t.Errorf("ParseTemplate() = %#v, want nil", got)
		}
	})

	t.Run("valid template parses", func(t *testing.T) {
		templateJSON := `[{"id":"01TEST","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap"}]`

		got, errParse := ParseTemplate(templateJSON)
		if errParse != nil {
			t.Fatalf("ParseTemplate() error = %v", errParse)
		}
		if len(got) != 1 {
			t.Fatalf("ParseTemplate() len = %d, want 1", len(got))
		}
		if got[0].ID != "01TEST" {
			t.Errorf("ParseTemplate()[0].ID = %q, want %q", got[0].ID, "01TEST")
		}
		if got[0].Ownership != OwnershipEditable {
			t.Errorf("ParseTemplate()[0].Ownership = %q, want %q", got[0].Ownership, OwnershipEditable)
		}
	})
}

func TestParseBlocklist(t *testing.T) {
	t.Run("empty blocklist returns nil", func(t *testing.T) {
		got, errParse := ParseBlocklist("")
		if errParse != nil {
			t.Fatalf("ParseBlocklist() error = %v", errParse)
		}
		if got != nil {
			t.Errorf("ParseBlocklist() = %#v, want nil", got)
		}
	})

	t.Run("valid blocklist parses", func(t *testing.T) {
		blocklistJSON := `[{"pattern":"-agentlib:","reason":"debug agent"}]`

		got, errParse := ParseBlocklist(blocklistJSON)
		if errParse != nil {
			t.Fatalf("ParseBlocklist() error = %v", errParse)
		}
		if len(got) != 1 {
			t.Fatalf("ParseBlocklist() len = %d, want 1", len(got))
		}
		if got[0].Pattern != "-agentlib:" {
			t.Errorf("ParseBlocklist()[0].Pattern = %q, want %q", got[0].Pattern, "-agentlib:")
		}
	})
}
