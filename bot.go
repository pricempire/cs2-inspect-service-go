package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	goSteam "github.com/Philipp15b/go-steam/v3"
	"github.com/Philipp15b/go-steam/v3/protocol/steamlang"
)

// Constants for bot management
const (
	// Reconnection settings
	InitialReconnectDelay  = 10 * time.Second
	MaxReconnectDelay      = 5 * time.Minute
	ReconnectBackoffFactor = 1.5
	
	// Health check intervals
	BotHealthCheckInterval = 1 * time.Minute
	
	// Timeouts
	LoginTimeout           = 30 * time.Second
	GCConnectionTimeout    = 30 * time.Second
	
	// Bot states
	BotStateDisconnected   = "disconnected"
	BotStateConnecting     = "connecting"
	BotStateConnected      = "connected"
	BotStateLoggingIn      = "logging_in"
	BotStateLoggedIn       = "logged_in"
	BotStateReady          = "ready"
	BotStateBusy           = "busy"
)

// BotManager handles the lifecycle of all bots
type BotManager struct {
	bots       []*Bot
	mutex      sync.RWMutex
	ctx        context.Context
	cancelFunc context.CancelFunc
	
	// Worker pool for bot operations
	workerPool     chan struct{}
	maxConcurrent  int
	
	// Communication channels
	commandChan    chan botCommand
	responseChan   chan botResponse
	
	// Status tracking
	isInitialized  bool
	initError      error
	initComplete   bool // Flag to track when initialization is complete
	
	// Reconnection queue
	reconnectQueue     chan *Bot
	reconnectQueueSize int
	reconnectMutex     sync.Mutex
}

// botCommand represents a command sent to the bot manager thread
type botCommand struct {
	action      string
	botUsername string
	data        interface{}
	resultChan  chan botResponse
}

// botResponse represents a response from the bot manager thread
type botResponse struct {
	success bool
	message string
	data    interface{}
}

// NewBotManager creates a new bot manager
func NewBotManager() *BotManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Determine max concurrent bot operations from env or use default
	maxConcurrent := 5000
	
	// Try to get the max concurrent value from environment variable
	if maxConcurrentStr := os.Getenv("MAX_CONCURRENT_BOTS"); maxConcurrentStr != "" {
		if value, err := strconv.Atoi(maxConcurrentStr); err == nil && value > 0 {
			maxConcurrent = value
			LogInfo("Using MAX_CONCURRENT_BOTS=%d from environment", maxConcurrent)
		} else {
			LogWarning("Invalid MAX_CONCURRENT_BOTS value: %s, using default: %d", maxConcurrentStr, maxConcurrent)
		}
	}
	
	bm := &BotManager{
		bots:          make([]*Bot, 0),
		ctx:           ctx,
		cancelFunc:    cancel,
		workerPool:    make(chan struct{}, maxConcurrent),
		maxConcurrent: maxConcurrent,
		commandChan:   make(chan botCommand, 100),
		responseChan:  make(chan botResponse, 100),
		isInitialized: false,
		reconnectQueue: make(chan *Bot, 100),
	}
	
	// Start the reconnection queue processor
	go bm.processReconnectQueue()
	
	return bm
}

// Initialize loads accounts and starts bots in a completely separate goroutine
func (bm *BotManager) Initialize() error {
	// Start the dedicated bot manager thread
	go bm.botManagerThread()
	
	LogInfo("Bot manager thread started")
	return nil
}

// botManagerThread is the main thread for bot management
func (bm *BotManager) botManagerThread() {
	LogInfo("Starting bot manager thread...")
	
	// Load accounts
	accounts, err := loadAccounts()
	if err != nil {
		LogError("Failed to load accounts: %v", err)
		bm.initError = err
		return
	}
	
	LogInfo("Loaded %d accounts", len(accounts))
	
	// Create all bot instances first
	for i, account := range accounts {
		// Create the bot instance
		bot := NewBot(account, bm.ctx)
		
		// Add the bot to the manager's list
		bm.mutex.Lock()
		bm.bots = append(bm.bots, bot)
		bm.mutex.Unlock()
		
		LogInfo("Created bot %d/%d: %s", i+1, len(accounts), account.Username)
	}
	
	// Initialize bots with a non-blocking approach
	totalBots := len(bm.bots)
	LogInfo("Starting initialization of %d bots with max concurrency of %d", totalBots, bm.maxConcurrent)
	
	// Create a channel to queue all bots for initialization
	botQueue := make(chan *Bot, totalBots)
	
	// Queue all bots for initialization
	go func() {
		for _, bot := range bm.bots {
			botQueue <- bot
		}
		close(botQueue)
		LogInfo("All %d bots have been queued for initialization", totalBots)
	}()
	
	// Create a wait group to track completion
	var wg sync.WaitGroup
	wg.Add(totalBots)
	
	// Create counters for tracking initialization progress
	var (
		initCounter     int32
		initMutex       sync.Mutex
		lastProgressLog time.Time = time.Now()
	)
	
	// Function to update and log progress
	updateProgress := func() {
		initMutex.Lock()
		defer initMutex.Unlock()
		
		initCounter++
		current := initCounter
		total := int32(totalBots)
		
		// Log progress every 10 bots or every 30 seconds, whichever comes first
		if current%10 == 0 || time.Since(lastProgressLog) > 30*time.Second {
			percentage := float64(current) / float64(total) * 100
			LogInfo("Bot initialization progress: %d/%d (%.1f%%)", current, total, percentage)
			lastProgressLog = time.Now()
		}
	}
	
	// Start worker goroutines to initialize bots
	for i := 0; i < bm.maxConcurrent; i++ {
		workerID := i + 1
		go func(id int) {
			LogInfo("Starting worker %d for bot initialization", id)
			for bot := range botQueue {
				// Initialize this bot
				LogInfo("Worker %d: Starting initialization of bot %s", id, bot.account.Username)
				
				// Create a timeout channel for this bot's initialization
				initTimeout := time.After(3 * time.Minute)
				
				// Create a channel to signal completion
				done := make(chan bool, 1)
				
				// Start the bot in a separate goroutine
				go func() {
					// Start the bot
					bot.Start()
					
					// Signal completion
					done <- true
				}()
				
				// Wait for either completion or timeout
				select {
				case <-done:
					// Bot initialization completed successfully
					LogInfo("Worker %d: Bot %s initialization complete", id, bot.account.Username)
				case <-initTimeout:
					// Bot initialization timed out
					LogWarning("Worker %d: Bot %s initialization timed out after 3 minutes", id, bot.account.Username)
					
					// Force disconnect the bot to clean up resources
					bot.mutex.Lock()
					if bot.client != nil {
						bot.client.Disconnect()
						bot.client = nil
					}
					if bot.cs2Handler != nil {
						bot.cs2Handler.Shutdown()
						bot.cs2Handler = nil
					}
					bot.state = BotStateDisconnected
					bot.mutex.Unlock()
				}
				
				// Update progress counter regardless of success or timeout
				updateProgress()
				
				// Mark this bot as done
				wg.Done()
			}
			LogInfo("Worker %d: Finished processing bot queue", id)
		}(workerID)
	}
	
	// Wait for all initializations to complete in a separate goroutine
	go func() {
		// Wait for all bots to initialize
		wg.Wait()
		LogInfo("All %d bots have been fully initialized", totalBots)
		
		// Count how many bots are in each state
		bm.mutex.RLock()
		readyCount := 0
		loggedInCount := 0
		disconnectedCount := 0
		otherCount := 0
		
		for _, bot := range bm.bots {
			bot.mutex.Lock()
			switch bot.state {
			case BotStateReady:
				readyCount++
			case BotStateLoggedIn:
				loggedInCount++
			case BotStateDisconnected:
				disconnectedCount++
			default:
				otherCount++
			}
			bot.mutex.Unlock()
		}
		bm.mutex.RUnlock()
		
		LogInfo("Bot initialization summary: Ready=%d, LoggedIn=%d, Disconnected=%d, Other=%d",
			readyCount, loggedInCount, disconnectedCount, otherCount)
		
		// Mark initialization as complete
		bm.mutex.Lock()
		bm.initComplete = true
		bm.mutex.Unlock()
		
		// Now that initialization is complete, start health monitoring
		go bm.monitorBotHealth()
		LogInfo("Bot health monitoring started after initialization completed")
	}()
	
	// Mark as initialized - the bots will continue to initialize in the background
	bm.isInitialized = true
	
	// Process commands
	for {
		select {
		case cmd := <-bm.commandChan:
			// Process command
			bm.processCommand(cmd)
			
		case <-bm.ctx.Done():
			LogInfo("Bot manager thread shutting down")
			return
		}
	}
}

// processCommand handles commands sent to the bot manager thread
func (bm *BotManager) processCommand(cmd botCommand) {
	switch cmd.action {
	case "getBot":
		// Get an available bot
		bot := bm.getAvailableBotInternal()
		if bot != nil {
			cmd.resultChan <- botResponse{
				success: true,
				data:    bot,
			}
		} else {
			cmd.resultChan <- botResponse{
				success: false,
				message: "No available bot found",
			}
		}
		
	case "releaseBot":
		// Release a bot
		if bot, ok := cmd.data.(*Bot); ok {
			bm.releaseBotInternal(bot)
			cmd.resultChan <- botResponse{
				success: true,
			}
		} else {
			cmd.resultChan <- botResponse{
				success: false,
				message: "Invalid bot data",
			}
		}
		
	case "reconnectBot":
		// Reconnect a specific bot
		var targetBot *Bot
		
		bm.mutex.RLock()
		for _, bot := range bm.bots {
			bot.mutex.Lock()
			if bot.account.Username == cmd.botUsername {
				targetBot = bot
				bot.mutex.Unlock()
				break
			}
			bot.mutex.Unlock()
		}
		bm.mutex.RUnlock()
		
		if targetBot != nil {
			// Start reconnect in a worker
			go func(b *Bot, resChan chan botResponse) {
				// Acquire a worker slot
				bm.workerPool <- struct{}{}
				defer func() {
					// Release the worker slot when done
					<-bm.workerPool
				}()
				
				success := b.Reconnect()
				resChan <- botResponse{
					success: success,
					message: fmt.Sprintf("Bot %s reconnect %s", b.account.Username, 
						map[bool]string{true: "succeeded", false: "failed"}[success]),
				}
			}(targetBot, cmd.resultChan)
		} else {
			cmd.resultChan <- botResponse{
				success: false,
				message: fmt.Sprintf("Bot %s not found", cmd.botUsername),
			}
		}
		
	default:
		cmd.resultChan <- botResponse{
			success: false,
			message: fmt.Sprintf("Unknown command: %s", cmd.action),
		}
	}
}

// GetAvailableBot returns a bot that is ready to handle requests
func (bm *BotManager) GetAvailableBot() *Bot {
	// Send command to bot manager thread
	resultChan := make(chan botResponse, 1)
	bm.commandChan <- botCommand{
		action:     "getBot",
		resultChan: resultChan,
	}
	
	// Wait for response
	select {
	case resp := <-resultChan:
		if resp.success {
			return resp.data.(*Bot)
		}
		return nil
	case <-time.After(5 * time.Second):
		LogWarning("Timeout waiting for available bot")
		return nil
	}
}

// getAvailableBotInternal is the internal implementation of GetAvailableBot
// that runs in the bot manager thread
func (bm *BotManager) getAvailableBotInternal() *Bot {
	bm.mutex.RLock()
	defer bm.mutex.RUnlock()
	
	// First, try to find a bot that is ready and not busy
	for _, bot := range bm.bots {
		bot.mutex.Lock()
		
		// Double-check that the bot is actually ready by checking GC readiness
		// if it's been more than 30 seconds since the last check
		if bot.state == BotStateReady && time.Since(bot.lastGCCheck) > 30*time.Second {
			if bot.client != nil && bot.cs2Handler != nil {
				isReady := bot.cs2Handler.IsReady()
				bot.lastGCCheck = time.Now()
				
				// Update state if GC is not ready
				if !isReady {
					LogInfo("Bot %s: GC is not ready when getting bot, updating state from %s to %s", 
						bot.account.Username, bot.state, BotStateLoggedIn)
					bot.state = BotStateLoggedIn
				}
			}
		}
		
		// Now check if the bot is ready
		if bot.state == BotStateReady {
			bot.state = BotStateBusy
			bot.lastUsed = time.Now()
			bot.mutex.Unlock()
			return bot
		}
		bot.mutex.Unlock()
	}
	
	// If no ready bot is found, try to find the least recently used bot that is logged in
	var leastRecentlyUsedBot *Bot
	var leastRecentTime time.Time
	
	for _, bot := range bm.bots {
		bot.mutex.Lock()
		if bot.state == BotStateLoggedIn && (leastRecentlyUsedBot == nil || bot.lastUsed.Before(leastRecentTime)) {
			leastRecentlyUsedBot = bot
			leastRecentTime = bot.lastUsed
		}
		bot.mutex.Unlock()
	}
	
	if leastRecentlyUsedBot != nil {
		leastRecentlyUsedBot.mutex.Lock()
		leastRecentlyUsedBot.state = BotStateBusy
		leastRecentlyUsedBot.lastUsed = time.Now()
		leastRecentlyUsedBot.mutex.Unlock()
		return leastRecentlyUsedBot
	}
	
	return nil
}

// ReleaseBot marks a bot as no longer busy
func (bm *BotManager) ReleaseBot(bot *Bot) {
	// Send command to bot manager thread
	resultChan := make(chan botResponse, 1)
	bm.commandChan <- botCommand{
		action:     "releaseBot",
		data:       bot,
		resultChan: resultChan,
	}
	
	// Wait for response (with timeout)
	select {
	case <-resultChan:
		// Response received, bot released
		return
	case <-time.After(2 * time.Second):
		// Timeout, log warning but continue
		LogWarning("Timeout waiting for bot release confirmation")
		return
	}
}

// releaseBotInternal is the internal implementation of ReleaseBot
// that runs in the bot manager thread
func (bm *BotManager) releaseBotInternal(bot *Bot) {
	bot.mutex.Lock()
	defer bot.mutex.Unlock()
	
	if bot.state == BotStateBusy {
		// Check if the bot's GC is ready
		isReady := false
		if bot.client != nil && bot.cs2Handler != nil {
			isReady = bot.cs2Handler.IsReady()
			bot.lastGCCheck = time.Now()
			
			// Log the GC ready state
			if isReady {
				LogDebug("Bot %s: GC is ready when releasing", bot.account.Username)
			} else {
				LogDebug("Bot %s: GC is not ready when releasing", bot.account.Username)
			}
		}
		
		// Update the bot state based on GC readiness
		if isReady {
			LogDebug("Bot %s: Changing state from %s to %s", bot.account.Username, bot.state, BotStateReady)
			bot.state = BotStateReady
		} else {
			LogDebug("Bot %s: Changing state from %s to %s", bot.account.Username, bot.state, BotStateLoggedIn)
			bot.state = BotStateLoggedIn
		}
	}
}

// Shutdown gracefully shuts down all bots
func (bm *BotManager) Shutdown() {
	LogInfo("Shutting down bot manager...")
	
	// Cancel the context to signal all bots to shut down
	bm.cancelFunc()
	
	// Wait for all bots to clean up
	bm.mutex.RLock()
	for _, bot := range bm.bots {
		bot.mutex.Lock()
		if bot.client != nil && bot.cs2Handler != nil {
			bot.cs2Handler.SendGoodbye()
			bot.cs2Handler.Shutdown()
		}
		bot.mutex.Unlock()
	}
	bm.mutex.RUnlock()
	
	// Close channels
	close(bm.commandChan)
	close(bm.responseChan)
	
	LogInfo("Bot manager shutdown complete")
}

// monitorBotHealth periodically checks all bots and reconnects any that are down
// This runs in its own goroutine within the bot manager thread
func (bm *BotManager) monitorBotHealth() {
	ticker := time.NewTicker(BotHealthCheckInterval)
	defer ticker.Stop()
	
	// Map to track last reconnect time for each bot
	lastReconnectTime := make(map[string]time.Time)
	
	LogInfo("Bot health monitoring started")
	
	for {
		select {
		case <-ticker.C:
			// Skip health checks if initialization is not complete
			bm.mutex.RLock()
			initComplete := bm.initComplete
			bm.mutex.RUnlock()
			
			if !initComplete {
				LogDebug("Health monitor: Skipping health check as initialization is not complete")
				continue
			}
			
			bm.mutex.RLock()
			for _, bot := range bm.bots {
				// Skip nil bots
				if bot == nil {
					continue
				}
				
				bot.mutex.Lock()
				username := bot.account.Username
				state := bot.state
				reconnectAttempts := bot.reconnectAttempts
				
				// Check if the bot needs reconnection
				needsReconnect := state == BotStateDisconnected
				
				// Check if the bot is logged in but GC is not ready
				// Only check if the client and cs2Handler are valid
				needsGCReconnect := false
				isGCReady := false
				
				// Proactively check GC readiness for all logged in bots
				if bot.client != nil && bot.cs2Handler != nil && 
					(state == BotStateLoggedIn || state == BotStateReady || state == BotStateBusy) {
					
					// Check if we need to update the GC ready status
					if time.Since(bot.lastGCCheck) > 30*time.Second {
						isGCReady = bot.cs2Handler.IsReady()
						bot.lastGCCheck = time.Now()
						
						// Update bot state based on GC readiness
						if isGCReady && state == BotStateLoggedIn {
							LogInfo("Health monitor: Bot %s GC is ready, updating state from %s to %s", 
								username, state, BotStateReady)
							bot.state = BotStateReady
						} else if !isGCReady && state == BotStateReady {
							LogInfo("Health monitor: Bot %s GC is not ready, updating state from %s to %s", 
								username, state, BotStateLoggedIn)
							bot.state = BotStateLoggedIn
						}
						
						// Check if we need to reconnect to GC
						needsGCReconnect = !isGCReady && time.Since(bot.lastGCCheck) > GCConnectionTimeout
					}
				}
				
				bot.mutex.Unlock()
				
				// Check if we've recently tried to reconnect this bot
				now := time.Now()
				lastReconnect, exists := lastReconnectTime[username]
				
				// Only trigger reconnect if:
				// 1. We haven't tried in the last 2 minutes, or
				// 2. This is the first attempt
				shouldTriggerReconnect := !exists || now.Sub(lastReconnect) > 2*time.Minute
				
				if needsReconnect && shouldTriggerReconnect {
					LogWarning("Health monitor: Bot %s is disconnected (attempt %d), adding to reconnect queue", 
						username, reconnectAttempts)
					
					// Record this reconnect attempt
					lastReconnectTime[username] = now
					
					// Add to reconnection queue instead of directly reconnecting
					bm.QueueBotForReconnect(bot)
					
				} else if needsGCReconnect && shouldTriggerReconnect {
					LogWarning("Health monitor: Bot %s is logged in but GC not ready, sending hello", username)
					
					// Record this reconnect attempt
					lastReconnectTime[username] = now
					
					bot.SendHello()
				}
			}
			bm.mutex.RUnlock()
			
		case <-bm.ctx.Done():
			LogInfo("Bot health monitor shutting down")
			return
		}
	}
}

// processReconnectQueue handles the reconnection of bots in a controlled manner
func (bm *BotManager) processReconnectQueue() {
	LogInfo("Bot reconnection queue processor started")
	
	for {
		select {
		case bot := <-bm.reconnectQueue:
			// Process bot reconnection
			if bot == nil {
				continue
			}
			
			// Decrement the queue size
			bm.reconnectMutex.Lock()
			bm.reconnectQueueSize--
			queueSize := bm.reconnectQueueSize
			bm.reconnectMutex.Unlock()
			
			LogInfo("Processing reconnection for bot %s (queue size: %d)", bot.account.Username, queueSize)
			
			// Acquire a worker slot
			bm.workerPool <- struct{}{}
			
			// Reconnect the bot
			success := bot.Reconnect()
			
			// Release the worker slot
			<-bm.workerPool
			
			if success {
				LogInfo("Bot %s reconnected successfully", bot.account.Username)
			} else {
				LogWarning("Bot %s reconnection failed", bot.account.Username)
			}
			
		case <-bm.ctx.Done():
			LogInfo("Bot reconnection queue processor shutting down")
			return
		}
	}
}

// QueueBotForReconnect adds a bot to the reconnection queue
func (bm *BotManager) QueueBotForReconnect(bot *Bot) {
	if bot == nil {
		return
	}
	
	// Check if the bot is already in the queue
	bm.reconnectMutex.Lock()
	defer bm.reconnectMutex.Unlock()
	
	// Add to queue size counter
	bm.reconnectQueueSize++
	queueSize := bm.reconnectQueueSize
	
	// Add to the queue
	go func() {
		LogInfo("Queuing bot %s for reconnection (queue size: %d)", bot.account.Username, queueSize)
		select {
		case bm.reconnectQueue <- bot:
			// Successfully added to queue
		case <-bm.ctx.Done():
			// Bot manager is shutting down
			bm.reconnectMutex.Lock()
			bm.reconnectQueueSize--
			bm.reconnectMutex.Unlock()
		}
	}()
}

// Bot represents a Steam bot with improved state management
type Bot struct {
	account         Account
	client          *goSteam.Client
	cs2Handler      *CS2Handler
	state           string
	lastUsed        time.Time
	lastGCCheck     time.Time
	reconnectDelay  time.Duration
	reconnectAttempts int
	mutex           sync.Mutex
	ctx             context.Context
	cancelFunc      context.CancelFunc
	refreshToken    string // Store the current refresh token
	sessionFile     string // Path to the session file
}

// NewBot creates a new bot instance
func NewBot(account Account, parentCtx context.Context) *Bot {
	ctx, cancel := context.WithCancel(parentCtx)
	
	// Get session directory from env or use default
	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		sessionDir = os.Getenv("SESSION_FOLDER")
		if sessionDir == "" {
			sessionDir = "sessions"
		}
	}
	
	sessionFile := filepath.Join(sessionDir, account.Username+".json")
	
	return &Bot{
		account:         account,
		state:           BotStateDisconnected,
		lastUsed:        time.Now(),
		lastGCCheck:     time.Time{},
		reconnectDelay:  InitialReconnectDelay,
		reconnectAttempts: 0,
		ctx:             ctx,
		cancelFunc:      cancel,
		refreshToken:    account.RefreshToken,
		sessionFile:     sessionFile,
	}
}

// Start initializes and starts the bot
func (b *Bot) Start() {
	LogInfo("Starting bot %s", b.account.Username)
	
	// Try to connect, if it fails, retry with a different proxy
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if b.connect() {
			// Connection successful
			return
		}
		
		// If we've been blacklisted, don't retry
		if isBlacklisted(b.account.Username) {
			LogWarning("Bot %s: Account has been blacklisted, not retrying", b.account.Username)
			return
		}
		
		// If we've reached the maximum number of retries, give up
		if i == maxRetries-1 {
			LogWarning("Bot %s: Failed to connect after %d attempts", b.account.Username, maxRetries)
			return
		}
		
		// Increment the proxy index to try a different proxy
		b.account.ProxyIndex++
		LogInfo("Bot %s: Retrying connection with proxy index %d", b.account.Username, b.account.ProxyIndex)
		
		// Wait a bit before retrying
		time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second)
	}
}

// connect establishes a connection to Steam
func (b *Bot) connect() bool {
	for {
		select {
		case <-b.ctx.Done():
			LogInfo("Bot %s shutting down", b.account.Username)
			return false
		default:
			// Continue with connection attempt
		}
		
		b.mutex.Lock()
		b.state = BotStateConnecting
		
		// Create a new client if needed
		if b.client == nil {
			LogInfo("Bot %s: Creating new Steam client", b.account.Username)
			b.client = goSteam.NewClient()
			
			// Set up proxy if configured
			proxyDialer, err := GetProxyForAccount(b.account.Username, b.account.ProxyIndex)
			if err != nil {
				LogWarning("Bot %s: Failed to set up proxy: %v", b.account.Username, err)
			} else if proxyDialer != nil {
				LogInfo("Bot %s: Setting up proxy with index %d", b.account.Username, b.account.ProxyIndex)
				b.client.SetProxyDialer(&proxyDialer)
			}
			
			b.cs2Handler = NewCS2Handler(b.client)
			b.client.GC.RegisterPacketHandler(b.cs2Handler)
		}
		b.mutex.Unlock()
		
		// Try to load session
		session, err := b.loadSession()
		if err != nil {
			LogError("Bot %s: Error loading session: %v", b.account.Username, err)
		}
		
		// Update refresh token if found in session
		if session != nil && session.RefreshToken != "" {
			b.refreshToken = session.RefreshToken
			LogInfo("Bot %s: Using refresh token from saved session", b.account.Username)
		}
		
		// Connect to Steam
		LogInfo("Bot %s: Connecting to Steam...", b.account.Username)
		
		// Set up a connection timeout
		connectionTimeout := time.After(30 * time.Second)
		connectionSuccess := make(chan bool, 1)
		
		// Connect in a goroutine so we can handle timeouts
		go func() {
			b.client.Connect()
			connectionSuccess <- true
		}()
		
		// Wait for either connection success or timeout
		select {
		case <-connectionSuccess:
			// Connection attempt completed (may be success or failure)
			LogInfo("Bot %s: Connection attempt completed", b.account.Username)
		
			// Process events
			connected := false
			loggedOn := false
			
			// Channel to signal when the bot becomes ready
			readySignal := make(chan bool, 1)
			
			// Create a timer for event timeout
			eventTimeoutTimer := time.NewTimer(60 * time.Second)
			timeoutActive := true
			
			// Create a channel to signal when we should exit the event loop
			exitEventLoop := make(chan bool, 1)
			
			// Start a goroutine to process events
			go func() {
				for {
					// Check if client is still valid before proceeding
					b.mutex.Lock()
					if b.client == nil {
						LogWarning("Bot %s: Client is nil, breaking event loop", b.account.Username)
						b.state = BotStateDisconnected
						b.mutex.Unlock()
						exitEventLoop <- true
						return
					}
					clientCopy := b.client // Make a copy of the client pointer while under mutex lock
					b.mutex.Unlock()
					
					// If timeout is not active, drain the channel if needed and stop the timer
					if !timeoutActive && !eventTimeoutTimer.Stop() {
						select {
						case <-eventTimeoutTimer.C:
						default:
						}
					}
					
					select {
					case event, ok := <-clientCopy.Events():
						// Check if channel is closed
						if !ok {
							LogWarning("Bot %s: Events channel closed", b.account.Username)
							b.mutex.Lock()
							b.state = BotStateDisconnected
							if b.client != nil {
								b.client.Disconnect()
								b.client = nil
							}
							if b.cs2Handler != nil {
								b.cs2Handler.Shutdown()
								b.cs2Handler = nil
							}
							b.mutex.Unlock()
							exitEventLoop <- true
							return
						}
						
						switch event.(type) {
						case *goSteam.ConnectedEvent:
							LogInfo("Bot %s: Connected to Steam", b.account.Username)
							b.mutex.Lock()
							b.state = BotStateConnected
							connected = true
							b.reconnectAttempts = 0
							b.reconnectDelay = InitialReconnectDelay
							b.mutex.Unlock()
							
							// Log on
							b.mutex.Lock()
							b.state = BotStateLoggingIn
							b.mutex.Unlock()
							
							loginDetails := &goSteam.LogOnDetails{
								Username: b.account.Username,
								Password: b.account.Password,
							}
							
							// Use refresh token if available
							if b.refreshToken != "" {
								LogInfo("Bot %s: Attempting login with refresh token", b.account.Username)
								loginDetails.RefreshToken = b.refreshToken
								loginDetails.Password = "" // Clear password when using refresh token
							} else {
								LogInfo("Bot %s: Attempting login with password", b.account.Username)
							}
							
							b.client.Auth.LogOn(loginDetails)
							
							// Reset the event timeout after connecting
							eventTimeoutTimer.Reset(60 * time.Second)
							
						case *goSteam.LoggedOnEvent:
							LogInfo("Bot %s: Logged on to Steam", b.account.Username)
							b.mutex.Lock()
							b.state = BotStateLoggedIn
							loggedOn = true
							b.mutex.Unlock()
							
							// Set game played
							b.client.GC.SetGamesPlayed(CS2AppID)
							
							// Send hello to GC
							b.SendHello()
							
						case *goSteam.LogOnFailedEvent:
							logonFailed := event.(*goSteam.LogOnFailedEvent)
							LogError("Bot %s: Login failed with result: %v", b.account.Username, logonFailed.Result)
							
							// Handle specific login failure cases
							switch logonFailed.Result {
							case steamlang.EResult_InvalidPassword, steamlang.EResult_AccountLogonDenied, steamlang.EResult_IllegalPassword:
								// Invalid credentials - blacklist the account
								LogError("Bot %s: Invalid credentials. Blacklisting account.", b.account.Username)
								if err := blacklistAccount(b.account.Username, BlacklistReasonCredentials); err != nil {
									LogError("Bot %s: Failed to blacklist account: %v", b.account.Username, err)
								}
								
							case steamlang.EResult_AccountLogonDeniedNoMail, steamlang.EResult_AccountLogonDeniedVerifiedEmailRequired,
								steamlang.EResult_AccountLoginDeniedNeedTwoFactor, steamlang.EResult_TwoFactorCodeMismatch,
								steamlang.EResult_TwoFactorActivationCodeMismatch:
								// Steam Guard issues - blacklist the account
								LogError("Bot %s: Steam Guard required or misconfigured. Blacklisting account.", b.account.Username)
								if err := blacklistAccount(b.account.Username, BlacklistReasonSteamGuard); err != nil {
									LogError("Bot %s: Failed to blacklist account: %v", b.account.Username, err)
								}
								
							case steamlang.EResult_Banned, steamlang.EResult_AccountDisabled, steamlang.EResult_AccountLockedDown,
								steamlang.EResult_Suspended, steamlang.EResult_AccountLimitExceeded:
								// Account banned or disabled - blacklist the account
								LogError("Bot %s: Account banned or disabled. Blacklisting account.", b.account.Username)
								if err := blacklistAccount(b.account.Username, BlacklistReasonBanned); err != nil {
									LogError("Bot %s: Failed to blacklist account: %v", b.account.Username, err)
								}
								
							case steamlang.EResult_AccountLoginDeniedThrottle, steamlang.EResult_RateLimitExceeded:
								// Rate limited - don't blacklist, just retry with different proxy
								LogWarning("Bot %s: Login throttled. Will retry with different proxy.", b.account.Username)
								
							default:
								// Other errors - log but don't blacklist
								LogError("Bot %s: Login failed with unhandled result: %v", b.account.Username, logonFailed.Result)
							}
							
							b.mutex.Lock()
							b.state = BotStateDisconnected
							b.mutex.Unlock()
							
							// Signal disconnection
							exitEventLoop <- true
							return
							
						case *goSteam.MachineAuthUpdateEvent:
							// Handle Steam Guard
							LogInfo("Bot %s: Received machine auth update", b.account.Username)
							
						case *goSteam.LoginKeyEvent:
							// Handle login key
							LogInfo("Bot %s: Received login key", b.account.Username)
							
						case *goSteam.DisconnectedEvent:
							LogInfo("Bot %s: Disconnected from Steam", b.account.Username)
							b.mutex.Lock()
							b.state = BotStateDisconnected
							b.mutex.Unlock()
							
							// Signal disconnection
							close(readySignal)
							exitEventLoop <- true
							return
						}
						
						// If we're fully connected and logged in, check if GC is ready
						if connected && loggedOn {
							b.mutex.Lock()
							isReady := b.cs2Handler != nil && b.cs2Handler.IsReady()
							currentState := b.state
							b.mutex.Unlock()
							
							if isReady && currentState != BotStateReady {
								LogInfo("Bot %s: GC ready check after hello - isReady: %v",
									b.account.Username, isReady)
								
								b.mutex.Lock()
								b.state = BotStateReady
								b.mutex.Unlock()
								
								LogInfo("Bot %s: Ready to handle requests (after hello)", b.account.Username)
								
								// Signal that the bot is ready
								select {
								case readySignal <- true:
									LogInfo("Bot %s: Sent ready signal to main event loop", b.account.Username)
								default:
									// Channel already has a value or is closed
								}
								
								// Disable the event timeout
								timeoutActive = false
							}
						}
						
					case <-eventTimeoutTimer.C:
						// Event timeout occurred
						b.mutex.Lock()
						currentState := b.state
						
						// Check if client is nil before proceeding
						if b.client == nil {
							LogWarning("Bot %s: Client is nil during event timeout", b.account.Username)
							b.state = BotStateDisconnected
							b.mutex.Unlock()
							exitEventLoop <- true
							return
						}
						
						isReady := b.cs2Handler != nil && b.cs2Handler.IsReady()
						b.mutex.Unlock()
						
						if currentState == BotStateReady {
							// Bot is ready, no need for timeout
							LogInfo("Bot %s: Will reconnect in %v (attempt %d)",
								b.account.Username, b.reconnectDelay, b.reconnectAttempts)
							timeoutActive = false
							continue
						}
						
						// Check if GC is ready
						LogInfo("Bot %s: GC ready check - isReady: %v, current state: %s",
							b.account.Username, isReady, currentState)
						
						if isReady && currentState != BotStateReady {
							b.mutex.Lock()
							// Double-check that client and handler are still valid
							if b.client == nil || b.cs2Handler == nil {
								LogWarning("Bot %s: Client or CS2Handler became nil during ready check", b.account.Username)
								b.state = BotStateDisconnected
								b.mutex.Unlock()
								exitEventLoop <- true
								return
							}
							
							b.state = BotStateReady
							b.mutex.Unlock()
							
							LogInfo("Bot %s: Ready to handle requests", b.account.Username)
							
							// Disable the event timeout for ready bots
							LogInfo("Bot %s: Event timeout disabled for ready bot", b.account.Username)
							timeoutActive = false
							
							// Signal that the bot is ready
							select {
							case readySignal <- true:
								LogInfo("Bot %s: Sent ready signal to main event loop", b.account.Username)
							default:
								// Channel already has a value or is closed
							}
							
							continue
						} else if currentState == BotStateLoggedIn {
							// Bot is logged in but GC is not ready yet, give it more time
							LogInfo("Bot %s: Resetting event timeout for bot in state: %s",
								b.account.Username, currentState)
							eventTimeoutTimer.Reset(30 * time.Second)
							timeoutActive = true
							continue
						}
						
						// If we reach here, we need to reconnect
						b.mutex.Lock()
						if b.client != nil {
							b.client.Disconnect()
							b.client = nil
						}
						if b.cs2Handler != nil {
							b.cs2Handler.Shutdown()
							b.cs2Handler = nil
						}
						b.state = BotStateDisconnected
						b.mutex.Unlock()
						
						// Break out of the event loop and try reconnecting
						exitEventLoop <- true
						return
						
					case ready := <-readySignal:
						if ready {
							LogInfo("Bot %s: Received ready signal, disabling event timeout", b.account.Username)
							timeoutActive = false
						} else {
							// Bot disconnected or failed
							b.mutex.Lock()
							if b.client != nil {
								b.client.Disconnect()
								b.client = nil
							}
							if b.cs2Handler != nil {
								b.cs2Handler.Shutdown()
								b.cs2Handler = nil
							}
							b.state = BotStateDisconnected
							b.mutex.Unlock()
							
							// Break out of the event loop and try reconnecting
							exitEventLoop <- true
							return
						}
						
					case <-b.ctx.Done():
						// Bot is shutting down
						LogInfo("Bot %s: Event timeout occurred but bot is in state %s, disabling timeout",
							b.account.Username, b.state)
						
						b.mutex.Lock()
						if b.client != nil {
							b.client.Disconnect()
							b.client = nil
						}
						if b.cs2Handler != nil {
							b.cs2Handler.Shutdown()
							b.cs2Handler = nil
						}
						b.mutex.Unlock()
						
						LogInfo("Bot %s: Shutting down during event processing", b.account.Username)
						exitEventLoop <- true
						return
					}
				}
			}()
			
			// Wait for the event loop to exit or for the bot to become ready
			select {
			case <-exitEventLoop:
				// Event loop exited, bot is disconnected or failed
				return false
				
			case <-readySignal:
				// Bot is ready
				LogInfo("Bot %s: Successfully connected and ready", b.account.Username)
				return true
				
			case <-time.After(5 * time.Minute):
				// Overall timeout for the entire connection process
				LogWarning("Bot %s: Overall connection process timed out after 5 minutes", b.account.Username)
				
				b.mutex.Lock()
				// Clean up resources
				if b.client != nil {
					b.client.Disconnect()
					b.client = nil
				}
				if b.cs2Handler != nil {
					b.cs2Handler.Shutdown()
					b.cs2Handler = nil
				}
				b.state = BotStateDisconnected
				b.mutex.Unlock()
				
				return false
			}
			
		case <-connectionTimeout:
			// Connection timed out
			LogWarning("Bot %s: Connection attempt timed out", b.account.Username)
			
			b.mutex.Lock()
			// Clean up resources
			if b.client != nil {
				b.client.Disconnect()
				b.client = nil
			}
			if b.cs2Handler != nil {
				b.cs2Handler.Shutdown()
				b.cs2Handler = nil
			}
			b.state = BotStateDisconnected
			b.mutex.Unlock()
			
			// Wait before retrying
			reconnectDelay := b.calculateBackoff()
			LogInfo("Bot %s: Waiting %v before reconnecting...", b.account.Username, reconnectDelay)
			select {
			case <-time.After(reconnectDelay):
				// Time to retry
				continue
			case <-b.ctx.Done():
				return false
			}
		}
	}
}

// calculateBackoff calculates the backoff delay for reconnection attempts
func (b *Bot) calculateBackoff() time.Duration {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	b.reconnectAttempts++
	if b.reconnectAttempts > 1 {
		b.reconnectDelay = time.Duration(float64(b.reconnectDelay) * ReconnectBackoffFactor)
		if b.reconnectDelay > MaxReconnectDelay {
			b.reconnectDelay = MaxReconnectDelay
		}
	}
	
	return b.reconnectDelay
}

// Reconnect forces a reconnection of the bot
func (b *Bot) Reconnect() bool {
	b.mutex.Lock()
	
	// Check if the account has been blacklisted
	if isBlacklisted(b.account.Username) {
		LogWarning("Bot %s: Account has been blacklisted, not reconnecting", b.account.Username)
		b.mutex.Unlock()
		return false
	}
	
	LogInfo("Bot %s: Forcing reconnect from state: %s", b.account.Username, b.state)
	
	// Clean up resources regardless of current state
	if b.cs2Handler != nil {
		// Try to send goodbye if connected to GC
		if b.cs2Handler.IsReady() {
			LogInfo("Bot %s: Sending goodbye to Game Coordinator before reconnect", b.account.Username)
			b.cs2Handler.SendGoodbye()
		}
		
		LogInfo("Bot %s: Shutting down CS2Handler", b.account.Username)
		b.cs2Handler.Shutdown()
		b.cs2Handler = nil
	}
	
	// Disconnect if connected
	if b.client != nil {
		LogInfo("Bot %s: Disconnecting client", b.account.Username)
		b.client.Disconnect()
		b.client = nil
	}
	
	// Reset state
	b.state = BotStateDisconnected
	b.mutex.Unlock()
	
	// Try to connect with a new client
	// Try to connect, if it fails, retry with a different proxy
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if b.connect() {
			// Connection successful
			return true
		}
		
		// If we've been blacklisted during connection, don't retry
		if isBlacklisted(b.account.Username) {
			LogWarning("Bot %s: Account has been blacklisted during reconnect, not retrying", b.account.Username)
			return false
		}
		
		// If we've reached the maximum number of retries, give up
		if i == maxRetries-1 {
			LogWarning("Bot %s: Failed to reconnect after %d attempts", b.account.Username, maxRetries)
			return false
		}
		
		// Increment the proxy index to try a different proxy
		b.account.ProxyIndex++
		LogInfo("Bot %s: Retrying reconnection with proxy index %d", b.account.Username, b.account.ProxyIndex)
		
		// Wait a bit before retrying
		time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second)
	}
	
	return false
}

// SendHello sends a hello message to the Game Coordinator
func (b *Bot) SendHello() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	// Check if client and cs2Handler are valid
	if b.client == nil {
		LogWarning("Bot %s: Cannot send hello, client is nil", b.account.Username)
		return
	}
	
	if b.cs2Handler == nil {
		LogWarning("Bot %s: Cannot send hello, CS2Handler is nil", b.account.Username)
		return
	}
	
	if b.state == BotStateLoggedIn || b.state == BotStateReady || b.state == BotStateBusy {
		LogInfo("Bot %s: Sending hello to Game Coordinator", b.account.Username)
		b.cs2Handler.SendHello()
		b.lastGCCheck = time.Now()
	} else {
		LogDebug("Bot %s: Not sending hello, bot state is %s", b.account.Username, b.state)
	}
}

// isGCReady checks if the Game Coordinator connection is ready
func (b *Bot) isGCReady() bool {
	if b.cs2Handler == nil {
		LogDebug("Bot %s: CS2Handler is nil, not ready", b.account.Username)
		return false
	}
	
	ready := b.cs2Handler.IsReady()
	b.lastGCCheck = time.Now()
	
	// Log the ready state for debugging
	if ready {
		LogDebug("Bot %s: GC is ready", b.account.Username)
	} else {
		LogDebug("Bot %s: GC is not ready", b.account.Username)
	}
	
	// If GC is ready and bot is in logged in state, update to ready state
	if ready && b.state == BotStateLoggedIn {
		LogInfo("Bot %s: GC is ready, updating state from %s to %s", 
			b.account.Username, b.state, BotStateReady)
		b.state = BotStateReady
	}
	
	return ready
}

// RequestItemInfo requests item information from the Game Coordinator
func (b *Bot) RequestItemInfo(paramA, paramD, paramS, paramM uint64) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	// Check if client and cs2Handler are valid
	if b.client == nil {
		LogWarning("Bot %s: Cannot request item info, client is nil", b.account.Username)
		return
	}
	
	if b.cs2Handler == nil {
		LogWarning("Bot %s: Cannot request item info, CS2Handler is nil", b.account.Username)
		return
	}
	
	b.cs2Handler.RequestItemInfo(paramA, paramD, paramS, paramM)
}

// BlacklistReason represents the reason an account was blacklisted
type BlacklistReason int

const (
	BlacklistReasonUnknown     BlacklistReason = 0
	BlacklistReasonBanned      BlacklistReason = 1
	BlacklistReasonSteamGuard  BlacklistReason = 2
	BlacklistReasonCredentials BlacklistReason = 3
)

// BlacklistedAccount represents an account that has been blacklisted
type BlacklistedAccount struct {
	Username string
	Reason   BlacklistReason
	Time     time.Time
}

var (
	// Global blacklist of accounts
	blacklistedAccounts = make(map[string]BlacklistedAccount)
	blacklistMutex      sync.RWMutex
)

// loadBlacklist loads blacklisted accounts from the blacklist file
func loadBlacklist() error {
	blacklistPath := os.Getenv("BLACKLIST_PATH")
	if blacklistPath == "" {
		blacklistPath = "blacklist.txt"
	}

	// Check if blacklist file exists
	if _, err := os.Stat(blacklistPath); os.IsNotExist(err) {
		LogInfo("Blacklist file does not exist: %s", blacklistPath)
		return nil
	}

	file, err := os.Open(blacklistPath)
	if err != nil {
		return fmt.Errorf("failed to open blacklist file: %v", err)
	}
	defer file.Close()

	blacklistMutex.Lock()
	defer blacklistMutex.Unlock()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 2 {
			username := parts[0]
			reasonStr := parts[1]
			
			var reason BlacklistReason
			switch reasonStr {
			case "banned":
				reason = BlacklistReasonBanned
			case "steamguard":
				reason = BlacklistReasonSteamGuard
			case "credentials":
				reason = BlacklistReasonCredentials
			default:
				reason = BlacklistReasonUnknown
			}
			
			blacklistedAccounts[username] = BlacklistedAccount{
				Username: username,
				Reason:   reason,
				Time:     time.Now(), // We don't have the original time, so use current time
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading blacklist file: %v", err)
	}

	LogInfo("Loaded %d blacklisted accounts", len(blacklistedAccounts))
	return nil
}

// saveBlacklist saves the blacklisted accounts to the blacklist file
func saveBlacklist() error {
	blacklistPath := os.Getenv("BLACKLIST_PATH")
	if blacklistPath == "" {
		blacklistPath = "blacklist.txt"
	}

	file, err := os.Create(blacklistPath)
	if err != nil {
		return fmt.Errorf("failed to create blacklist file: %v", err)
	}
	defer file.Close()

	blacklistMutex.RLock()
	defer blacklistMutex.RUnlock()

	for _, account := range blacklistedAccounts {
		var reasonStr string
		switch account.Reason {
		case BlacklistReasonBanned:
			reasonStr = "banned"
		case BlacklistReasonSteamGuard:
			reasonStr = "steamguard"
		case BlacklistReasonCredentials:
			reasonStr = "credentials"
		default:
			reasonStr = "unknown"
		}
		
		_, err := fmt.Fprintf(file, "%s:%s\n", account.Username, reasonStr)
		if err != nil {
			return fmt.Errorf("error writing to blacklist file: %v", err)
		}
	}

	return nil
}

// blacklistAccount adds an account to the blacklist
func blacklistAccount(username string, reason BlacklistReason) error {
	blacklistMutex.Lock()
	defer blacklistMutex.Unlock()

	blacklistedAccounts[username] = BlacklistedAccount{
		Username: username,
		Reason:   reason,
		Time:     time.Now(),
	}

	LogWarning("Account %s has been blacklisted. Reason: %d", username, reason)
	
	// Save the updated blacklist
	return saveBlacklist()
}

// isBlacklisted checks if an account is blacklisted
func isBlacklisted(username string) bool {
	blacklistMutex.RLock()
	defer blacklistMutex.RUnlock()
	
	_, exists := blacklistedAccounts[username]
	return exists
}

// loadAccounts loads Steam accounts from accounts.txt or the file specified in ACCOUNTS_FILE env var
func loadAccounts() ([]Account, error) {
	// Load blacklist first
	if err := loadBlacklist(); err != nil {
		LogWarning("Failed to load blacklist: %v", err)
	}
	
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
	proxyIndex := 1 // Start from 1 for proxy indices
	
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 4 {
			username := parts[0]
			
			// Skip blacklisted accounts
			if isBlacklisted(username) {
				LogInfo("Skipping blacklisted account: %s", username)
				continue
			}
			
			account := Account{
				Username:     username,
				Password:     parts[1],
				SentryHash:   parts[2],
				SharedSecret: parts[3],
				ProxyIndex:   proxyIndex,
			}
			
			// Try to load refresh token from session file
			sessionDir := os.Getenv("SESSION_DIR")
			if sessionDir == "" {
				sessionDir = os.Getenv("SESSION_FOLDER")
				if sessionDir == "" {
					sessionDir = "sessions"
				}
			}
			
			sessionFile := filepath.Join(sessionDir, account.Username+".json")
			
			// Check if session file exists
			if _, err := os.Stat(sessionFile); err == nil {
				// Read session file
				data, err := os.ReadFile(sessionFile)
				if err == nil {
					var session Session
					if err := json.Unmarshal(data, &session); err == nil {
						// Check if session is not too old (180 days)
						ageInDays := float64(time.Now().UnixNano()/int64(time.Millisecond)-session.Timestamp) / (1000 * 60 * 60 * 24)
						if ageInDays <= 180 && session.RefreshToken != "" {
							account.RefreshToken = session.RefreshToken
							LogInfo("Loaded refresh token for account %s", account.Username)
						}
					}
				}
			}
			
			accounts = append(accounts, account)
			proxyIndex++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	LogInfo("Loaded %d accounts (excluded %d blacklisted accounts)", len(accounts), len(blacklistedAccounts))
	return accounts, nil
}

// Global bot manager instance
var botManager *BotManager

// InitializeBots initializes the bot manager and starts all bots
func InitializeBots() error {
	botManager = NewBotManager()
	return botManager.Initialize()
}

// GetAvailableBot returns a bot that is ready to handle requests
func GetAvailableBot() *Bot {
	if botManager == nil {
		return nil
	}
	return botManager.GetAvailableBot()
}

// ReleaseBot marks a bot as no longer busy
func ReleaseBot(bot *Bot) {
	if botManager != nil && bot != nil {
		botManager.ReleaseBot(bot)
	}
}

// ShutdownBots gracefully shuts down all bots
func ShutdownBots() {
	if botManager != nil {
		botManager.Shutdown()
	}
}

// loadSession loads the session data from disk if it exists
func (b *Bot) loadSession() (*Session, error) {
	// Create sessions directory if it doesn't exist
	sessionDir := filepath.Dir(b.sessionFile)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, err
	}
	
	// Check if session file exists
	if _, err := os.Stat(b.sessionFile); os.IsNotExist(err) {
		LogInfo("Bot %s: No existing session found", b.account.Username)
		return nil, nil
	}
	
	// Read session file
	data, err := os.ReadFile(b.sessionFile)
	if err != nil {
		LogInfo("Bot %s: Error reading session file: %v", b.account.Username, err)
		return nil, err
	}
	
	// Parse session data
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		LogInfo("Bot %s: Error parsing session data: %v", b.account.Username, err)
		return nil, err
	}
	
	// Check if session is too old (180 days)
	ageInDays := float64(time.Now().UnixNano()/int64(time.Millisecond)-session.Timestamp) / (1000 * 60 * 60 * 24)
	if ageInDays > 180 {
		LogInfo("Bot %s: Session is too old (%.2f days), will create new one", b.account.Username, ageInDays)
		return nil, nil
	}
	
	LogInfo("Bot %s: Found existing session data", b.account.Username)
	return &session, nil
}

// saveSession saves the current session data to disk
func (b *Bot) saveSession() error {
	// Don't save if no refresh token
	if b.refreshToken == "" {
		LogInfo("Bot %s: No refresh token available to save", b.account.Username)
		return nil
	}
	
	// Create session data
	session := Session{
		RefreshToken: b.refreshToken,
		Timestamp:    time.Now().UnixNano() / int64(time.Millisecond),
		Username:     b.account.Username,
		HasGuard:     false, // Set based on Steam Guard status if available
	}
	
	// Serialize session data
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		LogInfo("Bot %s: Error serializing session data: %v", b.account.Username, err)
		return err
	}
	
	// Create sessions directory if it doesn't exist
	sessionDir := filepath.Dir(b.sessionFile)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		LogInfo("Bot %s: Error creating sessions directory: %v", b.account.Username, err)
		return err
	}
	
	// Write session file
	if err := os.WriteFile(b.sessionFile, data, 0644); err != nil {
		LogInfo("Bot %s: Error writing session file: %v", b.account.Username, err)
		return err
	}
	
	LogInfo("Bot %s: Session saved successfully", b.account.Username)
	return nil
}

// getProxyForBot returns a proxy string for the given bot
func getProxyForBot(username string, proxyIndex int) string {
	proxyStr := os.Getenv("PROXY_URL")
	if proxyStr == "" {
		return ""
	}
	
	// Replace [session] with username + index
	session := fmt.Sprintf("%s%d", username, proxyIndex)
	proxyStr = strings.ReplaceAll(proxyStr, "[session]", session)
	
	return proxyStr
} 

// handleConnectionFailure handles a failed connection attempt
func (b *Bot) handleConnectionFailure() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	// Clean up resources
	if b.client != nil {
		b.client.Disconnect()
		b.client = nil
	}
	if b.cs2Handler != nil {
		b.cs2Handler.Shutdown()
		b.cs2Handler = nil
	}
	
	// Update state
	b.state = BotStateDisconnected
	b.reconnectAttempts++
	
	// Calculate backoff
	b.reconnectDelay = time.Duration(float64(b.reconnectDelay) * ReconnectBackoffFactor)
	if b.reconnectDelay > MaxReconnectDelay {
		b.reconnectDelay = MaxReconnectDelay
	}
}

// UpdateRefreshToken updates the bot's refresh token and saves the session
func (b *Bot) UpdateRefreshToken(token string) {
	if token == "" {
		return
	}
	
	LogInfo("Bot %s: Updating refresh token", b.account.Username)
	b.refreshToken = token
	
	// Save the session with the new refresh token
	if err := b.saveSession(); err != nil {
		LogError("Bot %s: Failed to save session: %v", b.account.Username, err)
	}
} 