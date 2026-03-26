package startargs

import "testing"

func TestCompileBlocklistAndValidate(t *testing.T) {
	t.Run("empty blocklist allows everything", func(t *testing.T) {
		compiled, errCompile := CompileBlocklist(nil)
		if errCompile != nil {
			t.Fatalf("CompileBlocklist() error = %v", errCompile)
		}

		violation := compiled.Validate([]string{"-Xmx2G"})
		if violation != nil {
			t.Errorf("Validate() = %#v, want nil", violation)
		}
	})

	t.Run("single pattern blocks matching token", func(t *testing.T) {
		compiled := mustCompileBlocklist(t, []BlocklistEntry{
			{Pattern: "-agentlib:", Reason: "debug agent"},
		})

		violation := compiled.Validate([]string{"-agentlib:jdwp=transport=dt_socket"})
		if violation == nil {
			t.Fatal("Validate() = nil, want violation")
		}
		if violation.Reason != "debug agent" {
			t.Errorf("Validate().Reason = %q, want %q", violation.Reason, "debug agent")
		}
	})

	t.Run("single pattern allows non matching token", func(t *testing.T) {
		compiled := mustCompileBlocklist(t, []BlocklistEntry{
			{Pattern: "-agentlib:", Reason: "debug agent"},
		})

		violation := compiled.Validate([]string{"-Xmx2G"})
		if violation != nil {
			t.Errorf("Validate() = %#v, want nil", violation)
		}
	})

	t.Run("first matching pattern wins", func(t *testing.T) {
		compiled := mustCompileBlocklist(t, []BlocklistEntry{
			{Pattern: "-Dlog4j2\\.", Reason: "managed log4j"},
			{Pattern: "-D", Reason: "generic system property"},
		})

		violation := compiled.Validate([]string{"-Dlog4j2.configurationFile=foo.xml"})
		if violation == nil {
			t.Fatal("Validate() = nil, want violation")
		}
		if violation.Pattern != "-Dlog4j2\\." {
			t.Errorf("Validate().Pattern = %q, want %q", violation.Pattern, "-Dlog4j2\\.")
		}
	})

	t.Run("invalid regex returns compile error", func(t *testing.T) {
		_, errCompile := CompileBlocklist([]BlocklistEntry{
			{Pattern: "[", Reason: "broken"},
		})
		if errCompile == nil {
			t.Fatal("CompileBlocklist() error = nil, want error")
		}
	})
}

func mustCompileBlocklist(t *testing.T, entries []BlocklistEntry) *CompiledBlocklist {
	t.Helper()

	compiled, errCompile := CompileBlocklist(entries)
	if errCompile != nil {
		t.Fatalf("CompileBlocklist() error = %v", errCompile)
	}

	return compiled
}
