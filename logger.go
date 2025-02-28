package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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
	// Environment variable for log directory
	LogDirEnvVar = "LOG_DIR"
	// Default log directory
	DefaultLogDir = "logs"
)

var (
	// Whether detailed logging is enabled
	detailedLoggingEnabled bool
	// Whether colored logging is enabled
	coloredLoggingEnabled bool
	// Log file
	logFile *os.File
	// Logger instance
	logger *log.Logger
)

// InitLogger initializes the logger with the specified configuration
func InitLogger() {
	// Check if detailed logging is enabled via environment variable
	detailedLoggingEnabled = strings.ToLower(os.Getenv(LogDetailEnvVar)) == "true"
	
	// Check if colored logging is enabled via environment variable (default to true)
	coloredLoggingEnabled = os.Getenv(LogColorEnvVar) != "false"
	
	// Get log directory from environment variable or use default
	logDir := os.Getenv(LogDirEnvVar)
	if logDir == "" {
		logDir = DefaultLogDir
	}
	
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Failed to create log directory: %v", err)
	}
	
	// Create log file with current date in the filename
	currentTime := time.Now().Format("2006-01-02")
	logFilePath := filepath.Join(logDir, fmt.Sprintf("cs2-inspect-%s.log", currentTime))
	
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
	} else {
		// Create a multi-writer to write to both console and file
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		logger = log.New(multiWriter, "", log.LstdFlags)
		
		// Replace the standard logger
		log.SetOutput(multiWriter)
		log.SetFlags(log.LstdFlags)
	}
	
	LogInfo("Logging initialized. Logs will be saved to: %s", logFilePath)
	
	if detailedLoggingEnabled {
		LogInfo("Detailed logging enabled - logs will include file and function information")
	}
	
	if coloredLoggingEnabled {
		LogInfo("Colored logging enabled (console only)")
	}
}

// CloseLogger closes the log file
func CloseLogger() {
	if logFile != nil {
		logFile.Close()
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
	coloredLevelStr := level
	if coloredLoggingEnabled {
		coloredLevelStr = color + level + ColorReset
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
		fileInfo := fmt.Sprintf("%s:%s:%d", filename, funcName, line)
		
		// For file logging (without colors)
		if logger != nil {
			logger.Printf("[%s] %s - %s", levelStr, fileInfo, message)
		} else {
			// Fallback to standard log if logger is not initialized
			// For console (with colors if enabled)
			log.Printf("[%s] %s - %s", coloredLevelStr, fileInfo, message)
		}
	} else {
		// Log without file and function information
		if logger != nil {
			logger.Printf("[%s] %s", levelStr, message)
		} else {
			// Fallback to standard log if logger is not initialized
			log.Printf("[%s] %s", coloredLevelStr, message)
		}
	}
} 