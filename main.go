package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
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
		port = "4000"
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

	// Initialize bots
	LogInfo("Initializing bot manager...")
	if err := InitializeBots(); err != nil {
		LogError("Failed to initialize bots: %v", err)
		os.Exit(1)
	}
	LogInfo("Bot manager initialized successfully")
	
	// Register shutdown handler
	defer ShutdownBots()

	// Serve static files from the html directory
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("html/static"))))
	
	// Set up HTTP server routes
	http.HandleFunc("/inspect", handleInspect)
	http.HandleFunc("/", handleInspect)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/reconnect", handleReconnect)
	http.HandleFunc("/history", handleHistory)
	
	// Start HTTP server
	LogInfo("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		LogError("Failed to start HTTP server: %v", err)
		os.Exit(1)
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
	failedCount := 0
	blacklistedCount := 0
	
	botManager.mutex.RLock()
	defer botManager.mutex.RUnlock()
	
	// If a username is provided, only reconnect that bot
	if username != "" {
		for _, bot := range botManager.bots {
			bot.mutex.Lock()
			if bot.account.Username == username {
				botUsername := bot.account.Username
				botState := bot.state
				bot.mutex.Unlock()
				
				// Check if the account is blacklisted
				if isBlacklisted(botUsername) {
					LogWarning("Manual reconnect requested for blacklisted bot: %s", botUsername)
					blacklistedCount++
					continue
				}
				
				LogInfo("Manual reconnect triggered for bot: %s (current state: %s, force: %v)", 
					botUsername, botState, forceRestart)
				
				// If force restart is requested, completely restart the bot
				if forceRestart {
					// Use a channel to get the result of the reconnect
					resultChan := make(chan bool, 1)
					
					// Start reconnect in a goroutine to avoid blocking the HTTP response
					go func(b *Bot, ch chan<- bool) {
						// First try to gracefully disconnect
						b.mutex.Lock()
						if b.cs2Handler != nil {
							b.cs2Handler.SendGoodbye()
						}
						b.mutex.Unlock()
						
						// Wait a moment for goodbye to be sent
						time.Sleep(1 * time.Second)
						
						// Then force a complete reconnect
						success := b.Reconnect()
						ch <- success
					}(bot, resultChan)
					
					// Wait for the result with a timeout
					select {
					case success := <-resultChan:
						if success {
							reconnectedCount++
						} else {
							failedCount++
						}
					case <-time.After(2 * time.Second):
						// Don't wait for the result, just assume it's in progress
						LogInfo("Reconnect for bot %s is in progress", botUsername)
						reconnectedCount++
					}
				} else {
					// Standard reconnect
					go func(b *Bot) {
						if b.Reconnect() {
							LogInfo("Bot %s reconnected successfully", b.account.Username)
						} else {
							LogWarning("Bot %s failed to reconnect", b.account.Username)
						}
					}(bot)
					reconnectedCount++
				}
				break
			} else {
				bot.mutex.Unlock()
			}
		}
	} else {
		// Otherwise, reconnect all bots
		for _, bot := range botManager.bots {
			bot.mutex.Lock()
			botUsername := bot.account.Username
			botState := bot.state
			bot.mutex.Unlock()
			
			// Check if the account is blacklisted
			if isBlacklisted(botUsername) {
				LogWarning("Skipping reconnect for blacklisted bot: %s", botUsername)
				blacklistedCount++
				continue
			}
			
			LogInfo("Manual reconnect triggered for bot: %s (current state: %s, force: %v)", 
				botUsername, botState, forceRestart)
			
			// Use a goroutine to avoid blocking the HTTP response
			go func(b *Bot) {
				// Add a small delay to avoid all bots reconnecting simultaneously
				time.Sleep(time.Duration(rand.Intn(3)) * time.Second)
				
				if forceRestart {
					// First try to gracefully disconnect
					b.mutex.Lock()
					if b.cs2Handler != nil {
						b.cs2Handler.SendGoodbye()
					}
					b.mutex.Unlock()
					
					// Wait a moment for goodbye to be sent
					time.Sleep(1 * time.Second)
				}
				
				// Then force a complete reconnect
				if b.Reconnect() {
					LogInfo("Bot %s reconnected successfully", b.account.Username)
				} else {
					LogWarning("Bot %s failed to reconnect", b.account.Username)
				}
			}(bot)
			
			reconnectedCount++
		}
	}
	
	// Prepare response message
	message := fmt.Sprintf("Reconnect triggered for %d bots", reconnectedCount)
	if failedCount > 0 {
		message += fmt.Sprintf(", %d failed", failedCount)
	}
	if blacklistedCount > 0 {
		message += fmt.Sprintf(", %d blacklisted", blacklistedCount)
	}
	message += fmt.Sprintf(" (force: %v)", forceRestart)
	
	// Send response
	sendJSONResponse(w, ReconnectResponse{
		Success: reconnectedCount > 0,
		Message: message,
	})
} 