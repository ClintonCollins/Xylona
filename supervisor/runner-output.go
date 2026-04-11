package supervisor

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (c *Command) jobOutputReaders() (io.Reader, io.Reader, bool) {
	c.RLock()
	defer c.RUnlock()

	if c.currentCMD == nil && !c.InternalCommand {
		return nil, nil, false
	}
	if c.stdout == nil || c.stderr == nil {
		return nil, nil, false
	}
	return c.stdout, c.stderr, true
}

// readJobOut reads the output of the current command execution.
// It scans the combined output, splits it by lines, and processes each line.
// It pushes each line to the output buffer and handles the output listeners.
// If an error occurs while scanning the output, it logs the error.
// If the context is done, it stops reading the output.
// It closes the job notification after reading all the output.
func (c *Command) readJobOut() {
	log.Debug().Str("Game Server ID", c.ID).Msg("Reading job output")
	stdoutReader, stderrReader, ok := c.jobOutputReaders()
	if !ok {
		return
	}
	var disableOutput atomic.Bool
	scannerDone := make(chan struct{}, 2)

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go func() {
		defer wg.Done()
		scannersFinished := 0
		for {
			if scannersFinished == 2 {
				return
			}
			select {
			case <-c.instanceCtx.Done():
				return
			case <-c.processCtx.Done():
				return
			case <-scannerDone:
				scannersFinished++
			case <-c.toggleOutputType:
				disableOutput.Store(!disableOutput.Load())
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer func() {
			scannerDone <- struct{}{}
		}()
		scannerStdOut := bufio.NewScanner(stdoutReader)
		scannerStdOut.Split(bufio.ScanLines)
		for scannerStdOut.Scan() {
			if scannerStdOut.Err() != nil {
				log.Error().Err(scannerStdOut.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
				return
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			default:
				if disableOutput.Load() {
					continue
				}
				stdOut := scannerStdOut.Text()
				log.Debug().Str("ID", c.ID).Str("stdout", stdOut).Msg("Output")
				c.sendJobNotification(stdOut)
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer func() {
			scannerDone <- struct{}{}
		}()
		scannerStdErr := bufio.NewScanner(stderrReader)
		scannerStdErr.Split(bufio.ScanLines)
		for scannerStdErr.Scan() {
			if scannerStdErr.Err() != nil {
				log.Error().Err(scannerStdErr.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
				return
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			default:
				if disableOutput.Load() {
					continue
				}
				stdErr := scannerStdErr.Text()
				log.Debug().Str("ID", c.ID).Str("stderr", stdErr).Msg("Output")
				c.sendJobNotification(stdErr)
			}
		}
	}()

	wg.Wait()
	log.Debug().Str("Game Server ID", c.ID).Msg("Job output listener stopped")
	c.processCtxCancel()
	c.closeJobNotification()
}

// readTelnetOutput reads the output of the telnet connection.
func (c *Command) readTelnetOutput() {
	retries := 60
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Wait for telnet to start.
	for {
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job telnet reader.")
			return
		case <-c.processCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job telnet reader.")
			return
		case <-ticker.C:
			log.Debug().Str("Game Server ID", c.ID).Msg("Checking if telnet is running")
		}
		if c.telnetConn != nil {
			log.Debug().Str("Game Server ID", c.ID).Msg("Telnet is running")
			c.toggleOutputType <- struct{}{}
			break
		}
		retries--
		if retries <= 0 {
			log.Debug().Str("Game Server ID", c.ID).Msg("Telnet did not start")
			return
		}
	}

	scanner := bufio.NewScanner(c.telnetConn)
	// scanner.Buffer(make([]byte, 16), 65536)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if scanner.Err() != nil {
			log.Error().Err(scanner.Err()).Msg("Error scanning telnet")
			return
		}
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal.  Closing telnet reader.")
			return
		case <-c.processCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing telnet reader.")
			return
		default:
		}
		telnetOut := scanner.Text()
		c.sendJobNotification(telnetOut)
	}
	log.Debug().Str("Game Server ID", c.ID).Msg("Telnet listener stopped")
}

func (c *Command) handleOutputListeners(payload *xylona.Message) {
	var listenerIDsToRemove []string
	var removeLock sync.Mutex

	c.outputListenersLock.RLock()
	errGroup, ctx := errgroup.WithContext(c.instanceCtx)
	for id, listener := range c.outputListeners {
		errGroup.Go(func() error {
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", id).Msg("Received Xylona shutdown signal. Closing output listener.")
				return nil
			case <-ctx.Done():
				log.Debug().Str("Game Server ID", id).Msg("Received error group context shutdown signal. Closing output listener.")
				return nil
			case listener <- payload:
				// Give the channel receiver 500 milliseconds to handle the output, otherwise we discard the message.
			case <-time.After(time.Second * 1):
				removeLock.Lock()
				listenerIDsToRemove = append(listenerIDsToRemove, id)
				removeLock.Unlock()
				return nil
			}
			return nil
		})
	}
	c.outputListenersLock.RUnlock()
	_ = errGroup.Wait()

	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing output listener")
		c.RemoveOutputListener(id)
	}
}

func (c *Command) sendJobNotification(message string) {
	c.pushToOutputBuffer(message)
	c.handleOutputListeners(&xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: c.ID,
			Output:       message + "\n",
		},
	})
}

// SendOutput injects a message into the command's console output stream,
// writing it to the output buffer and broadcasting it to all output listeners.
// This allows external callers (e.g., the actions layer) to surface status
// messages in the game server console without routing through stdin.
func (c *Command) SendOutput(message string) {
	c.Lock()
	if c.currentCMD == nil && c.status == xylona.Status_OFFLINE {
		c.preserveBufferedOutputOnReuse = true
	}
	c.Unlock()
	c.sendJobNotification(message)
}

func (c *Command) pushToOutputBuffer(output string) {
	c.Lock()
	defer c.Unlock()
	if len(c.outBuffer) > maxOutputBufferBytes {
		c.outBuffer = c.outBuffer[len(c.outBuffer)-maxOutputBufferBytes:]
	}
	c.outBuffer += output + "\n"
}

// GetOutputBuffer returns the buffered command output.
func (c *Command) GetOutputBuffer() string {
	c.RLock()
	defer c.RUnlock()
	return c.outBuffer
}

// formatXylonaMessage produces a timestamped Xylona console line.
func formatXylonaMessage(message string) string {
	return fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), message)
}
