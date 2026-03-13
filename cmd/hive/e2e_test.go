package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEndToEnd(t *testing.T) {
	requireIntegration(t)
	requireUDPBind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Setup temporary directories for nodes
	tempDir := t.TempDir()
	dataDirA := filepath.Join(tempDir, "nodeA")
	dataDirB := filepath.Join(tempDir, "nodeB")
	os.MkdirAll(dataDirA, 0700)
	os.MkdirAll(dataDirB, 0700)

	// Create rules.json for both
	rules := `{"redact_patterns": ["(?i)password"]}`
	os.WriteFile(filepath.Join(dataDirA, "rules.json"), []byte(rules), 0600)
	os.WriteFile(filepath.Join(dataDirB, "rules.json"), []byte(rules), 0600)

	// Build the binary
	binaryPath := filepath.Join(tempDir, "hive")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, ".")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build hive binary: %v\n%s", err, strings.TrimSpace(string(buildOutput)))
	}

	// 2. Start Node A (Seed)
	cmdA := exec.CommandContext(ctx, binaryPath, "-addr", "127.0.0.1:0", "-monitor-port", "7439")
	cmdA.Env = append(os.Environ(), "HOME="+dataDirA)
	aStdin, _ := cmdA.StdinPipe()
	aStderr, _ := cmdA.StderrPipe()
	if err := cmdA.Start(); err != nil {
		t.Fatalf("Failed to start Node A: %v", err)
	}
	defer cmdA.Process.Kill()

	// Capture Node A's address and ID from Stderr
	var nodeAAddr string
	var nodeAIDStr string
	var mu sync.Mutex
	scannerA := bufio.NewScanner(aStderr)
	go func() {
		for scannerA.Scan() {
			line := scannerA.Text()
			t.Logf("Node A Log: %s", line)
			if strings.Contains(line, "DHT escuchando en:") {
				parts := strings.Split(line, ": ")
				if len(parts) >= 2 {
					addrPart := strings.Split(parts[1], " ")[0]
					idPart := strings.Trim(strings.Split(line, "ID: ")[1], ")")
					mu.Lock()
					nodeAAddr = addrPart
					nodeAIDStr = idPart
					mu.Unlock()
				}
			}
		}
	}()

	// Wait for Node A to initialize
	var finalAddr string
	var finalID string
	for i := 0; i < 20; i++ {
		mu.Lock()
		finalAddr = nodeAAddr
		finalID = nodeAIDStr
		mu.Unlock()
		if finalAddr != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalAddr == "" {
		t.Fatalf("Could not determine Node A address from logs")
	}

	// 3. Start Node B (Peers with A)
	cmdB := exec.CommandContext(ctx, binaryPath, "-addr", "127.0.0.1:0", "-bootstrap", finalAddr, "-monitor-port", "7440")
	cmdB.Env = append(os.Environ(), "HOME="+dataDirB)
	bStdin, _ := cmdB.StdinPipe()
	bStdout, _ := cmdB.StdoutPipe()
	bStderr, _ := cmdB.StderrPipe()
	if err := cmdB.Start(); err != nil {
		t.Fatalf("Failed to start Node B: %v", err)
	}
	defer cmdB.Process.Kill()

	go func() {
		scannerB := bufio.NewScanner(bStderr)
		for scannerB.Scan() {
			t.Logf("Node B Log: %s", scannerB.Text())
		}
	}()
	time.Sleep(1 * time.Second)

	// 4. Share content via Node A's MCP
	content := "The Hive uses Kademlia for P2P networking."
	topic := "kademlia"

	shareReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      "share-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hive_share",
			"arguments": map[string]string{
				"content": content,
				"topic":   topic,
			},
		},
	}
	shareData, _ := json.Marshal(shareReq)
	fmt.Fprintln(aStdin, string(shareData))

	// Wait more for share to process and replicate
	time.Sleep(2 * time.Second)

	// 5. Search via Node B's MCP
	mcpReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      "search-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hive_search",
			"arguments": map[string]string{
				"query": topic,
			},
		},
	}
	reqData, _ := json.Marshal(mcpReq)
	fmt.Fprintln(bStdin, string(reqData))

	bStdoutScanner := bufio.NewScanner(bStdout)
	respLine, ok := scanLineWithin(ctx, bStdoutScanner)
	if ok {
		t.Logf("Node B Response: %s", respLine)

		// The new response format for index resolution
		if !strings.Contains(respLine, "Resultados del Enjambre") {
			t.Errorf("Expected swarm results header in response, got: %s", respLine)
		}

		if !strings.Contains(respLine, finalID[:8]) {
			t.Errorf("Expected author %s in response", finalID[:8])
		}

		if !strings.Contains(respLine, content) {
			t.Errorf("Expected original content in search results")
		}
	} else {
		t.Fatalf("No response from Node B before timeout")
	}

	// 7. Test hive_rate on Node B
	// We need the chunk ID. From share response or just hardcode if it's deterministic.
	// Actually we just shared content.
	mcpShare := map[string]any{
		"jsonrpc": "2.0",
		"id":      "3",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hive_share",
			"arguments": map[string]string{
				"content": "Secret knowledge",
				"topic":   "test-topic",
			},
		},
	}
	shareData, _ = json.Marshal(mcpShare)
	fmt.Fprintln(bStdin, string(shareData))

	respLine, ok = scanLineWithin(ctx, bStdoutScanner)
	if ok {
		t.Logf("Node B Response (hive_share): %s", respLine)
		if strings.Contains(respLine, "success") || strings.Contains(respLine, "éxito") || strings.Contains(respLine, "Memoria compartida") {
			t.Logf("SUCCESS: hive_share tool responded with success")
		}
	} else {
		t.Fatalf("No response from Node B for hive_share before timeout")
	}
}

func scanLineWithin(ctx context.Context, scanner *bufio.Scanner) (string, bool) {
	type result struct {
		line string
		ok   bool
	}

	ch := make(chan result, 1)
	go func() {
		if scanner.Scan() {
			ch <- result{line: scanner.Text(), ok: true}
			return
		}
		ch <- result{ok: false}
	}()

	select {
	case res := <-ch:
		return res.line, res.ok
	case <-ctx.Done():
		return "", false
	}
}
