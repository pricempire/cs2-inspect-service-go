package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[37m"
)

// Logger constants
const (
	// Environment variable name to enable/disable detailed logging
	LogDetailEnvVar = "LOG_DETAIL"
	// Environment variable to enable/disable colored output
	LogColorEnvVar = "LOG_COLOR"
)

var (
	// Whether detailed logging is enabled
	detailedLoggingEnabled bool
	// Whether colored logging is enabled
	coloredLoggingEnabled bool
)

// InitLogger initializes the logger with the specified configuration
func InitLogger() {
	// Check if detailed logging is enabled via environment variable
	detailedLoggingEnabled = strings.ToLower(os.Getenv(LogDetailEnvVar)) == "true"
	
	// Check if colored logging is enabled via environment variable (default to true)
	coloredLoggingEnabled = os.Getenv(LogColorEnvVar) != "false"
	
	// Configure the standard logger to include date and time
	log.SetFlags(log.LstdFlags)
	
	if detailedLoggingEnabled {
		LogInfo("Detailed logging enabled - logs will include file and function information")
	}
	
	if coloredLoggingEnabled {
		LogInfo("Colored logging enabled")
	}
}

// LogDebug logs a debug message with file and function information if detailed logging is enabled
func LogDebug(format string, args ...interface{}) {
	logWithLevel("DEBUG", ColorCyan, format, args...)
}

// LogInfo logs an info message with file and function information if detailed logging is enabled
func LogInfo(format string, args ...interface{}) {
	logWithLevel("INFO", ColorGreen, format, args...)
}

// LogWarning logs a warning message with file and function information if detailed logging is enabled
func LogWarning(format string, args ...interface{}) {
	logWithLevel("WARNING", ColorYellow, format, args...)
}

// LogError logs an error message with file and function information if detailed logging is enabled
func LogError(format string, args ...interface{}) {
	logWithLevel("ERROR", ColorRed, format, args...)
}

// logWithLevel logs a message with the specified level and file/function information if detailed logging is enabled
func logWithLevel(level string, color string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	
	// Prepare the level string with color if enabled
	levelStr := level
	if coloredLoggingEnabled {
		levelStr = color + level + ColorReset
	}
	
	if detailedLoggingEnabled {
		// Get caller information
		pc, file, line, ok := runtime.Caller(2)
		if !ok {
			file = "unknown"
			line = 0
		}
		
		// Get just the filename without the path
		filename := filepath.Base(file)
		
		// Get function name
		funcName := runtime.FuncForPC(pc).Name()
		if lastDot := strings.LastIndex(funcName, "."); lastDot >= 0 {
			funcName = funcName[lastDot+1:]
		}
		
		// Log with file and function information
		log.Printf("[%s] %s:%s:%d - %s", levelStr, filename, funcName, line, message)
	} else {
		// Log without file and function information
		log.Printf("[%s] %s", levelStr, message)
	}
} 