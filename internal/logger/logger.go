package logger

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
)

var (
	/* Logger writing to stdout */
	stdout *log.Logger = nil

	/* Logger writing to stderr */
	stderr *log.Logger = nil

	/* Enable debug logs */
	debugEnabled bool = false
)

// Initialize the logger with the specified name.
// The name will be prefixed to the start of each logged message.
// If the logger is already initialize, future calls to Init() will
// return an error: "logger is already initialized"
func Init(name string) error {
	if stdout != nil || stderr != nil {
		return errors.New("logger is already initialized")
	}

	loggerPrefix := fmt.Sprintf("[%s] ", name)
	stdout = log.New(&bytes.Buffer{}, loggerPrefix, log.Ldate | log.Ltime)
	stderr = log.New(&bytes.Buffer{}, loggerPrefix, log.Ldate | log.Ltime)

	stdout.SetOutput(os.Stdout)
	stderr.SetOutput(os.Stderr)
	return nil
}

// Enable/disable debug logs
func SetDebug(state bool) {
	debugEnabled = state
}

// Print a new debug log to stdout.
// It will be visible only if debug is enabled
func Debug(text string) {
	if !debugEnabled {
		return
	}

	if stdout == nil {
		return
	}

	stdout.Printf("(debug) %s\n", text)
}

// Print a new debug log to stdout.
// It will be visible only if debug is enabled
func Debugf(format string, v...any) {
	if !debugEnabled {
		return
	}
	
	Debug(fmt.Sprintf(format, v...))
}

// Print a new info log to stdout.
func Info(text string) {
	if stdout == nil {
		return
	}

	stdout.Printf("(info) %s\n", text)
}

// Print a new info log to stdout.
func Infof(format string, v...any) {
	Info(fmt.Sprintf(format, v...))
}

// Print a new error log to stderr.
func Error(text string) {
	if stderr == nil {
		return
	}

	stderr.Printf("(error) %s\n", text)
}

// Print a new error log to stderr.
func Errorf(format string, v...any) {
	Error(fmt.Sprintf(format, v...))
}

// Print a new fatal log to stderr.
// The call is followed by os.Exit(1)
func Fatal(text string) {
	if stderr == nil {
		return
	}

	stderr.Fatalf("(fatal) %s\n", text)
}

// Print a new fatal log to stderr.
// The call is followed by os.Exit(1)
func Fatalf(format string, v...any) {
	Fatal(fmt.Sprintf(format, v...))
}
