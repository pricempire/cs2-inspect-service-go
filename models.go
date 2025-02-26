package main

import (
	"sync"
	"time"

	goSteam "github.com/Philipp15b/go-steam/v3"
)

// Constants for CS2 Game Coordinator messages
const (
	CS2AppID            = 730
	ClientRequestTimeout = 60 * time.Second 
)

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

// Global variables
var (
	bots     []*Bot
	botMutex sync.Mutex
) 