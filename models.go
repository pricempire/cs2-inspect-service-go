package main

import (
	"time"
)

// Constants for CS2 Game Coordinator messages
const (
	CS2AppID            = 730
	ClientRequestTimeout = 5 * time.Second 
)

// StickerInfo represents information about a sticker
type StickerInfo struct {
	Slot      uint32  `json:"slot,omitempty"`
	ID        uint32  `json:"id"`
	Wear      float32 `json:"wear"`
	Scale     float32 `json:"scale"`
	Rotation  float32 `json:"rotation"`
	TintId    uint32  `json:"tint_id,omitempty"`
	OffsetX   float32 `json:"offset_x,omitempty"`
	OffsetY   float32 `json:"offset_y,omitempty"`
	OffsetZ   float32 `json:"offset_z,omitempty"`
	Pattern   uint32  `json:"pattern,omitempty"`
	Name      string  `json:"name,omitempty"`
}

// ItemInfo represents detailed information about an item
type ItemInfo struct {
	AccountId         uint32        `json:"account_id,omitempty"`
	ItemId            uint64        `json:"item_id,omitempty"`
	DefIndex          uint32        `json:"defindex"`
	PaintIndex        uint32        `json:"paintindex"`
	Rarity            uint32        `json:"rarity"`
	Quality           uint32        `json:"quality"`
	PaintWear         float64       `json:"floatvalue"`
	PaintSeed         uint32        `json:"paintseed"`
	KilleaterScoreType uint32       `json:"killeater_score_type,omitempty"`
	KilleaterValue    int32        `json:"killeater_value,omitempty"` // int32 so we can represent -1 as a special value
	CustomName        string        `json:"custom_name,omitempty"`
	Stickers          []StickerInfo `json:"stickers"` // Always include, even if empty
	Inventory         uint32        `json:"inventory,omitempty"`
	Origin            uint32        `json:"origin,omitempty"`
	QuestId           uint32        `json:"quest_id,omitempty"`
	DropReason        uint32        `json:"drop_reason,omitempty"`
	MusicIndex        uint32        `json:"music_index,omitempty"`
	EntIndex          int32         `json:"ent_index,omitempty"`
	PetIndex          uint32        `json:"pet_index,omitempty"`
	Keychains         []StickerInfo `json:"keychains"` // Always include, even if empty
	IsSouvenir        bool          `json:"souvenir"`
	IsStatTrak        bool          `json:"stattrak"`
	
	// Additional fields from format service
	MarketHashName    string        `json:"market_hash_name,omitempty"`
	WearName          string        `json:"wear_name,omitempty"`
	Phase             string        `json:"phase,omitempty"`
	Pattern           string        `json:"pattern"`
	Image             string        `json:"image,omitempty"`
	Min               float64       `json:"min"`
	Max               float64       `json:"max"`
	Rank              int           `json:"rank"`
	TotalCount        int           `json:"total_count"`
	Type              string        `json:"type,omitempty"`
}

// InspectResponse represents the response from the inspect request
type InspectResponse struct {
	Success  bool      `json:"success"`
	Data     []byte    `json:"data,omitempty"`
	Error    string    `json:"error,omitempty"`
	ItemInfo *ItemInfo `json:"iteminfo,omitempty"`
	Cached   bool      `json:"cached,omitempty"`
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