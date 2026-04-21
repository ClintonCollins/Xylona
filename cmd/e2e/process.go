package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

func generateBase64Key(length int) (string, error) {
	b := make([]byte, length)
	_, errRead := rand.Read(b)
	if errRead != nil {
		return "", fmt.Errorf("generate random key: %w", errRead)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func startNode(name, workDir, logDir, xylonaExe string, httpPort int, extraEnv ...string) (*exec.Cmd, error) {
	cookieHashKey, errCookie := generateBase64Key(64)
	if errCookie != nil {
		return nil, errCookie
	}
	cookieBlockKey, errBlock := generateBase64Key(32)
	if errBlock != nil {
		return nil, errBlock
	}
	jwtSecretKey, errJWT := generateBase64Key(64)
	if errJWT != nil {
		return nil, errJWT
	}

	cmd := exec.Command(xylonaExe) //nolint:noctx // the test server process intentionally outlives the caller context until teardown.
	cmd.Dir = workDir
	baseEnv := []string{
		"DB_FILE_PATH=" + filepath.Join(workDir, "data.sqlite"),
		"HTTP_PORT=" + strconv.Itoa(httpPort),
		"COOKIE_HASH_KEY_BASE64=" + cookieHashKey,
		"COOKIE_BLOCK_KEY_BASE64=" + cookieBlockKey,
		"JWT_SECRET_KEY_BASE64=" + jwtSecretKey,
		"SECURE_COOKIES=false",
		"E2E_LOG_FILE=" + filepath.Join(logDir, "backend.log"),
	}
	cmd.Env = append(os.Environ(), append(baseEnv, extraEnv...)...)

	stdout, errStdout := cmd.StdoutPipe()
	if errStdout != nil {
		return nil, fmt.Errorf("stdout pipe: %w", errStdout)
	}
	stderr, errStderr := cmd.StderrPipe()
	if errStderr != nil {
		return nil, fmt.Errorf("stderr pipe: %w", errStderr)
	}

	log.Info().Msgf("[%s] Starting on HTTP :%d", name, httpPort)

	errStart := cmd.Start()
	if errStart != nil {
		return nil, fmt.Errorf("start %s: %w", name, errStart)
	}

	// Pipe stdout with prefix.
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Printf("[%s] %s\n", name, scanner.Text())
		}
	}()

	// Pipe stderr with prefix.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", name, scanner.Text())
		}
	}()

	return cmd, nil
}

func killByPIDFile(pidFile, label string) {
	data, errRead := os.ReadFile(pidFile)
	if errRead != nil {
		if !os.IsNotExist(errRead) {
			log.Warn().Err(errRead).Str("file", pidFile).Msg("Could not read PID file")
		}
		return
	}

	pidStr := strings.TrimSpace(string(data))
	pid, errParse := strconv.Atoi(pidStr)
	if errParse != nil {
		log.Warn().Str("pid", pidStr).Msg("Invalid PID in file")
		return
	}

	log.Info().Msgf("[Teardown] Killing %s (PID %d)...", label, pid)

	if runtime.GOOS == "windows" {
		//nolint:gosec,noctx // PID is parsed as a positive integer from our own pid file; taskkill has no context-aware API on Windows.
		killCmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F", "/T")
		errKill := killCmd.Run()
		if errKill != nil {
			proc, errFind := os.FindProcess(pid)
			if errFind == nil {
				_ = proc.Kill()
			}
		}
	} else {
		proc, errFind := os.FindProcess(pid)
		if errFind == nil {
			_ = proc.Signal(syscall.SIGTERM)
			time.Sleep(500 * time.Millisecond)
			_ = proc.Kill()
		}
	}

	_ = os.Remove(pidFile)
}

func waitForReady(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for ready cancelled: %w", ctx.Err())
		default:
		}

		resp, errGet := client.Get(url) //nolint:noctx // health check polling, context not needed
		if errGet == nil {
			_ = resp.Body.Close()
			// Any response means the server is up.
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("node at %s did not become ready within %s", url, timeout)
}
