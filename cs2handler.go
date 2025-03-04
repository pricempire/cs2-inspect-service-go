package main

import (
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	goSteam "github.com/Philipp15b/go-steam/v3"
	csgoProto "github.com/Philipp15b/go-steam/v3/csgo/protocol/protobuf"
	"github.com/Philipp15b/go-steam/v3/protocol/gamecoordinator"
	"google.golang.org/protobuf/proto"
)

// Constants for GC connection and special values
const (
	HelloInterval = 30 * time.Second
	MaxHelloAttempts = 5
	KilleaterNotSet = math.MaxUint32 // Special value to indicate Killeatervalue was nil
)

// ByteSlice is a wrapper for []byte that implements protocol.Serializer
type ByteSlice []byte

func (b ByteSlice) Serialize(w io.Writer) error {
	_, err := w.Write(b)
	return err
}

// CS2Handler handles all CS2-related functionality
type CS2Handler struct {
	client          *goSteam.Client
	ready           bool
	readyMutex      sync.RWMutex
	itemInfoRequest chan struct {
		paramA uint64
		paramD uint64
		paramS uint64
		paramM uint64
	}
	responseChannel chan []byte
	lastHelloTime   time.Time
	lastStatusCheck time.Time
	helloAttempts   int
	stopHelloTicker chan struct{}
}

// NewCS2Handler creates a new CS2Handler
func NewCS2Handler(client *goSteam.Client) *CS2Handler {
	handler := &CS2Handler{
		client: client,
		ready:  false,
		itemInfoRequest: make(chan struct {
			paramA uint64
			paramD uint64
			paramS uint64
			paramM uint64
		}, 10), // Buffer for up to 10 requests
		responseChannel: make(chan []byte, 10), // Increase buffer size to 10 to reduce chance of blocking
		lastHelloTime:   time.Time{},
		lastStatusCheck: time.Time{},
		helloAttempts:   0,
		stopHelloTicker: make(chan struct{}),
	}
	
	// Start the hello ticker to periodically check and reconnect to GC if needed
	go handler.startHelloTicker()
	
	return handler
}

// startHelloTicker starts a ticker that periodically sends hello messages to the GC if not connected
func (h *CS2Handler) startHelloTicker() {
	ticker := time.NewTicker(HelloInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			h.readyMutex.RLock()
			ready := h.ready
			h.readyMutex.RUnlock()
			
			// If not ready, check if we should send a hello message
			if !ready {
				// Check if we've exceeded the maximum number of hello attempts
				if h.helloAttempts >= MaxHelloAttempts {
					LogWarning("Max hello attempts reached (%d), waiting for bot reconnect", MaxHelloAttempts)
					h.helloAttempts = 0
					time.Sleep(2 * time.Minute) // Wait longer before trying again
				} else {
					h.helloAttempts++
					LogInfo("Not ready, sending hello (attempt %d/%d)", h.helloAttempts, MaxHelloAttempts)
					h.SendHello()
				}
			} else if ready {
				// Reset hello attempts if we're ready
				h.helloAttempts = 0
			}
			
		case <-h.stopHelloTicker:
			LogInfo("Hello ticker stopped")
			return
		}
	}
}

// IsReady returns whether the handler is ready to process requests
func (h *CS2Handler) IsReady() bool {
	h.readyMutex.RLock()
	defer h.readyMutex.RUnlock()
	
	// Log the current ready state for debugging
	if time.Since(h.lastStatusCheck) > 10*time.Second {
		if h.ready {
			LogDebug("CS2Handler is ready")
		} else {
			LogDebug("CS2Handler is not ready")
		}
		h.lastStatusCheck = time.Now()
	}
	
	return h.ready
}

// SetReady sets the ready state of the handler
func (h *CS2Handler) SetReady(ready bool) {
	h.readyMutex.Lock()
	defer h.readyMutex.Unlock()
	
	// If transitioning from not ready to ready, log it
	if !h.ready && ready {
		LogInfo("CS2Handler: Now ready to process requests")
		h.helloAttempts = 0 // Reset hello attempts
	} else if h.ready && !ready {
		LogInfo("CS2Handler: No longer ready to process requests")
	}
	
	h.ready = ready
}

// HandleGCPacket handles Game Coordinator packets
func (h *CS2Handler) HandleGCPacket(packet *gamecoordinator.GCPacket) {
	LogDebug("HandleGCPacket called with AppId: %d, MsgType: %d (0x%x)", packet.AppId, packet.MsgType, packet.MsgType)
	
	if packet.AppId != CS2AppID {
		LogDebug("Ignoring packet for AppId: %d", packet.AppId)
		return
	}  

	switch packet.MsgType {
	case uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientWelcome):
		LogInfo("Received welcome message from CS2 Game Coordinator!")
		h.SetReady(true)
		LogInfo("Connected to CS2 Game Coordinator!")
		
		// Process any pending item info requests
		select {
		case req := <-h.itemInfoRequest:
			LogInfo("Processing pending item info request")
			h.requestItemInfo(req.paramA, req.paramD, req.paramS, req.paramM)
		default:
			// No pending requests
		}
	
	case uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientConnectionStatus):
		LogInfo("Received connection status message from GC")
		
		// The connection status message indicates we're connected to the GC
		// Set ready to true if it's not already
		if !h.IsReady() {
			LogInfo("Setting ready state to true based on connection status message")
			h.SetReady(true)
		}
	
	case uint32(csgoProto.ECsgoGCMsg_k_EMsgGCCStrike15_v2_Client2GCEconPreviewDataBlockResponse):
		// Handle the item info response
		h.HandleItemInfoResponse(packet)
	default:
		LogDebug("Received unknown GC message type: %d (0x%x)", packet.MsgType, packet.MsgType)
	}
}

// HandleItemInfoResponse handles the response from the Game Coordinator for an item info request
func (h *CS2Handler) HandleItemInfoResponse(packet *gamecoordinator.GCPacket) {
	LogInfo("Received item info response with %d bytes of data", len(packet.Body))
	
	// Parse the protobuf message to validate it
	var response csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockResponse
	if err := proto.Unmarshal(packet.Body, &response); err != nil {
		LogError("Error unmarshaling response: %v", err)
		return
	}
	
	// Log the response details
	if response.Iteminfo != nil {
		LogInfo("Received item info: DefIndex=%d, PaintIndex=%d, PaintWear=%f, PaintSeed=%d",
			response.Iteminfo.GetDefindex(), 
			response.Iteminfo.GetPaintindex(),
			convertPaintWearToFloat(response.Iteminfo.GetPaintwear()),
			response.Iteminfo.GetPaintseed())
	} else {
		LogWarning("Response contains no item info")
	}
	
	// Always send the response data to the channel, even if it doesn't contain item info
	// This allows the handler to properly respond to the client with an error
	select {
	case h.responseChannel <- packet.Body:
		LogDebug("Sent response data to channel")
	default:
		LogWarning("Failed to send response data to channel (channel full or closed)")
		// Try to clear the channel and send again
		select {
		case <-h.responseChannel:
			// Channel cleared, now try to send again
			select {
			case h.responseChannel <- packet.Body:
				LogInfo("Sent response data to channel after clearing")
			default:
				LogError("Still failed to send response data to channel")
			}
		default:
			LogError("Could not clear channel")
		}
	}
}

// SendHello sends a hello message to the Game Coordinator
func (h *CS2Handler) SendHello() {
	LogInfo("Sending hello to CS2 Game Coordinator...")
	
	// Create an empty message (hello doesn't need any data)
	data := make([]byte, 0)
	
	// Send the hello message to the Game Coordinator
	gcMsg := gamecoordinator.NewGCMsg(CS2AppID, uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientHello), ByteSlice(data))
	h.client.GC.Write(gcMsg)
	
	// Update the last hello time
	h.lastHelloTime = time.Now()
}

// SendGoodbye sends a goodbye message to the Game Coordinator
func (h *CS2Handler) SendGoodbye() {
	LogInfo("Sending goodbye to CS2 Game Coordinator...")
	
	// Create an empty message (goodbye doesn't need any data)
	data := make([]byte, 0)
	
	// Send the goodbye message to the Game Coordinator
	// Use the constant value 4008 directly since the csgoProto package doesn't have this constant
	gcMsg := gamecoordinator.NewGCMsg(CS2AppID, 4008, ByteSlice(data))
	h.client.GC.Write(gcMsg)
}

// RequestItemInfo queues a request for item information
func (h *CS2Handler) RequestItemInfo(paramA, paramD, paramS, paramM uint64) {
	if !h.IsReady() {
		LogWarning("GC not ready, queueing item info request...")
		// Queue the request to be processed when the GC is ready
		h.itemInfoRequest <- struct {
			paramA uint64
			paramD uint64
			paramS uint64
			paramM uint64
		}{paramA, paramD, paramS, paramM}
	} else {
		h.requestItemInfo(paramA, paramD, paramS, paramM)
	}
}

// requestItemInfo sends a request for item information to the Game Coordinator
func (h *CS2Handler) requestItemInfo(paramA, paramD, paramS, paramM uint64) {
	LogInfo("Requesting item info for A:%d D:%d S:%d M:%d", paramA, paramD, paramS, paramM)
	 
	// Clear the response channel before sending a new request
	select {
	case <-h.responseChannel:
		LogDebug("Cleared previous response from channel")
	default:
		// Channel was already empty
	}
	
	// Create a protobuf message for the request
	request := &csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockRequest{
		ParamA: proto.Uint64(paramA), // A parameter
		ParamD: proto.Uint64(paramD), // D parameter
	}
	
	// Set either ParamS or ParamM based on which one is non-zero
	if paramS > 0 {
		request.ParamS = proto.Uint64(paramS) // S parameter is the owner
		request.ParamM = proto.Uint64(0)
	} else if paramM > 0 { 
		request.ParamS = proto.Uint64(0)
		request.ParamM = proto.Uint64(paramM) // M parameter is the market listing ID
	}
	
	// Serialize the protobuf message
	data, err := proto.Marshal(request)
	if err != nil {
		LogError("Error marshaling request: %v", err)
		return
	}
	
	// Log the request data for debugging
	LogDebug("Sending item info request with data length: %d bytes", len(data))
	
	// Send the inspection request to the Game Coordinator
	gcMsg := gamecoordinator.NewGCMsgProtobuf(CS2AppID, uint32(csgoProto.ECsgoGCMsg_k_EMsgGCCStrike15_v2_Client2GCEconPreviewDataBlockRequest), request)
	h.client.GC.Write(gcMsg)
}

// CheckGCConnection checks if the Game Coordinator connection is alive
func (h *CS2Handler) CheckGCConnection() bool {
	// Only check once every 30 seconds to avoid spamming
	if time.Since(h.lastStatusCheck) < 30*time.Second {
		return h.IsReady()
	}
	
	h.lastStatusCheck = time.Now()
	
	// If we're already ready, no need to check
	if h.IsReady() {
		return true
	}
	
	// If not ready and it's been a while since the last hello, send one
	if time.Since(h.lastHelloTime) > HelloInterval {
		LogInfo("CheckGCConnection: Not ready and hello interval elapsed, sending hello")
		h.SendHello()
	}
	
	// Return the current ready state
	return h.IsReady()
}

// Shutdown stops the hello ticker and cleans up resources
func (h *CS2Handler) Shutdown() {
	// Use a mutex to protect the channel close operation
	h.readyMutex.Lock()
	defer h.readyMutex.Unlock()
	
	// Create a non-blocking select to check if the channel is already closed
	select {
	case _, ok := <-h.stopHelloTicker:
		if !ok {
			// Channel is already closed, do nothing
			return
		}
	default:
		// Channel is still open, close it
		close(h.stopHelloTicker)
	}
}

// ExtractItemInfo extracts item information from a response packet
func ExtractItemInfo(responseData []byte) (*ItemInfo, error) {
	// Parse the protobuf message
	var response csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockResponse
	if err := proto.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}
	
	// Check if we have item info
	if response.Iteminfo == nil {
		return nil, fmt.Errorf("response contains no item info")
	}
	
	// Create a new ItemInfo struct
	itemInfo := &ItemInfo{
		// Extract all fields from the response
		AccountId:         response.Iteminfo.GetAccountid(),
		ItemId:            response.Iteminfo.GetItemid(),
		DefIndex:          response.Iteminfo.GetDefindex(),
		PaintIndex:        response.Iteminfo.GetPaintindex(),
		Rarity:            response.Iteminfo.GetRarity(),
		Quality:           response.Iteminfo.GetQuality(),
		PaintWear:         convertPaintWearToFloat(response.Iteminfo.GetPaintwear()),
		PaintSeed:         response.Iteminfo.GetPaintseed(),
		KilleaterScoreType: response.Iteminfo.GetKilleaterscoretype(),
		KilleaterValue:    getKilleaterValueOrNegativeOne(response.Iteminfo),
		CustomName:        response.Iteminfo.GetCustomname(),
		Inventory:         response.Iteminfo.GetInventory(),
		Origin:            response.Iteminfo.GetOrigin(),
		QuestId:           response.Iteminfo.GetQuestid(),
		DropReason:        response.Iteminfo.GetDropreason(),
		MusicIndex:        response.Iteminfo.GetMusicindex(),
		EntIndex:          response.Iteminfo.GetEntindex(),
		PetIndex:          response.Iteminfo.GetPetindex(),
		// Initialize stickers and keychains as empty arrays
		Stickers:          []StickerInfo{},
		Keychains:         []StickerInfo{},
	} 

	LogDebug("ItemInfo: %+v", response.Iteminfo)
	
	// Determine if the item is Souvenir
	// Origin 12 is tournament drops (Souvenir)
	itemInfo.IsSouvenir = itemInfo.Quality == 12 
	
	// Determine if the item is StatTrak based on KilleaterScoreType
	// If KilleaterScoreType is set (greater than 0), the item has a StatTrak counter
	itemInfo.IsStatTrak = itemInfo.KilleaterScoreType > 0 && itemInfo.Quality != 12
	
	
	// Extract sticker information
	if len(response.Iteminfo.GetStickers()) > 0 {
		for _, sticker := range response.Iteminfo.GetStickers() {
			stickerInfo := StickerInfo{
				Slot:      sticker.GetSlot(),
				ID:        sticker.GetStickerId(),
				Wear:      sticker.GetWear(),
				Scale:     sticker.GetScale(),
				Rotation:  sticker.GetRotation(),
				TintId:    sticker.GetTintId(),
				OffsetX:   sticker.GetOffsetX(),
				OffsetY:   sticker.GetOffsetY(),
				OffsetZ:   sticker.GetOffsetZ(),
				Pattern:   sticker.GetPattern(),
			}
			
			// Add sticker name from schema if available
			s := GetSchema()
			if s != nil {
				stickerIDStr := fmt.Sprintf("%d", stickerInfo.ID)
				if stickerData, ok := s.Stickers[stickerIDStr]; ok {
					stickerInfo.Name = stickerData.MarketHashName
				}
			}
			
			itemInfo.Stickers = append(itemInfo.Stickers, stickerInfo)
		}
	}
	
	// Extract keychain information
	if len(response.Iteminfo.GetKeychains()) > 0 {
		for _, keychain := range response.Iteminfo.GetKeychains() {
			keychainInfo := StickerInfo{
				Slot:      keychain.GetSlot(),
				ID:        keychain.GetStickerId(),
				Wear:      keychain.GetWear(),
				Scale:     keychain.GetScale(),
				Rotation:  keychain.GetRotation(),
				TintId:    keychain.GetTintId(),
				OffsetX:   keychain.GetOffsetX(),
				OffsetY:   keychain.GetOffsetY(),
				OffsetZ:   keychain.GetOffsetZ(),
				Pattern:   keychain.GetPattern(),
			}
			
			// Add keychain name from schema if available
			s := GetSchema()
			if s != nil {
				keychainIDStr := fmt.Sprintf("%d", keychainInfo.ID)
				if keychainData, ok := s.Keychains[keychainIDStr]; ok {
					keychainInfo.Name = keychainData.MarketHashName
				}
			}
			
			itemInfo.Keychains = append(itemInfo.Keychains, keychainInfo)
		}
	}
	
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
		} else if defIndexStr == "1209" {
			itemInfo.Type = "Sticker"
		} else if defIndexStr == "1349" || defIndexStr == "1348" {
			itemInfo.Type = "Graffiti"
		} else if defIndexStr == "1355" {
			itemInfo.Type = "Keychain"
		} else if _, ok := s.Agents[defIndexStr]; ok {
			itemInfo.Type = "Agent"
		} else {
			itemInfo.Type = "Unknown"
		}
	}
	
	return itemInfo, nil
}

// getKilleaterValueOrNegativeOne returns KilleaterNotSet if the Killeatervalue is nil, otherwise returns the actual value
func getKilleaterValueOrNegativeOne(itemInfo *csgoProto.CEconItemPreviewDataBlock) int32 { 
	if itemInfo != nil && itemInfo.Killeatervalue != nil {
		return int32(*itemInfo.Killeatervalue)
	} 
	return -1
}

// convertPaintWearToFloat converts the uint32 paint wear value to a float64 between 0 and 1
func convertPaintWearToFloat(paintWear uint32) float64 {
	// For CS2, paint wear is stored as a IEEE 754 binary32 float
	// We need to interpret the uint32 bits as a float32
	return float64(math.Float32frombits(paintWear))
} 