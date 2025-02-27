package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// HistoryType represents the type of history event
type HistoryType int

// History types
const (
	HistoryTypeUnknown        HistoryType = 99
	HistoryTypeTrade          HistoryType = 1
	HistoryTypeTradeBOT       HistoryType = 2
	HistoryTypeTradeCancelled HistoryType = 3
	HistoryTypeMarketListing  HistoryType = 4
	HistoryTypeMarketBuy      HistoryType = 5
	HistoryTypeMarketRelisting HistoryType = 6
	HistoryTypeMarketCancelled HistoryType = 7
	HistoryTypeStickerApply   HistoryType = 8
	HistoryTypeStickerRemove  HistoryType = 9
	HistoryTypeStickerChange  HistoryType = 10
	HistoryTypeStickerScrape  HistoryType = 11
	HistoryTypeUnboxed        HistoryType = 12
	HistoryTypeCrafted        HistoryType = 13
	HistoryTypeTradedUp       HistoryType = 14
	HistoryTypePurchasedIngame HistoryType = 15
	HistoryTypeDropped        HistoryType = 16
	HistoryTypeNametagAdded   HistoryType = 17
	HistoryTypeNametagRemoved HistoryType = 18
	HistoryTypeKeychainAdded  HistoryType = 19
	HistoryTypeKeychainRemoved HistoryType = 20
	HistoryTypeKeychainChanged HistoryType = 21
)

// History represents the database model for item history
type History struct {
	ID            int64       `json:"id"`
	UniqueID      string      `json:"uniqueId"`
	AssetID       int64       `json:"assetId"`
	PrevAssetID   int64       `json:"prevAssetId,omitempty"`
	Owner         string      `json:"owner"`
	PrevOwner     string      `json:"prevOwner,omitempty"`
	D             string      `json:"d"`
	Stickers      []byte      `json:"stickers,omitempty"`    // JSON data
	Keychains     []byte      `json:"keychains,omitempty"`   // JSON data
	PrevStickers  []byte      `json:"prevStickers,omitempty"` // JSON data
	PrevKeychains []byte      `json:"prevKeychains,omitempty"` // JSON data
	Type          HistoryType `json:"type"`
	CreatedAt     time.Time   `json:"createdAt"`
}

// Asset represents the database model for a CS2 item
type Asset struct {
	UniqueID         string    `json:"uniqueId"`
	AssetID          int64     `json:"assetId"`
	Ms               int64     `json:"ms"`
	D                string    `json:"d"`
	PaintSeed        int16     `json:"paintSeed,omitempty"`
	PaintIndex       int16     `json:"paintIndex,omitempty"`
	PaintWear        float64   `json:"paintWear,omitempty"`
	Quality          int16     `json:"quality,omitempty"`
	CustomName       string    `json:"customName,omitempty"`
	DefIndex         int16     `json:"defIndex,omitempty"`
	Origin           int16     `json:"origin,omitempty"`
	Rarity           int16     `json:"rarity,omitempty"`
	QuestID          int16     `json:"questId,omitempty"`
	Reason           int16     `json:"reason,omitempty"`
	MusicIndex       int16     `json:"musicIndex,omitempty"`
	EntIndex         int16     `json:"entIndex,omitempty"`
	IsStattrak       bool      `json:"isStattrak"`
	IsSouvenir       bool      `json:"isSouvenir"`
	Stickers         []byte    `json:"stickers,omitempty"`    // JSON data
	Keychains        []byte    `json:"keychains,omitempty"`   // JSON data
	KilleaterScoreType int16   `json:"killeaterScoreType,omitempty"`
	KilleaterValue   int32     `json:"killeaterValue,omitempty"`
	PetIndex         int16     `json:"petIndex,omitempty"`
	Inventory        int64     `json:"inventory"`
	DropReason       int16     `json:"dropReason,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

var db *sql.DB

// InitDB initializes the database connection
func InitDB() error {
	// Get database connection details from environment variables
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		return fmt.Errorf("DB_PASSWORD environment variable is required")
	}
	
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "cs2_inspect"
	}
	
	// Create connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	
	// Connect to database
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	
	// Test connection
	err = db.Ping()
	if err != nil {
		return err
	}
	
	log.Println("Successfully connected to PostgreSQL database")
	return nil
}

// FindAssetByParams looks up an asset in the database by its parameters
func FindAssetByParams(assetId int64, d string, ms int64) (*Asset, error) {
	query := `
		SELECT 
			unique_id, asset_id, ms, d, paint_seed, paint_index, paint_wear, 
			quality, custom_name, def_index, origin, rarity, quest_id, reason, 
			music_index, ent_index, is_stattrak, is_souvenir, stickers, keychains,
			killeater_score_type, killeater_value, pet_index, inventory, drop_reason,
			created_at, updated_at
		FROM asset 
		WHERE asset_id = $1 AND d = $2 AND ms = $3
		LIMIT 1
	`
	
	var asset Asset
	err := db.QueryRow(query, assetId, d, ms).Scan(
		&asset.UniqueID, &asset.AssetID, &asset.Ms, &asset.D, &asset.PaintSeed, 
		&asset.PaintIndex, &asset.PaintWear, &asset.Quality, &asset.CustomName, 
		&asset.DefIndex, &asset.Origin, &asset.Rarity, &asset.QuestID, &asset.Reason,
		&asset.MusicIndex, &asset.EntIndex, &asset.IsStattrak, &asset.IsSouvenir, 
		&asset.Stickers, &asset.Keychains, &asset.KilleaterScoreType, &asset.KilleaterValue,
		&asset.PetIndex, &asset.Inventory, &asset.DropReason, &asset.CreatedAt, &asset.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // No asset found
	}
	
	if err != nil {
		return nil, err
	}
	
	return &asset, nil
}

// SaveAsset saves an asset to the database
func SaveAsset(asset *Asset) error {
	// Set timestamps
	now := time.Now()
	asset.CreatedAt = now
	asset.UpdatedAt = now
	
	// Ensure stickers and keychains are valid JSON arrays if they're empty
	if len(asset.Stickers) == 0 {
		asset.Stickers = []byte("[]")
	}
	
	if len(asset.Keychains) == 0 {
		asset.Keychains = []byte("[]")
	}
	
	query := `
		INSERT INTO asset (
			unique_id, asset_id, ms, d, paint_seed, paint_index, paint_wear, 
			quality, custom_name, def_index, origin, rarity, quest_id, reason, 
			music_index, ent_index, is_stattrak, is_souvenir, stickers, keychains,
			killeater_score_type, killeater_value, pet_index, inventory, drop_reason,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, 
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
		)
		ON CONFLICT (asset_id, ms, d, stickers) DO UPDATE SET
			unique_id = EXCLUDED.unique_id,
			paint_seed = EXCLUDED.paint_seed,
			paint_index = EXCLUDED.paint_index,
			paint_wear = EXCLUDED.paint_wear,
			quality = EXCLUDED.quality,
			custom_name = EXCLUDED.custom_name,
			def_index = EXCLUDED.def_index,
			origin = EXCLUDED.origin,
			rarity = EXCLUDED.rarity,
			quest_id = EXCLUDED.quest_id,
			reason = EXCLUDED.reason,
			music_index = EXCLUDED.music_index,
			ent_index = EXCLUDED.ent_index,
			is_stattrak = EXCLUDED.is_stattrak,
			is_souvenir = EXCLUDED.is_souvenir,
			stickers = EXCLUDED.stickers,
			keychains = EXCLUDED.keychains,
			killeater_score_type = EXCLUDED.killeater_score_type,
			killeater_value = EXCLUDED.killeater_value,
			pet_index = EXCLUDED.pet_index,
			inventory = EXCLUDED.inventory,
			drop_reason = EXCLUDED.drop_reason,
			updated_at = EXCLUDED.updated_at
	`
	
	_, err := db.Exec(
		query,
		asset.UniqueID, asset.AssetID, asset.Ms, asset.D, asset.PaintSeed, 
		asset.PaintIndex, asset.PaintWear, asset.Quality, asset.CustomName, 
		asset.DefIndex, asset.Origin, asset.Rarity, asset.QuestID, asset.Reason,
		asset.MusicIndex, asset.EntIndex, asset.IsStattrak, asset.IsSouvenir, 
		asset.Stickers, asset.Keychains, asset.KilleaterScoreType, asset.KilleaterValue,
		asset.PetIndex, asset.Inventory, asset.DropReason, asset.CreatedAt, asset.UpdatedAt,
	)
	
	return err
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// FindAssetByUniqueID looks up an asset in the database by its unique ID
func FindAssetByUniqueID(uniqueID string) (*Asset, error) {
	query := `
		SELECT 
			unique_id, asset_id, ms, d, paint_seed, paint_index, paint_wear, 
			quality, custom_name, def_index, origin, rarity, quest_id, reason, 
			music_index, ent_index, is_stattrak, is_souvenir, stickers, keychains,
			killeater_score_type, killeater_value, pet_index, inventory, drop_reason,
			created_at, updated_at
		FROM asset 
		WHERE unique_id = $1
		LIMIT 1
	`
	
	var asset Asset
	err := db.QueryRow(query, uniqueID).Scan(
		&asset.UniqueID, &asset.AssetID, &asset.Ms, &asset.D, &asset.PaintSeed, 
		&asset.PaintIndex, &asset.PaintWear, &asset.Quality, &asset.CustomName, 
		&asset.DefIndex, &asset.Origin, &asset.Rarity, &asset.QuestID, &asset.Reason,
		&asset.MusicIndex, &asset.EntIndex, &asset.IsStattrak, &asset.IsSouvenir, 
		&asset.Stickers, &asset.Keychains, &asset.KilleaterScoreType, &asset.KilleaterValue,
		&asset.PetIndex, &asset.Inventory, &asset.DropReason, &asset.CreatedAt, &asset.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // No asset found
	}
	
	if err != nil {
		return nil, err
	}
	
	return &asset, nil
}

// FindHistoryByAssetID looks up history records for an asset
func FindHistoryByAssetID(assetID int64) (*History, error) {
	query := `
		SELECT 
			id, unique_id, asset_id, prev_asset_id, owner, prev_owner, d,
			stickers, keychains, prev_stickers, prev_keychains, type,
			created_at
		FROM history 
		WHERE asset_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	
	var history History
	err := db.QueryRow(query, assetID).Scan(
		&history.ID, &history.UniqueID, &history.AssetID, &history.PrevAssetID,
		&history.Owner, &history.PrevOwner, &history.D, &history.Stickers, 
		&history.Keychains, &history.PrevStickers, &history.PrevKeychains,
		&history.Type, &history.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // No history found
	}
	
	if err != nil {
		return nil, err
	}
	
	return &history, nil
}

// SaveHistory saves a history record to the database
func SaveHistory(history *History) error {
	// Set timestamp
	history.CreatedAt = time.Now()
	
	// Ensure JSON fields are valid JSON arrays if they're empty
	if len(history.Stickers) == 0 {
		history.Stickers = []byte("[]")
	}
	
	if len(history.Keychains) == 0 {
		history.Keychains = []byte("[]")
	}
	
	if len(history.PrevStickers) == 0 {
		history.PrevStickers = []byte("[]")
	}
	
	if len(history.PrevKeychains) == 0 {
		history.PrevKeychains = []byte("[]")
	}
	
	// First check if a record with this unique_id and asset_id already exists
	checkQuery := `
		SELECT id FROM history 
		WHERE unique_id = $1 AND asset_id = $2
		LIMIT 1
	`
	
	var existingId int64
	err := db.QueryRow(checkQuery, history.UniqueID, history.AssetID).Scan(&existingId)
	if err == nil {
		// Record already exists, just return the existing ID
		history.ID = existingId
		log.Printf("History record already exists with ID %d", existingId)
		return nil
	} else if err != sql.ErrNoRows {
		// An actual error occurred
		return fmt.Errorf("error checking for existing history: %v", err)
	}
	
	// No existing record, proceed with insert
	query := `
		INSERT INTO history (
			unique_id, asset_id, prev_asset_id, owner, prev_owner, d,
			stickers, keychains, prev_stickers, prev_keychains, type,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
		RETURNING id
	`

	log.Println("Saving history: ", history.UniqueID, history.AssetID, history.PrevAssetID, history.Owner, history.PrevOwner, history.D, history.Stickers, history.Keychains, history.PrevStickers, history.PrevKeychains, history.Type, history.CreatedAt)
	
	return db.QueryRow(
		query,
		history.UniqueID, history.AssetID, history.PrevAssetID, history.Owner, 
		history.PrevOwner, history.D, history.Stickers, history.Keychains,
		history.PrevStickers, history.PrevKeychains, history.Type,
		history.CreatedAt,
	).Scan(&history.ID)
}

// FindHistoryByUniqueID looks up history records for an item by its unique ID
func FindHistoryByUniqueID(uniqueID string) ([]*History, error) {
	query := `
		SELECT 
			id, unique_id, asset_id, prev_asset_id, owner, prev_owner, d,
			stickers, keychains, prev_stickers, prev_keychains, type,
			created_at
		FROM history 
		WHERE unique_id = $1
		ORDER BY created_at DESC
	`
	
	rows, err := db.Query(query, uniqueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var histories []*History
	for rows.Next() {
		var history History
		err := rows.Scan(
			&history.ID, &history.UniqueID, &history.AssetID, &history.PrevAssetID,
			&history.Owner, &history.PrevOwner, &history.D, &history.Stickers, 
			&history.Keychains, &history.PrevStickers, &history.PrevKeychains,
			&history.Type, &history.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		histories = append(histories, &history)
	}
	
	if err = rows.Err(); err != nil {
		return nil, err
	}
	
	return histories, nil
} 