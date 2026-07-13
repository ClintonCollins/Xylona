package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	"github.com/ziutek/telnet"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	consoleReadBufferBytes = 32 << 10
	maxConsoleRecordBytes  = 256 << 10
)

func (c *Command) jobOutputReaders() (io.Reader, io.Reader, <-chan struct{}, bool, bool) {
	c.RLock()
	defer c.RUnlock()

	if c.currentCMD == nil && c.currentPTYCMD == nil && !c.InternalCommand {
		return nil, nil, nil, false, false
	}
	if c.stdout == nil || c.stderr == nil {
		return nil, nil, nil, false, false
	}
	return c.stdout, c.stderr, c.processCtx.Done(), c.currentPTY != nil, true
}

// readJobOut reads the output of the current command execution.
// It scans the combined output, splits it by lines, and processes each line.
// It pushes each line to the output buffer and handles the output listeners.
// If an error occurs while scanning the output, it logs the error.
// If the context is done, it stops reading the output.
// It closes the job notification after reading all the output.
func (c *Command) readJobOut() {
	log.Debug().Str("Game Server ID", c.ID).Msg("Reading job output")
	stdoutReader, stderrReader, processDone, pseudoTerminal, ok := c.jobOutputReaders()
	if !ok {
		return
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go c.scanJobOutput(stdoutReader, "stdout", processDone, pseudoTerminal, wg)
	go c.scanJobOutput(stderrReader, "stderr", processDone, pseudoTerminal, wg)

	wg.Wait()
	log.Debug().Str("Game Server ID", c.ID).Msg("Job output listener stopped")
	c.closeJobNotification()
}

func (c *Command) scanJobOutput(
	reader io.Reader,
	logField string,
	processDone <-chan struct{},
	pseudoTerminal bool,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	errRead := readConsoleRecords(reader, func(output string) bool {
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
			return false
		case <-processDone:
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
			return false
		default:
			if c.telnetOutputActive.Load() {
				return true
			}
			log.Debug().Str("ID", c.ID).Str(logField, output).Msg("Output")
			c.sendJobNotification(output)
			return true
		}
	})

	if errRead != nil && !errors.Is(errRead, context.Canceled) {
		if pseudoTerminal && (errors.Is(errRead, syscall.EIO) || errors.Is(errRead, os.ErrClosed)) {
			log.Debug().Err(errRead).Msg("Pseudo-terminal output stream closed")
		} else {
			log.Error().Err(errRead).Msg("Error reading output")
		}
	}
}

// readTelnetOutput reads the output of one attached telnet connection.
func (c *Command) readTelnetOutput(telnetConnection *telnet.Conn, processDone <-chan struct{}) error {
	log.Debug().Str("Game Server ID", c.ID).Msg("Telnet is running")
	errRead := readConsoleRecords(telnetConnection, func(telnetOut string) bool {
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal.  Closing telnet reader.")
			return false
		case <-processDone:
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing telnet reader.")
			return false
		default:
		}
		c.sendJobNotification(telnetOut)
		return true
	})
	log.Debug().Str("Game Server ID", c.ID).Msg("Telnet listener stopped")
	return errRead
}

// readConsoleRecords emits newline- or carriage-return-delimited records.
// Oversized records are emitted in bounded chunks so one pathological game
// server write cannot terminate console delivery or grow memory without bound.
func readConsoleRecords(reader io.Reader, handle func(string) bool) error {
	readBuffer := make([]byte, consoleReadBufferBytes)
	record := make([]byte, 0, consoleReadBufferBytes)
	skipLineFeed := false
	recordChunkEmitted := false

	emitPrefix := func(length int) bool {
		output := strings.ToValidUTF8(string(record[:length]), "\uFFFD")
		if !handle(output) {
			return false
		}
		copy(record, record[length:])
		record = record[:len(record)-length]
		return true
	}
	emitDelimiter := func() bool {
		if len(record) > 0 {
			if !emitPrefix(len(record)) {
				return false
			}
		} else if !recordChunkEmitted && !handle("") {
			return false
		}
		recordChunkEmitted = false
		return true
	}

	for {
		bytesRead, errRead := reader.Read(readBuffer)
		for _, value := range readBuffer[:bytesRead] {
			if skipLineFeed {
				skipLineFeed = false
				if value == '\n' {
					continue
				}
			}

			switch value {
			case '\r':
				if !emitDelimiter() {
					return context.Canceled
				}
				skipLineFeed = true
			case '\n':
				if !emitDelimiter() {
					return context.Canceled
				}
			default:
				record = append(record, value)
				if len(record) >= maxConsoleRecordBytes {
					chunkLength := consoleChunkBoundary(record, maxConsoleRecordBytes)
					if chunkLength == 0 {
						continue
					}
					if !emitPrefix(chunkLength) {
						return context.Canceled
					}
					recordChunkEmitted = true
				}
			}
		}

		if errRead != nil {
			if len(record) > 0 && !emitPrefix(len(record)) {
				return context.Canceled
			}
			if errors.Is(errRead, io.EOF) {
				return nil
			}
			return fmt.Errorf("read console record: %w", errRead)
		}
	}
}

func consoleChunkBoundary(data []byte, limit int) int {
	boundary := 0
	for boundary < len(data) && boundary < limit {
		remaining := data[boundary:]
		if !utf8.FullRune(remaining) {
			break
		}
		_, runeBytes := utf8.DecodeRune(remaining)
		if boundary+runeBytes > limit {
			break
		}
		boundary += runeBytes
	}
	return boundary
}

func (c *Command) handleOutputListeners(payload *xylona.Message) {
	c.outputListenersLock.RLock()
	listenerIDsToRemove := c.deliverOutputListenersLocked(payload)
	c.outputListenersLock.RUnlock()
	c.removeSlowOutputListeners(listenerIDsToRemove)
}

func (c *Command) deliverOutputListenersLocked(payload *xylona.Message) []string {
	listenerIDsToRemove := make([]string, 0)
	for id, listener := range c.outputListeners {
		select {
		case listener <- payload:
		default:
			listenerIDsToRemove = append(listenerIDsToRemove, id)
		}
	}
	return listenerIDsToRemove
}

func (c *Command) removeSlowOutputListeners(listenerIDsToRemove []string) {
	for _, id := range listenerIDsToRemove {
		log.Warn().Str("ID", id).Str("Game Server ID", c.ID).
			Msg("Closing slow output listener so the subscriber can resynchronize")
		c.RemoveOutputListener(id)
	}
}

func (c *Command) sendJobNotification(message string) {
	message = strings.ToValidUTF8(message, "\uFFFD")
	c.outputListenersLock.Lock()
	c.pushToOutputBufferLocked(message)
	payload := &xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: c.ID,
			Output:       message + "\n",
			Sequence:     c.outputSequence,
		},
	}
	listenerIDsToRemove := c.deliverOutputListenersLocked(payload)
	c.outputListenersLock.Unlock()
	c.removeSlowOutputListeners(listenerIDsToRemove)
}

// SendOutput injects a message into the command's console output stream,
// writing it to the output buffer and broadcasting it to all output listeners.
// This allows external callers (e.g., the actions layer) to surface status
// messages in the game server console without routing through stdin.
func (c *Command) SendOutput(message string) {
	c.Lock()
	if c.currentCMD == nil && c.currentPTYCMD == nil && c.status == xylona.Status_OFFLINE {
		c.preserveBufferedOutputOnReuse = true
	}
	c.Unlock()
	c.sendJobNotification(message)
}

func (c *Command) pushToOutputBuffer(output string) {
	c.outputListenersLock.Lock()
	defer c.outputListenersLock.Unlock()
	c.pushToOutputBufferLocked(output)
}

func (c *Command) pushToOutputBufferLocked(output string) {
	output = strings.ToValidUTF8(output, "\uFFFD")
	c.outBuffer += output + "\n"
	if len(c.outBuffer) > maxOutputBufferBytes {
		start := len(c.outBuffer) - maxOutputBufferBytes
		for start < len(c.outBuffer) && !utf8.RuneStart(c.outBuffer[start]) {
			start++
		}
		c.outBuffer = c.outBuffer[start:]
	}
	c.outputSequence++
}

// GetOutputBuffer returns the buffered command output.
func (c *Command) GetOutputBuffer() string {
	c.outputListenersLock.RLock()
	defer c.outputListenersLock.RUnlock()
	return c.outBuffer
}

// GetOutputSnapshot returns a consistent console buffer and sequence pair.
func (c *Command) GetOutputSnapshot() (string, uint64) {
	c.outputListenersLock.RLock()
	defer c.outputListenersLock.RUnlock()
	return c.outBuffer, c.outputSequence
}

// formatXylonaMessage produces a timestamped Xylona console line.
func formatXylonaMessage(message string) string {
	return fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), message)
}
