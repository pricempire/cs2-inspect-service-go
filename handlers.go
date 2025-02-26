package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// handleInspect handles the inspect request
func handleInspect(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	// Handle OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Only allow GET requests
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the inspect link from the query string
	inspectLink := r.URL.Query().Get("link")
	if inspectLink == "" {
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   "Missing inspect link",
		})
		return
	}

	log.Printf("Received inspect request for link: %s", inspectLink)

	// Parse the inspect link
	paramA, paramD, owner, err := parseInspectLink(inspectLink)
	if err != nil {
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid inspect link: %v", err),
		})
		return
	}

	log.Printf("Parsed inspect link: A:%d D:%d Owner:%d", paramA, paramD, owner)

	// Get an available bot
	bot := getAvailableBot()
	if bot == nil {
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   "No bots available",
		})
		return
	}
	defer releaseBot(bot)

	log.Printf("Using bot: %s", bot.Account.Username)

	// Check if the bot is ready
	if !bot.CS2Handler.IsReady() {
		log.Printf("Bot %s is not ready, sending hello message", bot.Account.Username)
		
		// Send a hello message as a last resort
		bot.CS2Handler.SendHello()
		
		// Wait for the bot to become ready with a short timeout
		readyTimeout := time.After(5 * time.Second)
		readyCheckTicker := time.NewTicker(100 * time.Millisecond)
		defer readyCheckTicker.Stop()
		
		ready := false
		for !ready {
			select {
			case <-readyTimeout:
				sendJSONResponse(w, InspectResponse{
					Success: false,
					Error:   "Bot failed to connect to Game Coordinator",
				})
				return
			case <-readyCheckTicker.C:
				if bot.CS2Handler.IsReady() {
					ready = true
				}
			}
		}
	}

	// Request item info
	bot.CS2Handler.RequestItemInfo(paramA, paramD, owner)

	// Wait for response with timeout
	timeoutDuration := ClientRequestTimeout
	log.Printf("Waiting for response with timeout of %v", timeoutDuration)
	
	// Create a timeout channel
	timeoutChan := time.After(timeoutDuration)
	
	// Wait for either a response or a timeout
	select {
	case responseData := <-bot.CS2Handler.responseChannel:
		log.Printf("Received response with %d bytes", len(responseData))
		
		// Extract item info from the response
		itemInfo, err := ExtractItemInfo(responseData)
		if err != nil {
			log.Printf("Error extracting item info: %v", err)
			// Still return the raw data even if we couldn't extract the item info
			sendJSONResponse(w, InspectResponse{
				Success: true,
				Data:    responseData,
				Error:   fmt.Sprintf("Received response but couldn't extract item info: %v", err),
			})
			return
		}
		
		// Return the successful response with item info
		sendJSONResponse(w, InspectResponse{
			Success:  true,
			ItemInfo: itemInfo,
		})
		
	case <-timeoutChan:
		log.Printf("Request timed out after %v", timeoutDuration)
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   fmt.Sprintf("Request timed out after %v", timeoutDuration),
		})
	}
}

// handleHealth handles the health check request
func handleHealth(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	// Handle OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Only allow GET requests
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the status of all bots
	botMutex.Lock()
	botStatuses := make([]BotStatus, 0, len(bots))
	
	readyCount := 0
	for _, bot := range bots {
		bot.Mutex.Lock()
		status := BotStatus{
			Username:  bot.Account.Username,
			Connected: bot.Connected,
			LoggedOn:  bot.LoggedOn,
			Ready:     bot.CS2Handler.IsReady(),
			Busy:      bot.Busy,
		}
		bot.Mutex.Unlock()
		
		botStatuses = append(botStatuses, status)
		
		if status.Ready {
			readyCount++
		}
	}
	botMutex.Unlock()
	
	// Determine overall status
	status := "healthy"
	if readyCount == 0 {
		status = "unhealthy"
	} else if readyCount < len(bots) {
		status = "degraded"
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status: status,
		Bots:   botStatuses,
	})
} 