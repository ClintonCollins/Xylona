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
	"strconv"
	"strings"
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

func startNode(name, workDir, xylonaExe string, httpPort, fedPort int) (*exec.Cmd, error) {
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

	cmd := exec.Command(xylonaExe)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"DB_FILE_PATH="+filepath.Join(workDir, "data.sqlite"),
		"HTTP_PORT="+strconv.Itoa(httpPort),
		"FEDERATION_PORT="+strconv.Itoa(fedPort),
		"COOKIE_HASH_KEY_BASE64="+cookieHashKey,
		"COOKIE_BLOCK_KEY_BASE64="+cookieBlockKey,
		"JWT_SECRET_KEY_BASE64="+jwtSecretKey,
		"SECURE_COOKIES=false",
	)

	stdout, errStdout := cmd.StdoutPipe()
	if errStdout != nil {
		return nil, fmt.Errorf("stdout pipe: %w", errStdout)
	}
	stderr, errStderr := cmd.StderrPipe()
	if errStderr != nil {
		return nil, fmt.Errorf("stderr pipe: %w", errStderr)
	}

	log.Info().Msgf("[Federation Setup] Starting %s on HTTP :%d, Federation :%d", name, httpPort, fedPort)

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

	log.Info().Msgf("[Federation Teardown] Killing %s (PID %d)...", label, pid)

	// On Windows, use taskkill to kill the process tree.
	killCmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F", "/T")
	errKill := killCmd.Run()
	if errKill != nil {
		// Fallback: try os.Process.Kill.
		proc, errFind := os.FindProcess(pid)
		if errFind == nil {
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
			return ctx.Err()
		default:
		}

		resp, errGet := client.Get(url)
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
