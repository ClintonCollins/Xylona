package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// runHubSpokeSetup spins up a controller + one remote xylona-node and drives
// the join-token bootstrap flow. After it returns, both processes are live,
// a remote node row exists in the controller's DB, and the controller has a
// live NodeClient for the remote node in its registry. State is persisted to
// <e2eDir>/.e2e-data-hub-spoke/ so the companion teardown can clean up.
func runHubSpokeSetup(ctx context.Context, httpPort, nodePort int, adminUsername, adminPassword, e2eDir, projectRoot string) error {
	backendURL := fmt.Sprintf("http://localhost:%d", httpPort)
	log.Info().Str("backend", backendURL).Int("node_port", nodePort).Msg("[Hub-Spoke Setup] Target")

	errLock := acquireLock(e2eDir, "hub-spoke", map[string]int{
		"http": httpPort,
		"node": nodePort,
	})
	if errLock != nil {
		return fmt.Errorf("acquire lock: %w", errLock)
	}
	setupOK := false
	defer func() {
		if !setupOK {
			releaseLock(e2eDir, "hub-spoke")
		}
	}()

	dataDir := filepath.Join(e2eDir, ".e2e-data-hub-spoke")
	controllerDir := filepath.Join(dataDir, "controller")
	nodeDir := filepath.Join(dataDir, "node")

	errMkdir := os.MkdirAll(controllerDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create controller data dir: %w", errMkdir)
	}
	errMkdirNode := os.MkdirAll(nodeDir, 0o750)
	if errMkdirNode != nil {
		return fmt.Errorf("create node data dir: %w", errMkdirNode)
	}

	dbFile := filepath.Join(controllerDir, "data.sqlite")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(dbFile + suffix)
	}
	// Clean any stale node identity so each run exercises a fresh bootstrap.
	_ = os.Remove(filepath.Join(nodeDir, "node-identity.json"))

	// Build all three binaries.
	errFE := buildFrontend(projectRoot)
	if errFE != nil {
		return fmt.Errorf("build frontend: %w", errFE)
	}
	xylonaExe := filepath.Join(controllerDir, binaryName("xylona"))
	errCtrl := buildXylona(projectRoot, xylonaExe)
	if errCtrl != nil {
		return fmt.Errorf("build xylona: %w", errCtrl)
	}
	nodeExe := filepath.Join(nodeDir, binaryName("xylona-node"))
	errNode := buildXylonaNode(projectRoot, nodeExe)
	if errNode != nil {
		return fmt.Errorf("build xylona-node: %w", errNode)
	}

	// Seed controller DB.
	log.Info().Msg("[Hub-Spoke Setup] Seeding controller database...")
	migrationsDir := filepath.Join(projectRoot, "sql", "migrations")
	errSeed := runSeed(dbFile, adminUsername, adminPassword, migrationsDir)
	if errSeed != nil {
		return fmt.Errorf("seed database: %w", errSeed)
	}

	// Start controller.
	controllerCmd, errStart := startNode(
		"Controller", controllerDir, e2eDir, xylonaExe,
		httpPort, 0,
		"DUMMY_GAME_ID=e2e-test-game",
		"XYLONA_VERSION_CHECK_INTERVAL=30s",
	)
	if errStart != nil {
		return fmt.Errorf("start controller: %w", errStart)
	}
	if controllerCmd.Process != nil {
		_ = os.WriteFile(filepath.Join(controllerDir, "xylona.pid"), []byte(strconv.Itoa(controllerCmd.Process.Pid)), 0o600)
	}

	// Kill the controller on any failure below.
	var nodeCmd *exec.Cmd
	defer func() {
		if setupOK {
			return
		}
		killProcess(controllerCmd)
		if nodeCmd != nil {
			killProcess(nodeCmd)
		}
	}()

	log.Info().Msg("[Hub-Spoke Setup] Waiting for controller to be ready...")
	errReady := waitForReady(ctx, backendURL, 30*time.Second)
	if errReady != nil {
		return fmt.Errorf("controller never became ready: %w", errReady)
	}

	// Issue a join token as the admin superuser.
	adminClient, errLogin := newAuthenticatedClient(ctx, backendURL, adminUsername, adminPassword)
	if errLogin != nil {
		return fmt.Errorf("admin login: %w", errLogin)
	}

	tokenResp, errToken := adminClient.rpc.GenerateNodePairingObject(ctx, connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{
		TargetUrl: backendURL,
	}))
	if errToken != nil {
		return fmt.Errorf("generate join token: %w", errToken)
	}
	joinToken := strings.TrimSpace(tokenResp.Msg.GetPairingToken())
	if joinToken == "" {
		return errors.New("controller returned empty join token")
	}
	log.Info().Msg("[Hub-Spoke Setup] Obtained join token; starting xylona-node...")

	nodeCmd, errNodeStart := startXylonaNode(
		nodeExe,
		backendURL,
		joinToken,
		nodePort,
		nodeDir,
		"e2e-remote",
	)
	if errNodeStart != nil {
		return fmt.Errorf("start xylona-node: %w", errNodeStart)
	}
	if nodeCmd.Process != nil {
		_ = os.WriteFile(filepath.Join(nodeDir, "xylona-node.pid"), []byte(strconv.Itoa(nodeCmd.Process.Pid)), 0o600)
	}

	// Wait for the node to appear in the controller's node list.
	log.Info().Msg("[Hub-Spoke Setup] Waiting for remote node to register...")
	errWaitNode := waitForRemoteNodeRegistered(ctx, adminClient, 60*time.Second)
	if errWaitNode != nil {
		return fmt.Errorf("remote node never registered: %w", errWaitNode)
	}

	log.Info().Msg("[Hub-Spoke Setup] Hub-spoke environment ready")
	setupOK = true
	return nil
}

// startXylonaNode launches a xylona-node binary against the given controller
// using the provided join token. Stdout/stderr are prefixed "[Node]" so the
// test log remains readable. The node writes its identity file under dataDir.
func startXylonaNode(nodeExe, controllerURL, joinToken string, listenPort int, dataDir, nodeName string) (*exec.Cmd, error) {
	listenAddr := fmt.Sprintf(":%d", listenPort)
	advertiseURL := fmt.Sprintf("https://127.0.0.1:%d", listenPort)

	args := []string{
		"--controller-url=" + controllerURL,
		"--join-token=" + joinToken,
		"--listen=" + listenAddr,
		"--advertise-url=" + advertiseURL,
		"--node-name=" + nodeName,
		"--data-dir=" + dataDir,
	}

	//nolint:noctx // node intentionally outlives the caller context until teardown.
	cmd := exec.Command(nodeExe, args...)
	cmd.Dir = dataDir

	stdout, errStdout := cmd.StdoutPipe()
	if errStdout != nil {
		return nil, fmt.Errorf("stdout pipe: %w", errStdout)
	}
	stderr, errStderr := cmd.StderrPipe()
	if errStderr != nil {
		return nil, fmt.Errorf("stderr pipe: %w", errStderr)
	}

	errStart := cmd.Start()
	if errStart != nil {
		return nil, fmt.Errorf("start xylona-node: %w", errStart)
	}

	go pipeWithPrefix("[Node]", stdout, os.Stdout)
	go pipeWithPrefix("[Node]", stderr, os.Stderr)

	return cmd, nil
}

func pipeWithPrefix(prefix string, src interface {
	Read([]byte) (int, error)
}, dst *os.File) {
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		fmt.Fprintf(dst, "%s %s\n", prefix, scanner.Text())
	}
}

// waitForRemoteNodeRegistered polls the controller's ListNodes RPC until at
// least one node beyond the controller's self-node appears. Times out after
// the supplied deadline so a broken bootstrap doesn't hang the harness.
func waitForRemoteNodeRegistered(ctx context.Context, client *e2eClient, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for remote node canceled: %w", ctx.Err())
		default:
		}

		resp, errList := client.rpc.ListNodes(ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
		if errList == nil {
			if len(resp.Msg.GetNodes()) >= 2 {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("remote node did not register within %s", timeout)
}
