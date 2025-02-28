package main

import (
	"bufio"
	"context"
	"math/rand"
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
	
	// Map to track last reconnect time for each bot
	lastReconnectTime := make(map[string]time.Time)
	
	for {
		select {
		case <-ticker.C:
			bm.mutex.RLock()
			for _, bot := range bm.bots {
				bot.mutex.Lock()
				username := bot.account.Username
				state := bot.state
				reconnectAttempts := bot.reconnectAttempts
				
				// Check if the bot needs reconnection
				needsReconnect := state == BotStateDisconnected
				
				// Check if the bot is logged in but GC is not ready
				needsGCReconnect := (state == BotStateLoggedIn || state == BotStateReady) && 
					!bot.isGCReady() && 
					time.Since(bot.lastGCCheck) > GCConnectionTimeout
				
				bot.mutex.Unlock()
				
				// Check if we've recently tried to reconnect this bot
				now := time.Now()
				lastReconnect, exists := lastReconnectTime[username]
				
				// Only trigger reconnect if:
				// 1. We haven't tried in the last 2 minutes, or
				// 2. This is the first attempt
				shouldTriggerReconnect := !exists || now.Sub(lastReconnect) > 2*time.Minute
				
				if needsReconnect && shouldTriggerReconnect {
					LogWarning("Health monitor: Bot %s is disconnected (attempt %d), triggering reconnect", 
						username, reconnectAttempts)
					
					// Record this reconnect attempt
					lastReconnectTime[username] = now
					
					// Force a complete reconnect with new client
					go func(b *Bot) {
						// Add a small random delay to avoid all bots reconnecting simultaneously
						time.Sleep(time.Duration(rand.Intn(5)) * time.Second)
						b.Reconnect()
					}(bot)
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
			
			eventTimeout := time.After(60 * time.Second)
			eventLoop:
			for {
				select {
				case event := <-b.client.Events():
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
						
						// Reset the event timeout after connecting
						eventTimeout = time.After(60 * time.Second)
						
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
							
							// Check GC readiness after sending hello
							time.Sleep(2 * time.Second)
							b.mutex.Lock()
							if b.state == BotStateLoggedIn {
								isReady := b.cs2Handler.IsReady()
								LogInfo("Bot %s: GC ready check after hello - isReady: %v", 
									b.account.Username, isReady)
								
								if isReady {
									b.state = BotStateReady
									LogInfo("Bot %s: Ready to handle requests (after hello)", b.account.Username)
									// Signal to the main loop that we're ready
									select {
									case readySignal <- true:
										LogInfo("Bot %s: Sent ready signal to main event loop", b.account.Username)
									default:
										// Channel buffer is full, which means the signal was already sent
									}
								}
							}
							b.mutex.Unlock()
						}()
						
						// Reset the event timeout after logging in
						eventTimeout = time.After(60 * time.Second)
						
					case *goSteam.DisconnectedEvent:
						LogWarning("Bot %s: Disconnected from Steam: %v", b.account.Username, e)
						
						b.mutex.Lock()
						// Clean up resources
						if b.cs2Handler != nil {
							b.cs2Handler.Shutdown()
						}
						
						b.state = BotStateDisconnected
						b.mutex.Unlock()
						
						// Calculate backoff for next reconnect
						reconnectDelay := b.calculateBackoff()
						
						LogInfo("Bot %s: Will reconnect in %v (attempt %d)", 
							b.account.Username, reconnectDelay, b.reconnectAttempts)
						
						// Break out of the event loop to trigger reconnect
						break eventLoop
					}
					
					// If we're fully connected and logged in, check if GC is ready
					if connected && loggedOn {
						b.mutex.Lock()
						isReady := b.cs2Handler.IsReady()
						LogInfo("Bot %s: GC ready check - isReady: %v, current state: %s", 
							b.account.Username, isReady, b.state)
						
						if isReady && b.state == BotStateLoggedIn {
							b.state = BotStateReady
							LogInfo("Bot %s: Ready to handle requests", b.account.Username)
							
							// Disable the event timeout when the bot becomes ready
							// We don't need a timeout for a bot that's fully connected and ready
							eventTimeout = nil
							LogInfo("Bot %s: Event timeout disabled for ready bot", b.account.Username)
							
							// Signal that we're ready (in case the goroutine hasn't done it yet)
							select {
							case readySignal <- true:
								LogInfo("Bot %s: Sent ready signal to main event loop", b.account.Username)
							default:
								// Channel buffer is full, which means the signal was already sent
							}
						} else if b.state == BotStateReady || b.state == BotStateBusy {
							// If already in ready or busy state, disable timeout
							if eventTimeout != nil {
								eventTimeout = nil
								LogInfo("Bot %s: Event timeout disabled for bot in state: %s", 
									b.account.Username, b.state)
							}
						}
						b.mutex.Unlock()
					}
					
					// After processing any event, check if we should disable the timeout
					b.mutex.Lock()
					currentState := b.state
					b.mutex.Unlock()
					
					// If the bot is in a stable state, disable the timeout
					if (currentState == BotStateReady || currentState == BotStateBusy) && eventTimeout != nil {
						LogInfo("Bot %s: Disabling event timeout for stable state: %s", b.account.Username, currentState)
						eventTimeout = nil
					}
					
				case <-readySignal:
					// Bot has become ready, disable the timeout
					LogInfo("Bot %s: Received ready signal, disabling event timeout", b.account.Username)
					eventTimeout = nil
					
				case <-eventTimeout:
					// Event processing timed out
					LogWarning("Bot %s: Event processing timed out", b.account.Username)
					
					b.mutex.Lock()
					currentState := b.state
					b.mutex.Unlock()
					
					// Only disconnect if not in a ready or busy state
					if currentState != BotStateReady && currentState != BotStateBusy {
						LogWarning("Bot %s: Disconnecting due to event timeout in state: %s", b.account.Username, currentState)
						
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
						
						// Break out of the event loop to trigger reconnect
						break eventLoop
					} else {
						// If the bot is ready or busy, disable the timeout completely
						LogInfo("Bot %s: Event timeout occurred but bot is in state %s, disabling timeout", 
							b.account.Username, currentState)
						eventTimeout = nil
					}
					
				case <-b.ctx.Done():
					LogInfo("Bot %s: Shutting down during event processing", b.account.Username)
					return
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
				b.cs2Handler = nil
			}
			if b.client != nil {
				b.client = nil
			}
			b.mutex.Unlock()
			
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
			LogInfo("Bot %s: Waiting %v before retrying connection...", b.account.Username, reconnectDelay)
			select {
			case <-time.After(reconnectDelay):
				// Time to retry
				continue
			case <-b.ctx.Done():
				return
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
func (b *Bot) Reconnect() {
	b.mutex.Lock()
	
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
	
	// Reset state and reconnection parameters
	b.state = BotStateDisconnected
	b.reconnectDelay = InitialReconnectDelay
	b.reconnectAttempts = 0
	
	// Create a new client and handler
	b.client = goSteam.NewClient()
	
	// Set up proxy if configured
	proxyDialer, err := GetProxyForAccount(b.account.Username, b.account.ProxyIndex)
	if err != nil {
		LogWarning("Bot %s: Failed to set up proxy for reconnect: %v", b.account.Username, err)
	} else if proxyDialer != nil {
		LogInfo("Bot %s: Setting up proxy with index %d for reconnect", b.account.Username, b.account.ProxyIndex)
		b.client.SetProxyDialer(&proxyDialer)
	}
	
	b.cs2Handler = NewCS2Handler(b.client)
	b.client.GC.RegisterPacketHandler(b.cs2Handler)
	
	// Release the lock before starting connection
	b.mutex.Unlock()
	
	// Start a new connection in a goroutine
	go func() {
		LogInfo("Bot %s: Starting new connection after forced reconnect", b.account.Username)
		// Small delay to ensure previous connection is fully closed
		time.Sleep(2 * time.Second)
		b.client.Connect()
	}()
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
	
	// If GC is ready and bot is in logged in state, update to ready state
	if ready && b.state == BotStateLoggedIn {
		b.state = BotStateReady
		LogInfo("Bot %s: GC is ready, updating state to ready", b.account.Username)
	}
	
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
	proxyIndex := 1 // Start from 1 for proxy indices
	
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 4 {
			account := Account{
				Username:     parts[0],
				Password:     parts[1],
				SentryHash:   parts[2],
				SharedSecret: parts[3],
				ProxyIndex:   proxyIndex,
			}
			accounts = append(accounts, account)
			proxyIndex++
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