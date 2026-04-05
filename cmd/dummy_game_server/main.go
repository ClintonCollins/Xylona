// Command dummy_game_server provides a small controllable process for tests.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	heartbeat := flag.Duration("heartbeat", 5*time.Second, "heartbeat interval (0 to disable)")
	startupDelay := flag.Duration("startup-delay", 0, "delay before startup banner")
	flag.Parse()

	if *startupDelay > 0 {
		time.Sleep(*startupDelay)
	}

	startTime := time.Now()
	pid := os.Getpid()

	fmt.Printf("[dummy-game-server] started pid=%d\n", pid)

	shutdownCh := make(chan int, 1)

	// Signal handler goroutine
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("[dummy-game-server] received signal, shutting down")
		shutdownCh <- 0
	}()

	// Stdin reader goroutine
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, " ", 2)
			cmd := parts[0]
			arg := ""
			if len(parts) > 1 {
				arg = parts[1]
			}

			switch cmd {
			case "stop":
				fmt.Println("[dummy-game-server] goodbye")
				shutdownCh <- 0
				return
			case "echo":
				fmt.Println(arg)
			case "status":
				uptime := time.Since(startTime).Round(time.Millisecond)
				fmt.Printf("[dummy-game-server] pid=%d uptime=%s status=running\n", pid, uptime)
			case "crash":
				fmt.Fprintln(os.Stderr, "[dummy-game-server] crashing")
				os.Exit(1)
			case "stderr":
				fmt.Fprintln(os.Stderr, arg)
			case "flood":
				for i := range 100 {
					fmt.Printf("[dummy-game-server] flood line %d\n", i+1)
				}
			default:
				fmt.Printf("[dummy-game-server] unknown command: %s\n", cmd)
			}
		}
		if errScan := scanner.Err(); errScan != nil {
			fmt.Fprintf(os.Stderr, "[dummy-game-server] stdin read error: %v\n", errScan)
			shutdownCh <- 1
			return
		}
		// EOF — supervisor closed stdin (force-kill path)
		fmt.Println("[dummy-game-server] stdin closed, shutting down")
		shutdownCh <- 0
	}()

	// Main select loop
	if *heartbeat <= 0 {
		code := <-shutdownCh
		os.Exit(code)
	}

	ticker := time.NewTicker(*heartbeat)
	defer ticker.Stop()
	for {
		select {
		case code := <-shutdownCh:
			os.Exit(code) //nolint:gocritic // intentional exit in signal handler; ticker defer cleanup is irrelevant
		case <-ticker.C:
			uptime := time.Since(startTime).Round(time.Millisecond)
			fmt.Printf("[dummy-game-server] heartbeat pid=%d uptime=%s\n", pid, uptime)
		}
	}
}
