package main

import (
	"io"
	"os"
	"testing"

	"the-hive/internal/logger"
)

func TestMain(m *testing.M) {
	prev := logger.SetOutput(os.Stderr)
	if os.Getenv("HIVE_TEST_LOG") != "verbose" {
		logger.SetOutput(io.Discard)
	}

	code := m.Run()
	logger.SetOutput(prev)
	os.Exit(code)
}
