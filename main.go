package main

import (
	"fmt"
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
	http.HandleFunc("/reconnect", handleReconnect)
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

// handleReconnect handles requests to manually reconnect bots
func handleReconnect(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	// Handle OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Only allow GET and POST requests
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the username from the query string (optional)
	username := r.URL.Query().Get("username")
	
	// Check for force parameter
	forceRestart := r.URL.Query().Get("force") == "true"
	
	// If botManager is nil, return an error
	if botManager == nil {
		sendJSONResponse(w, ReconnectResponse{
			Success: false,
			Message: "Bot manager not initialized",
		})
		return
	}
	
	reconnectedCount := 0
	
	// If a username is provided, only reconnect that bot
	if username != "" {
		// Check if the account is blacklisted
		if isBlacklisted(username) {
			LogWarning("Manual reconnect requested for blacklisted bot: %s", username)
			sendJSONResponse(w, ReconnectResponse{
				Success: false,
				Message: fmt.Sprintf("Bot %s is blacklisted", username),
			})
			return
		}
		
		LogInfo("Manual reconnect triggered for bot: %s (force: %v)", username, forceRestart)
		
		// Send reconnect command to bot manager thread
		resultChan := make(chan botResponse, 1)
		botManager.commandChan <- botCommand{
			action:      "reconnectBot",
			botUsername: username,
			resultChan:  resultChan,
		}
		
		// Wait for response with timeout
		select {
		case resp := <-resultChan:
			if resp.success {
				reconnectedCount++
				sendJSONResponse(w, ReconnectResponse{
					Success: true,
					Message: resp.message,
				})
			} else {
				sendJSONResponse(w, ReconnectResponse{
					Success: false,
					Message: resp.message,
				})
			}
		case <-time.After(5 * time.Second):
			// Timeout, but reconnect is likely still in progress
			sendJSONResponse(w, ReconnectResponse{
				Success: true,
				Message: fmt.Sprintf("Reconnect for bot %s is in progress", username),
			})
		}
		
		return
	} else {
		// For reconnecting all bots, we'll use a different approach
		// since we don't want to block the HTTP response
		
		// Get a list of all bot usernames
		var botUsernames []string
		botManager.mutex.RLock()
		for _, bot := range botManager.bots {
			bot.mutex.Lock()
			botUsernames = append(botUsernames, bot.account.Username)
			bot.mutex.Unlock()
		}
		botManager.mutex.RUnlock()
		
		// Start a goroutine to reconnect all bots
		go func(usernames []string) {
			for _, username := range usernames {
				// Skip blacklisted accounts
				if isBlacklisted(username) {
					LogWarning("Skipping reconnect for blacklisted bot: %s", username)
					continue
				}
				
				// Send reconnect command
				resultChan := make(chan botResponse, 1)
				botManager.commandChan <- botCommand{
					action:      "reconnectBot",
					botUsername: username,
					resultChan:  resultChan,
				}
				
				// Don't wait for response, just fire and forget
				// The bot manager thread will handle it
				
				// Add a small delay between reconnects to avoid overwhelming the system
				time.Sleep(500 * time.Millisecond)
			}
		}(botUsernames)
		
		// Return success immediately
		sendJSONResponse(w, ReconnectResponse{
			Success: true,
			Message: fmt.Sprintf("Reconnect initiated for all bots (%d total)", len(botUsernames)),
		})
		return
	}
} 