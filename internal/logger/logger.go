// Package logger provides structured logging for TapeDeck using Go's log/slog.
package logger

import (
	"io"
	"log/slog"
	"os"
)

var logger *slog.Logger

// Init initializes the global logger with the specified level and writers
func Init(level string, writers ...io.Writer) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// If no writers provided, use stdout
	if len(writers) == 0 {
		writers = []io.Writer{os.Stdout}
	}

	// If multiple writers, use MultiWriter
	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	handler := slog.NewTextHandler(writer, opts)
	logger = slog.New(handler)
}

// Get returns the global logger instance
func Get() *slog.Logger {
	if logger == nil {
		// Default to info level if not initialized
		Init("info")
	}
	return logger
}

// Debug logs a debug message with optional key-value pairs
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}
