package logger

import (
	"fmt"
	"io"
	"os"
)

// Logger is a simple logger that writes to a specific io.Writer.
type Logger struct {
	writer io.Writer
}

// Default is a logger that writes exclusively to os.Stderr.
// This is critical for The Hive to avoid breaking the MCP protocol on os.Stdout.
var Default = &Logger{writer: os.Stderr}

// Printf logs a formatted string to the designated output (os.Stderr).
func (l *Logger) Printf(format string, v ...any) {
	fmt.Fprintf(l.writer, format+"\n", v...)
}

// Println logs a line to the designated output (os.Stderr).
func (l *Logger) Println(v ...any) {
	fmt.Fprintln(l.writer, v...)
}

// Info logs an info message to the default logger.
func Info(format string, v ...any) {
	Default.Printf("[INFO] "+format, v...)
}

// Warn logs a warning message to the default logger.
func Warn(format string, v ...any) {
	Default.Printf("[WARN] "+format, v...)
}

// Error logs an error message to the default logger.
func Error(format string, v ...any) {
	Default.Printf("[ERROR] "+format, v...)
}

// Debug logs a debug message to the default logger.
func Debug(format string, v ...any) {
	Default.Printf("[DEBUG] "+format, v...)
}
