package main

import (
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Global variables for tracking metrics
var (
	startTime      = time.Now()
	successCount   = 0
	cachedCount    = 0
	failedCount    = 0
	timeoutCount   = 0
	currentRequests = 0
	requestHistory = make([]int, 0, 60) // Store last 60 seconds of request counts
)

// ReconnectResponse represents the response from the reconnect request
type ReconnectResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		LogWarning("Error loading .env file: %v", err)
	}
	
	// Initialize the logger
	InitLogger()
	// Ensure logger is closed on exit
	defer CloseLogger()
	
	LogInfo("Starting CS:GO Skin Inspect Service")
	
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Initialize database connection
	LogInfo("Initializing database connection...")
	if err := InitDB(); err != nil {
		LogWarning("Failed to initialize database: %v", err)
		LogInfo("Continuing without database support - caching will be disabled")
	} else {
		LogInfo("Database connection established successfully")
		defer CloseDB()
	}
	
	// Initialize schema service
	LogInfo("Initializing schema service...")
	StartSchemaUpdater()
	LogInfo("Schema service initialized")

	// Initialize bots in background
	LogInfo("Starting bot manager initialization in background...")
	go func() {
		if err := InitializeBots(); err != nil {
			LogError("Failed to initialize bots: %v", err)
			// Don't exit the application, just log the error
		} else {
			LogInfo("Bot manager initialized successfully")
		}
	}()
	
	// Register shutdown handler for bots
	defer ShutdownBots()

	// Start metrics collection
	go collectMetrics()

	// Serve static files from the html directory
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("html/static"))))
	
	// Set up HTTP server routes
	http.HandleFunc("/inspect", handleInspect)
	http.HandleFunc("/float", handleInspect)
	http.HandleFunc("/", handleInspect)
	
	http.HandleFunc("/health", handleHealth) 
	http.HandleFunc("/history", handleHistory)
	
	// Start HTTP server
	// Listen on all interfaces (0.0.0.0)
	address := ":" + port
	LogInfo("Starting HTTP server on %s", address)
	LogInfo("To access the server, open http://localhost:%s in your browser", port)
	
	// Start the server with a simpler approach
	if err := http.ListenAndServe(address, nil); err != nil {
		LogError("Failed to start HTTP server: %v", err)
		
		// If that fails, try localhost only
		LogInfo("Trying to listen on localhost only...")
		localAddress := "127.0.0.1:" + port
		if err := http.ListenAndServe(localAddress, nil); err != nil {
			LogError("Failed to start HTTP server on localhost: %v", err)
			os.Exit(1)
		}
	}
}

// collectMetrics collects request metrics every second
func collectMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Add current request count to history
			requestHistory = append(requestHistory, currentRequests)
			currentRequests = 0
			
			// Keep only the last 60 seconds
			if len(requestHistory) > 60 {
				requestHistory = requestHistory[len(requestHistory)-60:]
			}
		}
	}
} 