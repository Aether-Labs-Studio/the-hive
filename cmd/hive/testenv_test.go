package main

import (
	"net"
	"os"
	"strings"
	"testing"
)

func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("HIVE_RUN_INTEGRATION") != "1" {
		t.Skip("integration test skipped; run with HIVE_RUN_INTEGRATION=1")
	}
}

func requireUDPBind(t *testing.T) {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err == nil {
		_ = conn.Close()
		return
	}

	if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "permission denied") {
		t.Skipf("network bind not permitted in this environment: %v", err)
	}

	t.Fatalf("failed to probe UDP bind capability: %v", err)
}
