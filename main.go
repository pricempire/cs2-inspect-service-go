package main

import (
	"encoding/json"
	"fmt"
	"log"
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
	log.Println("Starting CS:GO Skin Inspect Service")
	
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}
	
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	// Initialize database connection
	log.Println("Initializing database connection...")
	if err := InitDB(); err != nil {
		log.Printf("Warning: Failed to initialize database: %v", err)
		log.Println("Continuing without database support - caching will be disabled")
	} else {
		log.Println("Database connection established successfully")
		defer CloseDB()
	}

	// Load accounts
	accounts, err := loadAccounts()
	if err != nil {
		log.Fatalf("Failed to load accounts: %v", err)
	}
	log.Printf("Loaded %d accounts", len(accounts))

	// Initialize bots
	initializeBots(accounts)
	log.Printf("Initialized %d bots", len(bots))

	// Set up a ticker to periodically check if bots are connected to GC
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				botMutex.Lock()
				readyCount := 0
				disconnectedCount := 0
				
				for _, bot := range bots {
					bot.Mutex.Lock()
					
					// Check if the bot is connected and logged on
					if bot.Connected && bot.LoggedOn {
						// Check GC connection status
						isReady := bot.CS2Handler.CheckGCConnection()
						if isReady {
							readyCount++
						}
					} else {
						disconnectedCount++
						log.Printf("Bot %s: Status - Connected: %v, LoggedOn: %v", 
							bot.Account.Username, bot.Connected, bot.LoggedOn)
					}
					
					bot.Mutex.Unlock()
				}
				
				log.Printf("Bot status check: %d/%d bots ready, %d disconnected", 
					readyCount, len(bots), disconnectedCount)
				
				botMutex.Unlock()
			}
		}
	}()

	// Set up HTTP server
	http.HandleFunc("/inspect", handleInspect)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/reconnect", handleReconnect)
	http.HandleFunc("/history", handleHistory)
	
	// Start HTTP server
	log.Printf("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
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
	
	botMutex.Lock()
	reconnectedCount := 0
	
	// If a username is provided, only reconnect that bot
	if username != "" {
		for _, bot := range bots {
			bot.Mutex.Lock()
			if bot.Account.Username == username {
				if !bot.Connected || !bot.LoggedOn {
					bot.Mutex.Unlock()
					go reconnectBot(bot)
					reconnectedCount++
					log.Printf("Manual reconnect triggered for bot: %s", username)
				} else {
					bot.Mutex.Unlock()
					log.Printf("Bot %s is already connected, no reconnect needed", username)
				}
				break
			} else {
				bot.Mutex.Unlock()
			}
		}
	} else {
		// Otherwise, reconnect all disconnected bots
		for _, bot := range bots {
			bot.Mutex.Lock()
			if !bot.Connected || !bot.LoggedOn {
				botUsername := bot.Account.Username
				bot.Mutex.Unlock()
				go reconnectBot(bot)
				reconnectedCount++
				log.Printf("Manual reconnect triggered for bot: %s", botUsername)
			} else {
				bot.Mutex.Unlock()
			}
		}
	}
	botMutex.Unlock()
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ReconnectResponse{
		Success: true,
		Message: fmt.Sprintf("Reconnect triggered for %d bots", reconnectedCount),
	})
} 