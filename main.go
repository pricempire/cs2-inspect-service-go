package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/base64"

	goSteam "github.com/Philipp15b/go-steam/v3"
	csgoProto "github.com/Philipp15b/go-steam/v3/csgo/protocol/protobuf"
	"github.com/Philipp15b/go-steam/v3/protocol/gamecoordinator"
	"github.com/joho/godotenv"
	"golang.org/x/net/proxy"
	"google.golang.org/protobuf/proto"
)

// Constants for CS2 Game Coordinator messages
const (
	CS2AppID                                            = 730
	ClientRequestTimeout                                = 60 * time.Second 
)

// ByteSlice is a wrapper for []byte that implements protocol.Serializer
type ByteSlice []byte

func (b ByteSlice) Serialize(w io.Writer) error {
	_, err := w.Write(b)
	return err
}

// Account represents a Steam account
type Account struct {
	Username string
	Password string
	SentryHash string
	SharedSecret string
}

// Bot represents a Steam bot
type Bot struct {
	Account     Account
	Client      *goSteam.Client
	CS2Handler  *CS2Handler
	Connected   bool
	LoggedOn    bool
	Busy        bool
	LastUsed    time.Time
	Mutex       sync.Mutex
}

// CS2Handler handles all CS2-related functionality
type CS2Handler struct {
	client          *goSteam.Client
	ready           bool
	readyMutex      sync.RWMutex
	itemInfoRequest chan struct {
		paramA uint64
		paramD uint64
		owner  uint64
	}
	responseChannel chan []byte
	lastHelloTime   time.Time
	lastStatusCheck time.Time
}

// StickerInfo represents information about a sticker
type StickerInfo struct {
	ID       uint32  `json:"id"`
	Wear     float32 `json:"wear"`
	Scale    float32 `json:"scale"`
	Rotation float32 `json:"rotation"`
}

// ItemInfo represents detailed information about an item
type ItemInfo struct {
	DefIndex   uint32        `json:"def_index"`
	PaintIndex uint32        `json:"paint_index"`
	Rarity     uint32        `json:"rarity"`
	Quality    uint32        `json:"quality"`
	PaintWear  uint32        `json:"paint_wear"`
	PaintSeed  uint32        `json:"paint_seed"`
	CustomName string        `json:"custom_name,omitempty"`
	Stickers   []StickerInfo `json:"stickers,omitempty"`
}

// InspectResponse represents the response from the inspect request
type InspectResponse struct {
	Success bool     `json:"success"`
	Data    []byte   `json:"data,omitempty"`
	Error   string   `json:"error,omitempty"`
	ItemInfo *ItemInfo `json:"itemInfo,omitempty"`
}

// BotStatus represents the status of a bot
type BotStatus struct {
	Username  string `json:"username"`
	Connected bool   `json:"connected"`
	LoggedOn  bool   `json:"loggedOn"`
	Ready     bool   `json:"ready"`
	Busy      bool   `json:"busy"`
}

// HealthResponse represents the response from the health check
type HealthResponse struct {
	Status string      `json:"status"`
	Bots   []BotStatus `json:"bots"`
}

// Global variables
var (
	bots     []*Bot
	botMutex sync.Mutex
)

// NewCS2Handler creates a new CS2Handler
func NewCS2Handler(client *goSteam.Client) *CS2Handler {
	return &CS2Handler{
		client: client,
		ready:  false,
		itemInfoRequest: make(chan struct {
			paramA uint64
			paramD uint64
			owner  uint64
		}, 10), // Buffer for up to 10 requests
		responseChannel: make(chan []byte, 10), // Increase buffer size to 10 to reduce chance of blocking
		lastHelloTime:   time.Time{},
		lastStatusCheck: time.Time{},
	}
}

// IsReady returns whether the handler is ready to process requests
func (h *CS2Handler) IsReady() bool {
	h.readyMutex.RLock()
	defer h.readyMutex.RUnlock()
	return h.ready
}

// SetReady sets the ready state of the handler
func (h *CS2Handler) SetReady(ready bool) {
	h.readyMutex.Lock()
	defer h.readyMutex.Unlock()
	h.ready = ready
}

// HandleGCPacket handles Game Coordinator packets
func (h *CS2Handler) HandleGCPacket(packet *gamecoordinator.GCPacket) {
	log.Printf("HandleGCPacket called with AppId: %d, MsgType: %d (0x%x)", packet.AppId, packet.MsgType, packet.MsgType)
	
	if packet.AppId != CS2AppID {
		log.Printf("Ignoring packet for AppId: %d", packet.AppId)
		return
	}  

	switch packet.MsgType {
	case uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientWelcome):
		h.SetReady(true)
		log.Println("Connected to CS2 Game Coordinator!")
		
		// Process any pending item info requests
		select {
		case req := <-h.itemInfoRequest:
			log.Printf("Processing pending item info request")
			h.requestItemInfo(req.paramA, req.paramD, req.owner)
		default:
			// No pending requests
		}
	
	case uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientConnectionStatus):
		log.Println("Received connection status message from GC")
		
		// The connection status message indicates we're connected to the GC
		// Set ready to true if it's not already
		if !h.IsReady() {
			log.Println("Setting ready state to true based on connection status message")
			h.SetReady(true)
		}
	
	case uint32(csgoProto.ECsgoGCMsg_k_EMsgGCCStrike15_v2_Client2GCEconPreviewDataBlockResponse):
		// Handle the item info response
		h.HandleItemInfoResponse(packet)
	default:
		log.Printf("Received unknown GC message type: %d (0x%x)", packet.MsgType, packet.MsgType)
	}
}

// HandleItemInfoResponse handles the response from the Game Coordinator for an item info request
func (h *CS2Handler) HandleItemInfoResponse(packet *gamecoordinator.GCPacket) {
	log.Printf("Received item info response with %d bytes of data", len(packet.Body))
	
	// Parse the protobuf message to validate it
	var response csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockResponse
	if err := proto.Unmarshal(packet.Body, &response); err != nil {
		log.Printf("Error unmarshaling response: %v", err)
		return
	}
	
	// Log the response details
	if response.Iteminfo != nil {
		log.Printf("Received item info: DefIndex=%d, PaintIndex=%d, PaintWear=%d, PaintSeed=%d",
			response.Iteminfo.GetDefindex(), 
			response.Iteminfo.GetPaintindex(),
			response.Iteminfo.GetPaintwear(),
			response.Iteminfo.GetPaintseed())
	} else {
		log.Println("Response contains no item info")
	}
	
	// Always send the response data to the channel, even if it doesn't contain item info
	// This allows the handler to properly respond to the client with an error
	select {
	case h.responseChannel <- packet.Body:
		log.Println("Sent response data to channel")
	default:
		log.Println("Failed to send response data to channel (channel full or closed)")
		// Try to clear the channel and send again
		select {
		case <-h.responseChannel:
			// Channel cleared, now try to send again
			select {
			case h.responseChannel <- packet.Body:
				log.Println("Sent response data to channel after clearing")
			default:
				log.Println("Still failed to send response data to channel")
			}
		default:
			log.Println("Could not clear channel")
		}
	}
}

// SendHello sends a hello message to the Game Coordinator
func (h *CS2Handler) SendHello() {
	log.Println("Sending hello to CS2 Game Coordinator...")
	
	// Create an empty message (hello doesn't need any data)
	data := make([]byte, 0)
	
	// Send the hello message to the Game Coordinator
	gcMsg := gamecoordinator.NewGCMsg(CS2AppID, uint32(csgoProto.EGCBaseClientMsg_k_EMsgGCClientHello), ByteSlice(data))
	h.client.GC.Write(gcMsg)
	
	// Update the last hello time
	h.lastHelloTime = time.Now()
}

// RequestItemInfo queues a request for item information
func (h *CS2Handler) RequestItemInfo(paramA, paramD, owner uint64) {
	if h.IsReady() {
		log.Printf("GC is ready, requesting item info immediately...")
		h.requestItemInfo(paramA, paramD, owner)
	} else {
		log.Printf("GC not ready, queueing item info request...")
		// Queue the request to be processed when the GC is ready
		h.itemInfoRequest <- struct {
			paramA uint64
			paramD uint64
			owner  uint64
		}{paramA, paramD, owner}
	}
}

// requestItemInfo sends a request for item information to the Game Coordinator
func (h *CS2Handler) requestItemInfo(paramA, paramD, owner uint64) {
	log.Printf("Requesting item info for A:%d D:%d Owner:%d", paramA, paramD, owner)
	 
	// Clear the response channel before sending a new request
	select {
	case <-h.responseChannel:
		log.Println("Cleared previous response from channel")
	default:
		// Channel was already empty
	}
	
	// Create a protobuf message for the request
	request := &csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockRequest{
		ParamS: proto.Uint64(owner),  // S parameter is the owner
		ParamA: proto.Uint64(paramA), // A parameter
		ParamD: proto.Uint64(paramD), // D parameter
	}
	
	// Serialize the protobuf message
	data, err := proto.Marshal(request)
	if err != nil {
		log.Printf("Error marshaling request: %v", err)
		return
	}
	
	// Log the request data for debugging
	log.Printf("Sending item info request with data length: %d bytes", len(data))
	
	// Send the inspection request to the Game Coordinator
	gcMsg := gamecoordinator.NewGCMsgProtobuf(CS2AppID, uint32(csgoProto.ECsgoGCMsg_k_EMsgGCCStrike15_v2_Client2GCEconPreviewDataBlockRequest), &csgoProto.CMsgGCCStrike15V2_Client2GCEconPreviewDataBlockRequest{
		ParamS: proto.Uint64(owner),
		ParamA: proto.Uint64(paramA),
		ParamD: proto.Uint64(paramD),
	})
	h.client.GC.Write(gcMsg)
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
		DefIndex:   response.Iteminfo.GetDefindex(),
		PaintIndex: response.Iteminfo.GetPaintindex(),
		Rarity:     response.Iteminfo.GetRarity(),
		Quality:    response.Iteminfo.GetQuality(),
		PaintWear:  response.Iteminfo.GetPaintwear(),
		PaintSeed:  response.Iteminfo.GetPaintseed(),
		CustomName: response.Iteminfo.GetCustomname(),
	}
	
	// Extract sticker information
	if len(response.Iteminfo.GetStickers()) > 0 {
		for _, sticker := range response.Iteminfo.GetStickers() {
			stickerInfo := StickerInfo{
				ID:       sticker.GetStickerId(),
				Wear:     sticker.GetWear(),
				Scale:    sticker.GetScale(),
				Rotation: sticker.GetRotation(),
			}
			itemInfo.Stickers = append(itemInfo.Stickers, stickerInfo)
		}
	} 
	
	return itemInfo, nil
}

// parseInspectLink parses a CS:GO inspect link
func parseInspectLink(link string) (paramA, paramD uint64, owner uint64, err error) {
	r := regexp.MustCompile(`S(\d+)A(\d+)D(\d+)`)
	matches := r.FindStringSubmatch(link)
	if len(matches) != 4 {
		return 0, 0, 0, fmt.Errorf("invalid inspect link format")
	}

	ownerTemp, _ := strconv.ParseUint(matches[1], 10, 64)
	owner = ownerTemp
	paramA, _ = strconv.ParseUint(matches[2], 10, 64)
	paramD, _ = strconv.ParseUint(matches[3], 10, 64)

	return paramA, paramD, owner, nil
}

// loadAccounts loads Steam accounts from accounts.txt or the file specified in ACCOUNTS_FILE env var
func loadAccounts() ([]Account, error) {
	// Get accounts file from env or use default
	accountsFile := os.Getenv("ACCOUNTS_FILE")
	if accountsFile == "" {
		accountsFile = "accounts.txt"
	}
	
	file, err := os.Open(accountsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var accounts []Account
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 4 {
			account := Account{
				Username:     parts[0],
				Password:     parts[1],
				SentryHash:   parts[2],
				SharedSecret: parts[3],
			}
			accounts = append(accounts, account)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// initializeBots initializes all Steam bots
func initializeBots(accounts []Account) {
	// Get the base proxy URL from environment
	baseProxyURL := os.Getenv("PROXY_URL")
	
	for i, account := range accounts {
		// Create a unique proxy dialer for each account
		var proxyDialer proxy.Dialer
		
		if baseProxyURL != "" {
			// Clone the proxy URL for this account
			proxyURL := baseProxyURL
			
			// Clean the proxy URL - remove any non-ASCII characters
			proxyURL = strings.Map(func(r rune) rune {
				if r > 127 {
					return -1
				}
				return r
			}, proxyURL)
			
			// Replace [session] with a unique session ID for this account
			// Use account index + timestamp to ensure uniqueness
			sessionID := strconv.Itoa(i) + "_" + strconv.Itoa(int(time.Now().Unix() % 1000))
			proxyURL = strings.Replace(proxyURL, "[session]", sessionID, -1)
			log.Printf("Account %s: Using proxy with session ID: %s", account.Username, sessionID)
			
			// Parse the proxy URL
			parsedURL, err := url.Parse(proxyURL)
			if err != nil {
				log.Printf("Error parsing proxy URL for account %s: %v", account.Username, err)
			} else {
				log.Printf("Account %s: Configuring proxy: %s://%s", account.Username, parsedURL.Scheme, parsedURL.Host)
				
				// Handle different proxy types
				switch parsedURL.Scheme {
				case "socks5":
					// SOCKS5 proxy
					dialer, err := proxy.FromURL(parsedURL, proxy.Direct)
					if err != nil {
						log.Printf("Error creating SOCKS5 proxy dialer: %v", err)
					} else {
						proxyDialer = dialer
						log.Printf("Account %s: SOCKS5 proxy configured successfully: %s", account.Username, parsedURL.Host)
					}
					
				case "http", "https":
					// HTTP proxy - we need to create a custom dialer
					proxyDialer = &httpProxyDialer{
						proxyURL: parsedURL,
					}
					log.Printf("Account %s: HTTP proxy configured successfully: %s", account.Username, parsedURL.Host)
					
				default:
					log.Printf("Unsupported proxy scheme: %s", parsedURL.Scheme)
				}
			}
		}

		bot := &Bot{
			Account:   account,
			Connected: false,
			LoggedOn:  false,
			Busy:      false,
			LastUsed:  time.Now(),
		}
		bots = append(bots, bot)
		go startBot(bot, proxyDialer)
	}
}

// httpProxyDialer implements the proxy.Dialer interface for HTTP proxies
type httpProxyDialer struct {
	proxyURL *url.URL
}

// Dial connects to the address through the HTTP proxy
func (d *httpProxyDialer) Dial(network, addr string) (net.Conn, error) {
	// For HTTP proxies, we need to use CONNECT method to establish a connection
	proxyAddr := d.proxyURL.Host
	if d.proxyURL.Port() == "" {
		if d.proxyURL.Scheme == "https" {
			proxyAddr = proxyAddr + ":443"
		} else {
			proxyAddr = proxyAddr + ":80"
		}
	}
	
	// Connect to the proxy server
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to proxy %s: %v", proxyAddr, err)
	}
	
	// Send a CONNECT request
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	
	// Add proxy authentication if needed
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		auth := username + ":" + password
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		req += fmt.Sprintf("Proxy-Authorization: %s\r\n", basicAuth)
	}
	
	// End the request
	req += "\r\n"
	
	// Send the request to the proxy
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("error sending CONNECT request: %v", err)
	}
	
	// Read the response
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("error reading proxy response: %v", err)
	}
	defer resp.Body.Close()
	
	// Check if the connection was established
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", resp.Status)
	}
	
	return conn, nil
}

// startBot starts a Steam bot
func startBot(bot *Bot, proxyDialer proxy.Dialer) {
	for {
		bot.Mutex.Lock()
		if bot.Client == nil {
			bot.Client = goSteam.NewClient()
			bot.CS2Handler = NewCS2Handler(bot.Client)
			bot.Client.GC.RegisterPacketHandler(bot.CS2Handler)
			
			// Set proxy dialer if available
			if proxyDialer != nil {
				// Create a pointer to the proxy.Dialer interface
				// This is a workaround for the unusual API design in go-steam
				proxyDialerPtr := &proxyDialer
				bot.Client.SetProxyDialer(proxyDialerPtr)
				log.Printf("Bot %s: Using proxy for connection", bot.Account.Username)
			}
		}
		bot.Mutex.Unlock()

		// Connect to Steam
		log.Printf("Bot %s: Connecting to Steam...", bot.Account.Username)
		bot.Client.Connect()

		// Handle events
		for event := range bot.Client.Events() {
			switch e := event.(type) {
			case *goSteam.ConnectedEvent:
				log.Printf("Bot %s: Connected to Steam", bot.Account.Username)
				bot.Mutex.Lock()
				bot.Connected = true
				bot.Mutex.Unlock()

				// Log on
				loginDetails := &goSteam.LogOnDetails{
					Username: bot.Account.Username,
					Password: bot.Account.Password,
				}
				bot.Client.Auth.LogOn(loginDetails)

			case *goSteam.LoggedOnEvent:
				log.Printf("Bot %s: Logged on to Steam", bot.Account.Username)
				bot.Mutex.Lock()
				bot.LoggedOn = true
				bot.Mutex.Unlock()

				// Set game played
				bot.Client.GC.SetGamesPlayed(CS2AppID)
				
				// Send a single hello after a short delay
				go func() {
					time.Sleep(5 * time.Second)
					bot.CS2Handler.SendHello()
					log.Printf("Bot %s: Sent initial hello to Game Coordinator", bot.Account.Username)
				}()
				
			case *goSteam.DisconnectedEvent:
				log.Printf("Bot %s: Disconnected from Steam: %v", bot.Account.Username, e)
				bot.Mutex.Lock()
				bot.Connected = false
				bot.LoggedOn = false
				bot.CS2Handler.SetReady(false)
				bot.Mutex.Unlock()
				
				// Reconnect after a delay
				time.Sleep(10 * time.Second)
				break
			}
		}

		log.Printf("Bot %s: Event loop ended, reconnecting in 10 seconds...", bot.Account.Username)
		time.Sleep(10 * time.Second)
	}
}

// getAvailableBot returns an available bot
func getAvailableBot() *Bot {
	botMutex.Lock()
	defer botMutex.Unlock()

	// Find a bot that is connected, logged on, ready, and not busy
	for _, bot := range bots {
		bot.Mutex.Lock()
		if bot.Connected && bot.LoggedOn && bot.CS2Handler.IsReady() && !bot.Busy {
			bot.Busy = true
			bot.LastUsed = time.Now()
			bot.Mutex.Unlock()
			return bot
		}
		bot.Mutex.Unlock()
	}

	// If no bot is available, find the least recently used bot that is connected and logged on
	var leastRecentlyUsedBot *Bot
	var leastRecentTime time.Time

	for _, bot := range bots {
		bot.Mutex.Lock()
		if bot.Connected && bot.LoggedOn && bot.CS2Handler.IsReady() && 
			(leastRecentlyUsedBot == nil || bot.LastUsed.Before(leastRecentTime)) {
			leastRecentlyUsedBot = bot
			leastRecentTime = bot.LastUsed
		}
		bot.Mutex.Unlock()
	}

	if leastRecentlyUsedBot != nil {
		leastRecentlyUsedBot.Mutex.Lock()
		leastRecentlyUsedBot.Busy = true
		leastRecentlyUsedBot.LastUsed = time.Now()
		leastRecentlyUsedBot.Mutex.Unlock()
		return leastRecentlyUsedBot
	}

	return nil
}

// releaseBot marks a bot as not busy
func releaseBot(bot *Bot) {
	bot.Mutex.Lock()
	defer bot.Mutex.Unlock()
	bot.Busy = false
}

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

// sendJSONResponse sends a JSON response
func sendJSONResponse(w http.ResponseWriter, response InspectResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
	
	// Simply return the current ready state
	return h.IsReady()
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
						log.Printf("Bot %s: Status - Connected: %v, LoggedOn: %v", 
							bot.Account.Username, bot.Connected, bot.LoggedOn)
					}
					
					bot.Mutex.Unlock()
				}
				log.Printf("Bot status check: %d/%d bots ready", readyCount, len(bots))
				botMutex.Unlock()
			}
		}
	}()

	// Set up HTTP server
	http.HandleFunc("/inspect", handleInspect)
	http.HandleFunc("/health", handleHealth)
	
	// Start HTTP server
	log.Printf("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
