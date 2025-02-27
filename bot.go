package main

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"
	"time"

	goSteam "github.com/Philipp15b/go-steam/v3"
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
}

// NewBotManager creates a new bot manager
func NewBotManager() *BotManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &BotManager{
		bots:       make([]*Bot, 0),
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Initialize loads accounts and starts bots
func (bm *BotManager) Initialize() error {
	// Load accounts
	accounts, err := loadAccounts()
	if err != nil {
		return err
	}
	
	LogInfo("Loaded %d accounts", len(accounts))
	
	// Initialize bots
	for _, account := range accounts {
		bot := NewBot(account, bm.ctx)
		bm.mutex.Lock()
		bm.bots = append(bm.bots, bot)
		bm.mutex.Unlock()
		
		// Start the bot
		go bot.Start()
	}
	
	// Start health monitoring
	go bm.monitorBotHealth()
	
	return nil
}

// GetAvailableBot returns a bot that is ready to handle requests
func (bm *BotManager) GetAvailableBot() *Bot {
	bm.mutex.RLock()
	defer bm.mutex.RUnlock()
	
	// First, try to find a bot that is ready and not busy
	for _, bot := range bm.bots {
		bot.mutex.Lock()
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
	bot.mutex.Lock()
	defer bot.mutex.Unlock()
	
	if bot.state == BotStateBusy {
		if bot.isGCReady() {
			bot.state = BotStateReady
		} else {
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
	
	LogInfo("Bot manager shutdown complete")
}

// monitorBotHealth periodically checks all bots and reconnects any that are down
func (bm *BotManager) monitorBotHealth() {
	ticker := time.NewTicker(BotHealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			bm.mutex.RLock()
			for _, bot := range bm.bots {
				bot.mutex.Lock()
				username := bot.account.Username
				state := bot.state
				
				// Check if the bot needs reconnection
				needsReconnect := state == BotStateDisconnected
				
				// Check if the bot is logged in but GC is not ready
				needsGCReconnect := (state == BotStateLoggedIn || state == BotStateReady) && 
					!bot.isGCReady() && 
					time.Since(bot.lastGCCheck) > GCConnectionTimeout
				
				bot.mutex.Unlock()
				
				if needsReconnect {
					LogWarning("Health monitor: Bot %s is disconnected, triggering reconnect", username)
					bot.Reconnect()
				} else if needsGCReconnect {
					LogWarning("Health monitor: Bot %s is logged in but GC not ready, sending hello", username)
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
}

// NewBot creates a new bot instance
func NewBot(account Account, parentCtx context.Context) *Bot {
	ctx, cancel := context.WithCancel(parentCtx)
	return &Bot{
		account:         account,
		state:           BotStateDisconnected,
		lastUsed:        time.Now(),
		lastGCCheck:     time.Time{},
		reconnectDelay:  InitialReconnectDelay,
		reconnectAttempts: 0,
		ctx:             ctx,
		cancelFunc:      cancel,
	}
}

// Start initializes and starts the bot
func (b *Bot) Start() {
	LogInfo("Starting bot %s", b.account.Username)
	b.connect()
}

// connect establishes a connection to Steam
func (b *Bot) connect() {
	for {
		select {
		case <-b.ctx.Done():
			LogInfo("Bot %s shutting down", b.account.Username)
			return
		default:
			// Continue with connection attempt
		}
		
		b.mutex.Lock()
		b.state = BotStateConnecting
		
		// Create a new client if needed
		if b.client == nil {
			b.client = goSteam.NewClient()
			b.cs2Handler = NewCS2Handler(b.client)
			b.client.GC.RegisterPacketHandler(b.cs2Handler)
		}
		b.mutex.Unlock()
		
		LogInfo("Bot %s: Connecting to Steam...", b.account.Username)
		b.client.Connect()
		
		// Process events
		connected := false
		loggedOn := false
		
		for event := range b.client.Events() {
			select {
			case <-b.ctx.Done():
				LogInfo("Bot %s: Shutting down during event processing", b.account.Username)
				return
			default:
				// Continue processing events
			}
			
			switch e := event.(type) {
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
				b.client.Auth.LogOn(loginDetails)
				
			case *goSteam.LoggedOnEvent:
				LogInfo("Bot %s: Logged on to Steam", b.account.Username)
				b.mutex.Lock()
				b.state = BotStateLoggedIn
				loggedOn = true
				b.mutex.Unlock()
				
				// Set game played
				b.client.GC.SetGamesPlayed(CS2AppID)
				
				// Send a hello message after a short delay
				go func() {
					time.Sleep(5 * time.Second)
					b.SendHello()
				}()
				
			case *goSteam.DisconnectedEvent:
				LogWarning("Bot %s: Disconnected from Steam: %v", b.account.Username, e)
				
				b.mutex.Lock()
				// Clean up resources
				if b.cs2Handler != nil {
					b.cs2Handler.Shutdown()
				}
				
				b.state = BotStateDisconnected
				
				// Calculate backoff for next reconnect
				b.reconnectAttempts++
				if b.reconnectAttempts > 1 {
					b.reconnectDelay = time.Duration(float64(b.reconnectDelay) * ReconnectBackoffFactor)
					if b.reconnectDelay > MaxReconnectDelay {
						b.reconnectDelay = MaxReconnectDelay
					}
				}
				
				reconnectDelay := b.reconnectDelay
				reconnectAttempts := b.reconnectAttempts
				b.mutex.Unlock()
				
				LogInfo("Bot %s: Will reconnect in %v (attempt %d)", 
					b.account.Username, reconnectDelay, reconnectAttempts)
				
				// Break out of the event loop to trigger reconnect
				break
			}
			
			// If we're fully connected and logged in, check if GC is ready
			if connected && loggedOn {
				b.mutex.Lock()
				isReady := b.cs2Handler.IsReady()
				if isReady && b.state == BotStateLoggedIn {
					b.state = BotStateReady
					LogInfo("Bot %s: Ready to handle requests", b.account.Username)
				}
				b.mutex.Unlock()
			}
		}
		
		// If we get here, the event loop has ended, which means we're disconnected
		b.mutex.Lock()
		b.state = BotStateDisconnected
		reconnectDelay := b.reconnectDelay
		b.mutex.Unlock()
		
		// Wait before reconnecting
		LogInfo("Bot %s: Waiting %v before reconnecting...", b.account.Username, reconnectDelay)
		select {
		case <-time.After(reconnectDelay):
			// Time to reconnect
		case <-b.ctx.Done():
			LogInfo("Bot %s: Shutting down during reconnect wait", b.account.Username)
			return
		}
		
		// Clean up for next connection attempt
		b.mutex.Lock()
		if b.cs2Handler != nil {
			b.cs2Handler.Shutdown()
		}
		b.client = goSteam.NewClient()
		b.cs2Handler = NewCS2Handler(b.client)
		b.client.GC.RegisterPacketHandler(b.cs2Handler)
		b.mutex.Unlock()
	}
}

// Reconnect forces a reconnection of the bot
func (b *Bot) Reconnect() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.state == BotStateDisconnected {
		// Already disconnected, just reset reconnect parameters
		b.reconnectDelay = InitialReconnectDelay
		b.reconnectAttempts = 0
		return
	}
	
	// Send goodbye if connected to GC
	if b.cs2Handler != nil && b.cs2Handler.IsReady() {
		b.cs2Handler.SendGoodbye()
	}
	
	// Clean up resources
	if b.cs2Handler != nil {
		b.cs2Handler.Shutdown()
	}
	
	// Disconnect if connected
	if b.client != nil && (b.state == BotStateConnected || b.state == BotStateLoggedIn || 
		b.state == BotStateReady || b.state == BotStateBusy) {
		b.client.Disconnect()
	}
	
	// Create a new client
	b.client = goSteam.NewClient()
	b.cs2Handler = NewCS2Handler(b.client)
	b.client.GC.RegisterPacketHandler(b.cs2Handler)
	
	// Reset state
	b.state = BotStateDisconnected
	b.reconnectDelay = InitialReconnectDelay
	b.reconnectAttempts = 0
	
	// The main loop will handle reconnection
}

// SendHello sends a hello message to the Game Coordinator
func (b *Bot) SendHello() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.state == BotStateLoggedIn || b.state == BotStateReady || b.state == BotStateBusy {
		if b.cs2Handler != nil {
			LogInfo("Bot %s: Sending hello to Game Coordinator", b.account.Username)
			b.cs2Handler.SendHello()
			b.lastGCCheck = time.Now()
		}
	}
}

// isGCReady checks if the Game Coordinator connection is ready
func (b *Bot) isGCReady() bool {
	if b.cs2Handler == nil {
		return false
	}
	
	ready := b.cs2Handler.IsReady()
	b.lastGCCheck = time.Now()
	return ready
}

// RequestItemInfo requests item information from the Game Coordinator
func (b *Bot) RequestItemInfo(paramA, paramD, paramS, paramM uint64) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.cs2Handler != nil {
		b.cs2Handler.RequestItemInfo(paramA, paramD, paramS, paramM)
	}
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