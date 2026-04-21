package nodeclient

import (
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/node"
)

func TestFakeNodeClientRecordsNewFileAPIs(t *testing.T) {
	fake := &FakeNodeClient{
		StreamWriteFileResult: node.WriteFileResult{BytesWritten: 7, SHA256: "sum"},
		CopyFilesResult:       []string{"dst.txt"},
		ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
			Found:      true,
			Version:    "123",
			SourcePath: "appmanifest_1.acf",
		},
	}

	writeResult, errWrite := fake.StreamWriteFile(t.Context(), "/srv", "dst.txt", strings.NewReader("payload"), node.ProtectionPolicy{ServerExecutable: "server.jar"})
	if errWrite != nil {
		t.Fatalf("StreamWriteFile error = %v", errWrite)
	}
	if writeResult.BytesWritten != 7 || writeResult.SHA256 != "sum" {
		t.Fatalf("StreamWriteFile result = %+v, want configured result", writeResult)
	}
	if len(fake.StreamWriteFileCalls) != 1 {
		t.Fatalf("StreamWriteFileCalls len = %d, want 1", len(fake.StreamWriteFileCalls))
	}
	if string(fake.StreamWriteFileCalls[0].Content) != "payload" {
		t.Fatalf("recorded stream content = %q, want payload", string(fake.StreamWriteFileCalls[0].Content))
	}
	if fake.StreamWriteFileCalls[0].Policy.ServerExecutable != "server.jar" {
		t.Fatalf("recorded stream policy = %+v, want server.jar", fake.StreamWriteFileCalls[0].Policy)
	}

	copied, errCopy := fake.CopyFiles(t.Context(), "/srv", []node.CopyFileOperation{{SourceRelativePath: "src.txt", DestinationRelativePath: "dst.txt"}}, node.ProtectionPolicy{BaseCommand: "./run.sh"})
	if errCopy != nil {
		t.Fatalf("CopyFiles error = %v", errCopy)
	}
	if len(copied) != 1 || copied[0] != "dst.txt" {
		t.Fatalf("CopyFiles result = %v, want [dst.txt]", copied)
	}
	if fake.CopyFilesCalls[0].Operations[0].DestinationRelativePath != "dst.txt" {
		t.Fatalf("recorded copy operations = %+v, want destination", fake.CopyFilesCalls[0].Operations)
	}
	if fake.CopyFilesCalls[0].Policy.BaseCommand != "./run.sh" {
		t.Fatalf("recorded copy policy = %+v, want ./run.sh", fake.CopyFilesCalls[0].Policy)
	}

	probe, errProbe := fake.ProbeInstalledVersion(t.Context(), node.InstalledVersionProbeRequest{
		Directory: "/srv",
		Kind:      node.InstalledVersionProbeKindSteamManifest,
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion error = %v", errProbe)
	}
	if !probe.Found || probe.Version != "123" {
		t.Fatalf("ProbeInstalledVersion result = %+v, want configured result", probe)
	}
	if len(fake.ProbeInstalledVersionCalls) != 1 {
		t.Fatalf("ProbeInstalledVersionCalls len = %d, want 1", len(fake.ProbeInstalledVersionCalls))
	}
}
