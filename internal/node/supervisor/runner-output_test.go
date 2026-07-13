package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestReadConsoleRecords(t *testing.T) {
	longRecord := strings.Repeat("x", maxConsoleRecordBytes+17)
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "line feed and unterminated tail",
			input: "one\ntwo",
			want:  []string{"one", "two"},
		},
		{
			name:  "carriage return records",
			input: "one\rtwo\rthree\r",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "crlf is one delimiter",
			input: "one\r\ntwo\r\n",
			want:  []string{"one", "two"},
		},
		{
			name:  "blank line records are preserved",
			input: "one\n\ntwo",
			want:  []string{"one", "", "two"},
		},
		{
			name:  "blank crlf records are preserved once",
			input: "\r\n\r\n",
			want:  []string{"", ""},
		},
		{
			name:  "oversized record is bounded",
			input: longRecord,
			want:  []string{longRecord[:maxConsoleRecordBytes], longRecord[maxConsoleRecordBytes:]},
		},
		{
			name:  "exact bounded record does not invent blank line",
			input: strings.Repeat("x", maxConsoleRecordBytes) + "\n",
			want:  []string{strings.Repeat("x", maxConsoleRecordBytes)},
		},
		{
			name:  "multibyte rune is not split at chunk boundary",
			input: strings.Repeat("x", maxConsoleRecordBytes-1) + "🎮\n",
			want: []string{
				strings.Repeat("x", maxConsoleRecordBytes-1),
				"🎮",
			},
		},
		{
			name:  "invalid source bytes are replaced",
			input: string([]byte{'a', 0xff, 'b', '\n'}),
			want:  []string{"a\uFFFDb"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make([]string, 0)
			errRead := readConsoleRecords(strings.NewReader(test.input), func(record string) bool {
				got = append(got, record)
				return true
			})
			if errRead != nil {
				t.Fatalf("readConsoleRecords() error = %v", errRead)
			}
			if len(got) != len(test.want) {
				t.Fatalf("record count = %d, want %d", len(got), len(test.want))
			}
			for index := range test.want {
				if !utf8.ValidString(got[index]) {
					t.Fatalf("record[%d] is not valid UTF-8", index)
				}
				if got[index] != test.want[index] {
					t.Fatalf("record[%d] length = %d, want %d", index, len(got[index]), len(test.want[index]))
				}
			}
		})
	}
}

func TestCommandConsoleBufferAndPayloadRemainValidUTF8(t *testing.T) {
	command := &Command{
		ID:                  "valid-utf8",
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		RWMutex:             &sync.RWMutex{},
	}
	listener := make(chan *xylona.Message, 1)
	command.AddOutputListener("valid-utf8", listener)
	command.sendJobNotification(string([]byte{'a', 0xff, 'b'}))
	payload := (<-listener).GetGameServerConsoleOutput().GetOutput()
	if !utf8.ValidString(payload) || payload != "a\uFFFDb\n" {
		t.Fatalf("console payload = %q, want valid replacement text", payload)
	}

	command.pushToOutputBuffer("a" + "🎮" + strings.Repeat("x", maxOutputBufferBytes-4))
	buffer := command.GetOutputBuffer()
	if !utf8.ValidString(buffer) {
		t.Fatal("truncated console buffer is not valid UTF-8")
	}
	if len(buffer) > maxOutputBufferBytes {
		t.Fatalf("truncated console buffer bytes = %d, want at most %d", len(buffer), maxOutputBufferBytes)
	}
}

type failingInputWriter struct {
	err error
}

func (w *failingInputWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestCommandSendInputRejectsUnavailableWriters(t *testing.T) {
	writeFailure := errors.New("input pipe closed")
	tests := []struct {
		name      string
		command   *Command
		wantError error
	}{
		{
			name: "offline synthetic command",
			command: &Command{
				stdInWriter: &strings.Builder{},
				RWMutex:     &sync.RWMutex{},
			},
			wantError: ErrConsoleInputUnavailable,
		},
		{
			name: "telnet has not attached",
			command: &Command{
				currentCMD:  &exec.Cmd{},
				inputMethod: InputMethod{Type: InputTypeTelnet},
				RWMutex:     &sync.RWMutex{},
			},
			wantError: ErrConsoleInputUnavailable,
		},
		{
			name: "stdin write fails without a working console mirror",
			command: &Command{
				currentCMD:  &exec.Cmd{},
				stdInWriter: &failingInputWriter{err: writeFailure},
				inputMethod: InputMethod{Type: InputTypeStdIn},
				RWMutex:     &sync.RWMutex{},
			},
			wantError: ErrConsoleInputUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errSend := test.command.SendInput("status")
			if errSend == nil {
				t.Fatal("SendInput() error = nil")
			}
			if test.wantError != nil && !errors.Is(errSend, test.wantError) {
				t.Fatalf("SendInput() error = %v, want errors.Is(%v)", errSend, test.wantError)
			}
		})
	}
}

func TestCommandOutputReplayAndSlowListenerRecovery(t *testing.T) {
	command := &Command{
		ID:                  "console-replay",
		instanceCtx:         t.Context(),
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		RWMutex:             &sync.RWMutex{},
	}

	slow := make(chan *xylona.Message)
	command.AddOutputListener("slow", slow)
	command.sendJobNotification("before replay")
	_, open := <-slow
	if open {
		t.Fatal("slow listener remained open after detachment")
	}

	live := make(chan *xylona.Message, 1)
	replay := command.AddOutputListenerWithReplay("live", live)
	replayOutput := replay.GetGameServerConsoleOutput()
	if !replayOutput.GetResetBuffer() || replayOutput.GetOutput() != "before replay\n" || replayOutput.GetSequence() != 1 {
		t.Fatalf("replay output = %+v", replayOutput)
	}

	command.sendJobNotification("after replay")
	message := <-live
	liveOutput := message.GetGameServerConsoleOutput()
	if liveOutput.GetResetBuffer() || liveOutput.GetSequence() != 2 || liveOutput.GetOutput() != "after replay\n" {
		t.Fatalf("live output = %+v", liveOutput)
	}
}

func TestCommandOutputReplayHasContiguousSequenceBoundary(t *testing.T) {
	command := &Command{
		ID:                  "console-sequence",
		instanceCtx:         t.Context(),
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		RWMutex:             &sync.RWMutex{},
	}
	for sequence := 1; sequence <= 50; sequence++ {
		command.sendJobNotification(fmt.Sprintf("line-%d", sequence))
	}

	halfway := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for sequence := 51; sequence <= 100; sequence++ {
			command.sendJobNotification(fmt.Sprintf("line-%d", sequence))
			if sequence == 75 {
				close(halfway)
				<-release
			}
		}
	}()
	<-halfway
	live := make(chan *xylona.Message, 32)
	replay := command.AddOutputListenerWithReplay("live", live)
	replayOutput := replay.GetGameServerConsoleOutput()
	if replayOutput.GetSequence() != 75 || !replayOutput.GetResetBuffer() {
		t.Fatalf("replay boundary = %+v, want reset at sequence 75", replayOutput)
	}
	if !strings.Contains(replayOutput.GetOutput(), "line-75\n") || strings.Contains(replayOutput.GetOutput(), "line-76\n") {
		t.Fatalf("replay buffer did not end at the replay sequence")
	}
	close(release)
	<-done

	for wantSequence := uint64(76); wantSequence <= 100; wantSequence++ {
		select {
		case message := <-live:
			output := message.GetGameServerConsoleOutput()
			if output.GetResetBuffer() || output.GetSequence() != wantSequence {
				t.Fatalf("live sequence = %+v, want %d", output, wantSequence)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for live sequence %d", wantSequence)
		}
	}
}

type stagedConsoleReader struct {
	stage          int
	firstProcessed chan struct{}
	releaseSecond  chan struct{}
}

func (r *stagedConsoleReader) Read(buffer []byte) (int, error) {
	switch r.stage {
	case 0:
		r.stage++
		return copy(buffer, "suppressed\n"), nil
	case 1:
		r.stage++
		close(r.firstProcessed)
		<-r.releaseSecond
		return copy(buffer, "visible\n"), io.EOF
	default:
		return 0, io.EOF
	}
}

func TestCommandScanJobOutputResumesAfterTelnetDisconnect(t *testing.T) {
	reader := &stagedConsoleReader{
		firstProcessed: make(chan struct{}),
		releaseSecond:  make(chan struct{}),
	}
	command := &Command{
		ID:                  "telnet-stdout-resume",
		instanceCtx:         t.Context(),
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		RWMutex:             &sync.RWMutex{},
	}
	command.telnetOutputActive.Store(true)
	processDone := make(chan struct{})
	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(1)
	done := make(chan struct{})
	go func() {
		command.scanJobOutput(reader, "stdout", processDone, false, waitGroup)
		waitGroup.Wait()
		close(done)
	}()

	<-reader.firstProcessed
	command.telnetOutputActive.Store(false)
	close(reader.releaseSecond)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stdout reader did not finish")
	}
	output := command.GetOutputBuffer()
	if strings.Contains(output, "suppressed") || !strings.Contains(output, "visible\n") {
		t.Fatalf("console output = %q, want only resumed stdout", output)
	}
}

func TestTelnetConsoleReconnectsWhileProcessLives(t *testing.T) {
	listenConfig := &net.ListenConfig{}
	listener, errListen := listenConfig.Listen(t.Context(), "tcp", "localhost:0")
	if errListen != nil {
		t.Fatalf("listen: %v", errListen)
	}
	t.Cleanup(func() {
		errClose := listener.Close()
		if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Errorf("close listener: %v", errClose)
		}
	})

	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address type = %T", listener.Addr())
	}
	instanceCtx, cancelInstance := context.WithCancel(t.Context())
	processCtx, cancelProcess := context.WithCancel(instanceCtx)
	t.Cleanup(cancelInstance)
	t.Cleanup(cancelProcess)
	command := &Command{
		ID:         "telnet-reconnect",
		currentCMD: &exec.Cmd{},
		inputMethod: InputMethod{
			Type: InputTypeTelnet,
			TelnetCredentials: &TelnetCredentials{
				Port: tcpAddress.Port,
			},
		},
		instanceCtx:         instanceCtx,
		processCtx:          processCtx,
		processGeneration:   1,
		RWMutex:             &sync.RWMutex{},
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
	}

	managerDone := make(chan struct{})
	go func() {
		defer close(managerDone)
		connectTelnetAndSetAsStdinWriter(command)
	}()

	first := acceptTelnetTestConnection(t, listener)
	waitForTelnetAttached(t, command, 5*time.Second)
	errSendFirst := command.SendInput("status-one")
	if errSendFirst != nil {
		t.Fatalf("send first telnet input: %v", errSendFirst)
	}
	if got := readTelnetTestInput(t, first); got != "status-one\n" {
		t.Fatalf("first telnet input = %q, want %q", got, "status-one\\n")
	}
	_, errWriteFirst := first.Write([]byte("first telnet record\r"))
	if errWriteFirst != nil {
		t.Fatalf("write first telnet output: %v", errWriteFirst)
	}
	waitForCommandOutput(t, command, "first telnet record")
	errCloseFirst := first.Close()
	if errCloseFirst != nil {
		t.Fatalf("close first telnet connection: %v", errCloseFirst)
	}
	waitForCommandOutput(t, command, "Telnet console disconnected")
	waitForTelnetDetached(t, command, 5*time.Second)
	errUnavailable := command.SendInput("status-unavailable")
	if !errors.Is(errUnavailable, ErrConsoleInputUnavailable) {
		t.Fatalf("SendInput() during reconnect error = %v, want %v", errUnavailable, ErrConsoleInputUnavailable)
	}

	second := acceptTelnetTestConnection(t, listener)
	waitForTelnetAttached(t, command, 5*time.Second)
	errSendSecond := command.SendInput("status-two")
	if errSendSecond != nil {
		t.Fatalf("send second telnet input: %v", errSendSecond)
	}
	if got := readTelnetTestInput(t, second); got != "status-two\n" {
		t.Fatalf("second telnet input = %q, want %q", got, "status-two\\n")
	}
	_, errWriteSecond := second.Write([]byte("second telnet record\r"))
	if errWriteSecond != nil {
		t.Fatalf("write second telnet output: %v", errWriteSecond)
	}
	waitForCommandOutput(t, command, "second telnet record")

	cancelProcess()
	errCloseSecond := second.Close()
	if errCloseSecond != nil {
		t.Fatalf("close second telnet connection: %v", errCloseSecond)
	}
	select {
	case <-managerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("telnet manager did not stop with process context")
	}
}

func TestTelnetManagerStopsWithOriginalExecutionAfterCommandReuse(t *testing.T) {
	listenConfig := &net.ListenConfig{}
	listener, errListen := listenConfig.Listen(t.Context(), "tcp", "localhost:0")
	if errListen != nil {
		t.Fatalf("listen: %v", errListen)
	}
	t.Cleanup(func() {
		errClose := listener.Close()
		if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Errorf("close listener: %v", errClose)
		}
	})
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address type = %T", listener.Addr())
	}
	instanceCtx, cancelInstance := context.WithCancel(t.Context())
	oldProcessCtx, cancelOldProcess := context.WithCancel(instanceCtx)
	newProcessCtx, cancelNewProcess := context.WithCancel(instanceCtx)
	t.Cleanup(cancelInstance)
	t.Cleanup(cancelOldProcess)
	t.Cleanup(cancelNewProcess)
	command := &Command{
		ID:                  "telnet-reuse",
		currentCMD:          &exec.Cmd{},
		inputMethod:         InputMethod{Type: InputTypeTelnet, TelnetCredentials: &TelnetCredentials{Port: tcpAddress.Port}},
		instanceCtx:         instanceCtx,
		processCtx:          oldProcessCtx,
		processCtxCancel:    cancelOldProcess,
		processGeneration:   1,
		RWMutex:             &sync.RWMutex{},
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
	}
	execution := captureTelnetExecution(command)
	managerDone := make(chan struct{})
	go func() {
		defer close(managerDone)
		connectTelnetForExecution(command, execution)
	}()
	first := acceptTelnetTestConnection(t, listener)
	waitForTelnetAttached(t, command, 5*time.Second)

	command.Lock()
	command.processCtx = newProcessCtx
	command.processCtxCancel = cancelNewProcess
	command.processGeneration = 2
	command.telnetConn = nil
	command.stdInWriter = nil
	command.telnetOutputActive.Store(false)
	command.Unlock()
	cancelOldProcess()
	errCloseFirst := first.Close()
	if errCloseFirst != nil {
		t.Fatalf("close old telnet connection: %v", errCloseFirst)
	}
	select {
	case <-managerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("old telnet manager followed replacement process context")
	}
	select {
	case <-newProcessCtx.Done():
		t.Fatal("old telnet manager canceled replacement process context")
	default:
	}
}

type blockingInputWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockingInputWriter) Write(buffer []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(buffer), nil
}

func TestCommandStopKeepsCancellationBoundToOriginalExecution(t *testing.T) {
	instanceCtx, cancelInstance := context.WithCancel(t.Context())
	oldProcessCtx, cancelOldProcess := context.WithCancel(instanceCtx)
	newProcessCtx, cancelNewProcess := context.WithCancel(instanceCtx)
	t.Cleanup(cancelInstance)
	t.Cleanup(cancelOldProcess)
	t.Cleanup(cancelNewProcess)
	writer := &blockingInputWriter{entered: make(chan struct{}), release: make(chan struct{})}
	command := &Command{
		ID:                  "stop-reuse",
		User:                "test-user",
		currentCMD:          &exec.Cmd{},
		stdInWriter:         writer,
		instanceCtx:         instanceCtx,
		processCtx:          oldProcessCtx,
		processCtxCancel:    cancelOldProcess,
		processGeneration:   1,
		stopTimeout:         10 * time.Millisecond,
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		RWMutex:             &sync.RWMutex{},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		command.Stop("stop")
	}()
	<-writer.entered
	command.Lock()
	command.processCtx = newProcessCtx
	command.processCtxCancel = cancelNewProcess
	command.processGeneration = 2
	command.Unlock()
	close(writer.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not finish")
	}
	select {
	case <-oldProcessCtx.Done():
	default:
		t.Fatal("Stop() did not cancel original process after timeout")
	}
	select {
	case <-newProcessCtx.Done():
		t.Fatal("Stop() canceled replacement process")
	default:
	}
}

func TestReportUnexpectedProcessExitSurfacesUnknownWaitFailure(t *testing.T) {
	tests := []struct {
		name               string
		status             xylona.Status
		intentional        bool
		wantMessage        bool
		wantCrash          bool
		wantLifecycleKnown bool
	}{
		{
			name:               "online unintentional wait failure",
			status:             xylona.Status_ONLINE,
			wantMessage:        true,
			wantCrash:          true,
			wantLifecycleKnown: true,
		},
		{
			name:               "update wait failure remains an operation error",
			status:             xylona.Status_UPDATING,
			wantMessage:        true,
			wantLifecycleKnown: true,
		},
		{name: "intentional stop", status: xylona.Status_ONLINE, intentional: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bus := eventbus.Get()
			crashEvents := bus.SubscribeReliable(eventbus.TopicGameServerCrashed)
			defer bus.Unsubscribe(eventbus.TopicGameServerCrashed, crashEvents)
			command := &Command{
				ID:                  "unknown-exit",
				nodeID:              "node-unknown-exit",
				status:              test.status,
				outputListeners:     make(map[string]chan *xylona.Message),
				outputListenersLock: &sync.RWMutex{},
				RWMutex:             &sync.RWMutex{},
			}
			command.intentionalStop.Store(test.intentional)
			lifecycleCode, lifecycleKnown := lifecycleExitDetails(
				command,
				errors.New("wait transport failed"),
				-1,
				false,
			)
			if lifecycleCode != -1 || lifecycleKnown != test.wantLifecycleKnown {
				t.Fatalf("lifecycle exit = (%d, %t), want (-1, %t)", lifecycleCode, lifecycleKnown, test.wantLifecycleKnown)
			}
			reportUnexpectedProcessExit(command, errors.New("wait transport failed"), -1, false)
			hasMessage := strings.Contains(command.GetOutputBuffer(), "exit code unavailable")
			if hasMessage != test.wantMessage {
				t.Fatalf("unknown-exit message present = %t, want %t; output %q", hasMessage, test.wantMessage, command.GetOutputBuffer())
			}
			if test.wantCrash {
				select {
				case rawCrash := <-crashEvents:
					crash, ok := rawCrash.(eventbus.ServerCrashedEvent)
					if !ok || crash.ServerID != command.ID || crash.ServerNodeID != command.nodeID || crash.ExitCode != -1 {
						t.Fatalf("unknown-exit crash = %+v", rawCrash)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for unknown-exit crash event")
				}
			} else {
				select {
				case rawCrash := <-crashEvents:
					t.Fatalf("non-gameplay unknown exit published crash: %+v", rawCrash)
				case <-time.After(50 * time.Millisecond):
				}
			}
		})
	}
}

func acceptTelnetTestConnection(t *testing.T, listener net.Listener) net.Conn {
	t.Helper()
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		t.Fatalf("listener type = %T", listener)
	}
	errDeadline := tcpListener.SetDeadline(time.Now().Add(5 * time.Second))
	if errDeadline != nil {
		t.Fatalf("set accept deadline: %v", errDeadline)
	}
	connection, errAccept := listener.Accept()
	if errAccept != nil {
		t.Fatalf("accept telnet connection: %v", errAccept)
	}
	return connection
}

func waitForTelnetAttached(t *testing.T, command *Command, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		command.RLock()
		attached := command.telnetConn != nil && command.stdInWriter != nil
		command.RUnlock()
		if attached {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for telnet input attachment")
}

func waitForTelnetDetached(t *testing.T, command *Command, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		command.RLock()
		detached := command.telnetConn == nil && command.stdInWriter == nil
		command.RUnlock()
		if detached && !command.telnetOutputActive.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for telnet input detachment")
}

func readTelnetTestInput(t *testing.T, connection net.Conn) string {
	t.Helper()
	errDeadline := connection.SetReadDeadline(time.Now().Add(time.Second))
	if errDeadline != nil {
		t.Fatalf("set telnet read deadline: %v", errDeadline)
	}
	buffer := make([]byte, 128)
	bytesRead, errRead := connection.Read(buffer)
	if errRead != nil {
		t.Fatalf("read telnet input: %v", errRead)
	}
	return string(buffer[:bytesRead])
}
