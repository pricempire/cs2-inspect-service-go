package main

import (
	"bufio"
	"log"
	"os"
	"strings"
	"time"

	goSteam "github.com/Philipp15b/go-steam/v3"
)

// Constants for reconnection
const (
	ReconnectDelay = 10 * time.Second
	MaxReconnectAttempts = 5
	ReconnectBackoff = 2 // Multiplier for exponential backoff
)

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
	for _, account := range accounts {
		bot := &Bot{
			Account:   account,
			Connected: false,
			LoggedOn:  false,
			Busy:      false,
			LastUsed:  time.Now(),
		}
		bots = append(bots, bot)
		go startBot(bot)
	}
	
	// Start the bot health monitor
	go monitorBotHealth()
}

// startBot starts a Steam bot
func startBot(bot *Bot) {
	reconnectAttempts := 0
	currentDelay := ReconnectDelay
	
	for {
		bot.Mutex.Lock()
		if bot.Client == nil {
			bot.Client = goSteam.NewClient()
			bot.CS2Handler = NewCS2Handler(bot.Client)
			bot.Client.GC.RegisterPacketHandler(bot.CS2Handler)
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
				reconnectAttempts = 0 // Reset reconnect attempts on successful connection
				currentDelay = ReconnectDelay // Reset delay
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
				
				// Clean up resources
				if bot.CS2Handler != nil {
					bot.CS2Handler.Shutdown()
				}
				
				bot.Connected = false
				bot.LoggedOn = false
				if bot.CS2Handler != nil {
					bot.CS2Handler.SetReady(false)
				}
				
				// Increment reconnect attempts
				reconnectAttempts++
				
				// Calculate backoff delay
				if reconnectAttempts > 1 {
					currentDelay = time.Duration(float64(currentDelay) * ReconnectBackoff)
					log.Printf("Bot %s: Reconnect attempt %d, backing off for %v", 
						bot.Account.Username, reconnectAttempts, currentDelay)
				}
				
				bot.Mutex.Unlock()
				
				// Break out of the event loop to trigger reconnect
				break
			}
		}

		log.Printf("Bot %s: Event loop ended, reconnecting in %v (attempt %d/%d)...", 
			bot.Account.Username, currentDelay, reconnectAttempts, MaxReconnectAttempts)
		
		// If we've exceeded max reconnect attempts, wait longer
		if reconnectAttempts >= MaxReconnectAttempts {
			log.Printf("Bot %s: Max reconnect attempts reached, waiting for 5 minutes before trying again", 
				bot.Account.Username)
			time.Sleep(5 * time.Minute)
			reconnectAttempts = 0
			currentDelay = ReconnectDelay
		} else {
			time.Sleep(currentDelay)
		}
		
		// Create a new client for the next attempt
		bot.Mutex.Lock()
		if bot.CS2Handler != nil {
			bot.CS2Handler.Shutdown()
		}
		bot.Client = goSteam.NewClient()
		bot.CS2Handler = NewCS2Handler(bot.Client)
		bot.Client.GC.RegisterPacketHandler(bot.CS2Handler)
		log.Printf("Bot %s: Created new client for reconnection attempt", bot.Account.Username)
		bot.Mutex.Unlock()
	}
}

// monitorBotHealth periodically checks all bots and reconnects any that are down
func monitorBotHealth() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		<-ticker.C
		
		botMutex.Lock()
		for _, bot := range bots {
			bot.Mutex.Lock()
			
			// Check if the bot is disconnected
			if !bot.Connected || !bot.LoggedOn {
				username := bot.Account.Username
				bot.Mutex.Unlock()
				
				log.Printf("Bot health monitor: %s is disconnected, attempting to reconnect", username)
				
				// Create a new client for the bot
				go reconnectBot(bot)
			} else {
				bot.Mutex.Unlock()
			}
		}
		botMutex.Unlock()
	}
}

// reconnectBot attempts to reconnect a disconnected bot
func reconnectBot(bot *Bot) {
	bot.Mutex.Lock()
	
	// Only attempt to reconnect if the bot is still disconnected
	if !bot.Connected || !bot.LoggedOn {
		// Clean up existing resources
		if bot.CS2Handler != nil {
			bot.CS2Handler.Shutdown()
		}
		
		// Create a new client
		bot.Client = goSteam.NewClient()
		bot.CS2Handler = NewCS2Handler(bot.Client)
		bot.Client.GC.RegisterPacketHandler(bot.CS2Handler)
		
		log.Printf("Bot health monitor: Created new client for %s", bot.Account.Username)
	}
	
	bot.Mutex.Unlock()
	
	// The bot's startBot goroutine will handle the actual reconnection
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