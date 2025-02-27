package main

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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

	// Check if we should refresh the data from the Game Coordinator
	refresh := r.URL.Query().Get("refresh") != ""
	
	LogInfo("Received inspect request for link: %s (refresh: %v)", inspectLink, refresh)

	// Parse the inspect link
	paramA, paramD, paramS, paramM, err := parseInspectLink(inspectLink)
	if err != nil {
		LogError("Invalid inspect link: %v", err)
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid inspect link: %v", err),
		})
		return
	}

	LogInfo("Parsed inspect link: A:%d D:%d S:%d M:%d", paramA, paramD, paramS, paramM)
	
	// Check if we have a database connection and should use cached data
	if db != nil && !refresh {
		// Try to find the item in the database by asset parameters
		assetID := int64(paramA)
		dValue := strconv.FormatUint(paramD, 10)
		
		// Use either paramS or paramM as the owner value
		var msValue int64
		if paramS > 0 {
			msValue = int64(paramS)
		} else {
			msValue = int64(paramM)
		}
		
		LogInfo("Checking database for asset: AssetID=%d, D=%s, MS=%d", assetID, dValue, msValue)
		
		asset, err := FindAssetByParams(assetID, dValue, msValue)
		if err != nil {
			LogError("Error querying database: %v", err)
		} else if asset != nil {
			LogInfo("Found asset in database: %s", asset.UniqueID)
			
			// Create item info from the asset
			itemInfo := &ItemInfo{
				DefIndex:          uint32(asset.DefIndex),
				PaintIndex:        uint32(asset.PaintIndex),
				Rarity:            uint32(asset.Rarity),
				Quality:           uint32(asset.Quality),
				PaintWear:         asset.PaintWear,
				PaintSeed:         uint32(asset.PaintSeed),
				CustomName:        asset.CustomName.String,
				KilleaterScoreType: uint32(asset.KilleaterScoreType),
				KilleaterValue:    getKilleaterValue(asset.KilleaterValue),
				Origin:            uint32(asset.Origin),
				QuestId:           uint32(asset.QuestID),
				DropReason:        uint32(asset.DropReason),
				MusicIndex:        uint32(asset.MusicIndex),
				EntIndex:          int32(asset.EntIndex),
				PetIndex:          uint32(asset.PetIndex),
				Inventory:         uint32(asset.Inventory),
				IsStatTrak:        asset.IsStattrak,
				IsSouvenir:        asset.IsSouvenir,
			}
			
			// If we have stickers, unmarshal them
			if asset.Stickers != nil && len(asset.Stickers) > 0 {
				var stickers []StickerInfo
				if err := json.Unmarshal(asset.Stickers, &stickers); err == nil {
					itemInfo.Stickers = stickers
				}
			}
			
			// If we have keychains, unmarshal them
			if asset.Keychains != nil && len(asset.Keychains) > 0 {
				var keychains []StickerInfo
				if err := json.Unmarshal(asset.Keychains, &keychains); err == nil {
					itemInfo.Keychains = keychains
				}
			}
			
			// Apply schema information to cached items
			applySchemaToItemInfo(itemInfo)
			
			// Return the cached response
			sendJSONResponse(w, InspectResponse{
				Success:  true,
				ItemInfo: itemInfo,
				Cached:   true,
			})
			return
		}
	}

	// Get an available bot
	bot := GetAvailableBot()
	if bot == nil {
		LogError("No bots available")
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   "No bots available",
		})
		return
	}
	defer ReleaseBot(bot)

	LogInfo("Using bot: %s", bot.account.Username)

	// Check if the bot is ready
	if !bot.isGCReady() {
		LogInfo("Bot %s is not ready, sending hello message", bot.account.Username)
		
		// Send a hello message as a last resort
		bot.SendHello()
		
		// Wait for the bot to become ready with a short timeout
		readyTimeout := time.After(5 * time.Second)
		readyCheckTicker := time.NewTicker(100 * time.Millisecond)
		defer readyCheckTicker.Stop()
		
		ready := false
		for !ready {
			select {
			case <-readyTimeout:
				LogError("Bot failed to connect to Game Coordinator")
				sendJSONResponse(w, InspectResponse{
					Success: false,
					Error:   "Bot failed to connect to Game Coordinator",
				})
				return
			case <-readyCheckTicker.C:
				if bot.isGCReady() {
					ready = true
				}
			}
		}
	}

	// Request item info
	bot.RequestItemInfo(paramA, paramD, paramS, paramM)

	// Wait for response with timeout
	timeoutDuration := ClientRequestTimeout
	LogInfo("Waiting for response with timeout of %v", timeoutDuration)
	
	// Create a timeout channel
	timeoutChan := time.After(timeoutDuration)
	
	// Wait for either a response or a timeout
	select {
	case responseData := <-bot.cs2Handler.responseChannel:
		LogInfo("Received response with %d bytes", len(responseData))
		
		// Extract item info from the response
		itemInfo, err := ExtractItemInfo(responseData)
		if err != nil {
			LogError("Error extracting item info: %v", err)
			// Still return the raw data even if we couldn't extract the item info
			sendJSONResponse(w, InspectResponse{
				Success: true,
				Data:    responseData,
				Error:   fmt.Sprintf("Received response but couldn't extract item info: %v", err),
			})
			return
		}
		
		// Save to database if we have a connection
		if db != nil && itemInfo != nil {
			// Generate a unique ID for the asset using item properties
			uniqueID := generateUniqueID(itemInfo)
			
			// Convert stickers to JSON
			var stickersJSON []byte
			if len(itemInfo.Stickers) > 0 {
				stickersJSON, _ = json.Marshal(itemInfo.Stickers)
			}
			
			// Convert keychains to JSON
			var keychainsJSON []byte
			if len(itemInfo.Keychains) > 0 {
				keychainsJSON, _ = json.Marshal(itemInfo.Keychains)
			}
			
			// Check if we already have this asset with a different owner or stickers/keychains
			existingAsset, err := FindAssetByUniqueID(uniqueID)
			if err != nil {
				LogError("Error checking for existing asset: %v", err)
			}
			
			// Create asset record
			// Use either paramS or paramM as the owner value
			var msValue int64
			if paramS > 0 {
				msValue = int64(paramS)
			} else {
				msValue = int64(paramM)
			}
			
			asset := &Asset{
				UniqueID:           uniqueID,
				AssetID:            int64(paramA),
				Ms:                 msValue,
				D:                  strconv.FormatUint(paramD, 10),
				PaintSeed:          int16(itemInfo.PaintSeed),
				PaintIndex:         int16(itemInfo.PaintIndex),
				PaintWear:          float64(itemInfo.PaintWear),
				Quality:            int16(itemInfo.Quality),
				CustomName:         sql.NullString{String: itemInfo.CustomName, Valid: itemInfo.CustomName != ""},
				DefIndex:           int16(itemInfo.DefIndex),
				Rarity:             int16(itemInfo.Rarity),
				Origin:             int16(itemInfo.Origin),
				QuestID:            int16(itemInfo.QuestId),
				Reason:             int16(itemInfo.DropReason),
				MusicIndex:         int16(itemInfo.MusicIndex),
				EntIndex:           int16(itemInfo.EntIndex),
				PetIndex:           int16(itemInfo.PetIndex),
				IsStattrak:         itemInfo.IsStatTrak,
				IsSouvenir:         itemInfo.IsSouvenir,
				Stickers:           stickersJSON,
				Keychains:          keychainsJSON,
				KilleaterScoreType: int16(itemInfo.KilleaterScoreType),
				KilleaterValue:     int32(itemInfo.KilleaterValue),
				Inventory:          int64(itemInfo.Inventory),
				DropReason:         int16(itemInfo.DropReason),
			}

			// Create a history record
			var historyType HistoryType
			var prevAssetID int64
			var prevOwner string
			var prevStickers []byte
			var prevKeychains []byte

			if existingAsset != nil {
				ownerChanged := existingAsset.Ms != asset.Ms
				stickersChanged := !bytes.Equal(existingAsset.Stickers, asset.Stickers)
				keychainsChanged := !bytes.Equal(existingAsset.Keychains, asset.Keychains)
				nametagChanged := existingAsset.CustomName.String != asset.CustomName.String
				
				if ownerChanged || stickersChanged || keychainsChanged || nametagChanged {
					// Determine history type
					historyType = HistoryTypeUnknown
					
					if ownerChanged {
						historyType = HistoryTypeTrade
					} else if stickersChanged {
						// Determine if stickers were added, removed, or changed
						if len(existingAsset.Stickers) == 0 && len(asset.Stickers) > 0 {
							historyType = HistoryTypeStickerApply
						} else if len(existingAsset.Stickers) > 0 && len(asset.Stickers) == 0 {
							historyType = HistoryTypeStickerRemove
						} else {
							historyType = HistoryTypeStickerChange
						}
					} else if keychainsChanged {
						// Determine if keychains were added, removed, or changed
						if len(existingAsset.Keychains) == 0 && len(asset.Keychains) > 0 {
							historyType = HistoryTypeKeychainAdded
						} else if len(existingAsset.Keychains) > 0 && len(asset.Keychains) == 0 {
							historyType = HistoryTypeKeychainRemoved
						} else {
							historyType = HistoryTypeKeychainChanged
						}
					} else if nametagChanged {
						if !asset.CustomName.Valid || asset.CustomName.String == "" {
							historyType = HistoryTypeNametagRemoved
						} else {
							historyType = HistoryTypeNametagAdded
						}
					}

					prevAssetID = existingAsset.AssetID
					prevOwner = strconv.FormatInt(existingAsset.Ms, 10)
					prevStickers = existingAsset.Stickers
					prevKeychains = existingAsset.Keychains
				} else {
					// No changes detected, don't create a history record
					historyType = 0
				}
			} else {
				// This is a new item, determine history type based on origin
				switch asset.Origin {
				case 8:
					historyType = HistoryTypeTradedUp
				case 4:
					historyType = HistoryTypeDropped
				case 1:
					historyType = HistoryTypePurchasedIngame
				case 2:
					historyType = HistoryTypeUnboxed
				case 3:
					historyType = HistoryTypeCrafted
				case 12:
					historyType = HistoryTypeDropped // Tournament drops
				default:
					historyType = HistoryTypeUnknown
				}
			}
			
			// Create and save history record if we have a history type
			if historyType != 0 {
				// Ensure PrevAssetID is 0 if it's not set
				if prevAssetID == 0 {
					prevAssetID = 0
				}
				
				// Ensure prevOwner is not empty
				if prevOwner == "" {
					prevOwner = "0"
				}

				history := &History{
					UniqueID:      uniqueID,
					AssetID:       asset.AssetID,
					PrevAssetID:   prevAssetID,
					Owner:         strconv.FormatInt(asset.Ms, 10),
					PrevOwner:     prevOwner,
					D:             asset.D,
					Stickers:      asset.Stickers,
					Keychains:     asset.Keychains,
					PrevStickers:  prevStickers,
					PrevKeychains: prevKeychains,
					Type:          historyType,
				}
				
				// Save history record
				if err := SaveHistory(history); err != nil {
					LogError("Error saving history record: %v", err)
				} else {
					LogInfo("Saved history record: %s (Type: %d)", uniqueID, historyType)
				}
			}
			
			// Save to database
			if err := SaveAsset(asset); err != nil {
				LogError("Error saving asset to database: %v", err)
			} else {
				LogInfo("Saved asset to database: %s", uniqueID)
			}
		}
		
		// Return the successful response with item info
		sendJSONResponse(w, InspectResponse{
			Success:  true,
			ItemInfo: itemInfo,
			Cached:   false,
		})
		
	case <-timeoutChan:
		LogError("Request timed out after %v", timeoutDuration)
		
		// Send goodbye message to GC and reinitialize the bot
		LogInfo("Sending goodbye message and reinitializing bot due to timeout")
		
		// Force a reconnect of the bot
		bot.Reconnect()
		
		sendJSONResponse(w, InspectResponse{
			Success: false,
			Error:   fmt.Sprintf("Request timed out after %v", timeoutDuration),
		})
	}
}

// generateUniqueID creates a unique identifier for an item based on its properties
func generateUniqueID(item *ItemInfo) string {
	// Combine the relevant item properties
	values := []interface{}{
		item.PaintSeed,
		item.PaintIndex,
		item.PaintWear,
		item.DefIndex,
		item.Origin,
		item.Rarity,
		item.QuestId,
		item.Quality,
		item.DropReason,
	}
	
	// Convert values to strings and join with hyphens
	var stringValues []string
	for _, v := range values {
		stringValues = append(stringValues, fmt.Sprintf("%v", v))
	}
	stringToHash := strings.Join(stringValues, "-")
	
	// Create SHA1 hash and take first 8 characters
	h := sha1.New()
	h.Write([]byte(stringToHash))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// applySchemaToItemInfo applies schema information to an ItemInfo object
func applySchemaToItemInfo(itemInfo *ItemInfo) {
	// Add additional fields from schema
	s := GetSchema()
	if s != nil {
		// Set wear name
		itemInfo.WearName = GetWearName(itemInfo.PaintWear)
		
		// Set phase name for Doppler knives
		itemInfo.Phase = GetPhaseName(int16(itemInfo.PaintIndex))
		
		// Build market hash name
		itemInfo.MarketHashName = BuildMarketHashName(
			int16(itemInfo.DefIndex),
			int16(itemInfo.PaintIndex),
			int16(itemInfo.Quality),
			itemInfo.IsStatTrak,
			itemInfo.IsSouvenir,
			itemInfo.PaintWear,
		)
		
		// Set pattern name if available
		itemInfo.Pattern = GetPatternName(itemInfo.MarketHashName, int16(itemInfo.PaintSeed))
		
		// Set item type
		defIndexStr := fmt.Sprintf("%d", itemInfo.DefIndex)
		if _, ok := s.Weapons[defIndexStr]; ok {
			itemInfo.Type = "Weapon"
			
			// Set min/max wear values if available
			if itemInfo.PaintIndex > 0 {
				paintIndexStr := fmt.Sprintf("%d", itemInfo.PaintIndex)
				if weapon, ok := s.Weapons[defIndexStr]; ok {
					if paint, ok := weapon.Paints[paintIndexStr]; ok {
						itemInfo.Min = paint.Min
						itemInfo.Max = paint.Max
						itemInfo.Image = paint.Image
					}
				}
			}
		}
		
		// Set additional fields for the response
		itemInfo.Rank = 0 // This would need to be calculated or retrieved from somewhere
		itemInfo.TotalCount = 0 // This would need to be calculated or retrieved from somewhere
		
		// Ensure these boolean fields are set
		// (they should already be set from the item data, but just to be sure)
		if itemInfo.IsStatTrak {
			itemInfo.IsStatTrak = true
		}
		
		if itemInfo.IsSouvenir {
			itemInfo.IsSouvenir = true
		}
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
	if botManager == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{
			Status: "unhealthy",
			Bots:   []BotStatus{},
		})
		return
	}

	botManager.mutex.RLock()
	botStatuses := make([]BotStatus, 0, len(botManager.bots))
	
	readyCount := 0
	for _, bot := range botManager.bots {
		bot.mutex.Lock()
		status := BotStatus{
			Username:  bot.account.Username,
			Connected: bot.state == BotStateConnected || bot.state == BotStateLoggedIn || bot.state == BotStateReady || bot.state == BotStateBusy,
			LoggedOn:  bot.state == BotStateLoggedIn || bot.state == BotStateReady || bot.state == BotStateBusy,
			Ready:     bot.state == BotStateReady || bot.state == BotStateBusy,
			Busy:      bot.state == BotStateBusy,
		}
		bot.mutex.Unlock()
		
		botStatuses = append(botStatuses, status)
		
		if status.Ready {
			readyCount++
		}
	}
	botManager.mutex.RUnlock()
	
	// Determine overall status
	status := "healthy"
	if readyCount == 0 {
		status = "unhealthy"
	} else if readyCount < len(botManager.bots) {
		status = "degraded"
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status: status,
		Bots:   botStatuses,
	})
}

// HistoryResponse represents the response for a history request
type HistoryResponse struct {
	Success  bool       `json:"success"`
	History  []*History `json:"history,omitempty"`
	Error    string     `json:"error,omitempty"`
}

// handleHistory handles the history request
func handleHistory(w http.ResponseWriter, r *http.Request) {
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

	// Get the unique ID from the query string
	uniqueID := r.URL.Query().Get("uniqueId")
	if uniqueID == "" {
		sendJSONResponse(w, HistoryResponse{
			Success: false,
			Error:   "Missing uniqueId parameter",
		})
		return
	}

	log.Printf("Received history request for uniqueId: %s", uniqueID)

	// Check if we have a database connection
	if db == nil {
		sendJSONResponse(w, HistoryResponse{
			Success: false,
			Error:   "Database connection not available",
		})
		return
	}

	// Find history records for the item
	history, err := FindHistoryByUniqueID(uniqueID)
	if err != nil {
		log.Printf("Error querying history: %v", err)
		sendJSONResponse(w, HistoryResponse{
			Success: false,
			Error:   fmt.Sprintf("Error querying history: %v", err),
		})
		return
	}

	// Return the history records
	sendJSONResponse(w, HistoryResponse{
		Success: true,
		History: history,
	})
}

// getKilleaterValue returns the KilleaterValue if it's >= 0, otherwise returns -1
func getKilleaterValue(value int32) int32 {
	if value >= 0 {
		return value
	}
	return -1
}
