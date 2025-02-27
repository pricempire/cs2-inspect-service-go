package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Schema represents the CS2 item schema from CSFloat
type Schema struct {
	Weapons   map[string]Weapon   `json:"weapons"`
	Stickers  map[string]Sticker  `json:"stickers"`
	Keychains map[string]Keychain `json:"keychains"`
	Agents    map[string]Agent    `json:"agents"`
}

// Weapon represents a weapon in the schema
type Weapon struct {
	Name   string           `json:"name"`
	Paints map[string]Paint `json:"paints"`
}

// Paint represents a weapon paint (skin) in the schema
type Paint struct {
	Name  string  `json:"name"`
	Image string  `json:"image"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// Sticker represents a sticker in the schema
type Sticker struct {
	MarketHashName string `json:"market_hash_name"`
}

// Keychain represents a keychain in the schema
type Keychain struct {
	MarketHashName string `json:"market_hash_name"`
}

// Agent represents an agent in the schema
type Agent struct {
	MarketHashName string `json:"market_hash_name"`
	Image          string `json:"image"`
}

// Phase mapping for Doppler knives
var Phase = map[int16]string{
	418: "Phase 1",
	419: "Phase 2",
	420: "Phase 3",
	421: "Phase 4",
	415: "Ruby",
	416: "Sapphire",
	417: "Black Pearl",
	569: "Phase 1",
	570: "Phase 2",
	571: "Phase 3",
	572: "Phase 4",
	568: "Emerald",
	618: "Phase 2",
	619: "Sapphire",
	617: "Black Pearl",
	852: "Phase 1",
	853: "Phase 2",
	854: "Phase 3",
	855: "Phase 4",
	1119: "Emerald",
	1120: "Phase 1",
	1121: "Phase 2",
	1122: "Phase 3",
	1123: "Phase 4",
}

// PatternNames maps specific pattern seeds to their names for certain skins
var PatternNames = map[string]map[int16]string{
	"AK-47 | Case Hardened": {
		661: "Scar Pattern",
		555: "Honorable Mention",
		760: "Golden Booty",
		// Add more pattern names as needed
	},
	"Karambit | Case Hardened": {
		387: "Blue Gem",
		601: "Hidden Blue Gem",
		// Add more pattern names as needed
	},
	// Add more weapons as needed
}

// CHPatterns stores Case Hardened pattern tiers
var CHPatterns map[string]map[string][]int

// FadePercentages stores fade percentages by pattern seed
var FadePercentages map[string]map[string][]int

// MarbleFadePatterns stores marble fade pattern types
var MarbleFadePatterns map[string]map[string][]int

var (
	schema     *Schema
	schemaLock sync.RWMutex
)

// LoadSchema loads the CS2 item schema from CSFloat
func LoadSchema() error {
	LogInfo("Loading schema from CSFloat...")
	
	// Create a client with a timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// Make the request
	resp, err := client.Get("https://csfloat.com/api/v1/schema")
	if err != nil {
		LogError("Failed to fetch schema: %v", err)
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		LogError("Schema API returned non-OK status: %d", resp.StatusCode)
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		LogError("Failed to read schema response: %v", err)
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	LogInfo("Received schema response with %d bytes", len(body))
	
	// Create a temporary schema to unmarshal into
	tempSchema := &Schema{}
	if err := json.Unmarshal(body, tempSchema); err != nil {
		LogError("Failed to unmarshal schema: %v", err)
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	// Validate the schema
	if tempSchema.Stickers == nil || len(tempSchema.Stickers) == 0 {
		LogError("Schema is invalid: no stickers found")
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	if tempSchema.Keychains == nil || len(tempSchema.Keychains) == 0 {
		LogError("Schema is invalid: no keychains found")
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	if tempSchema.Weapons == nil || len(tempSchema.Weapons) == 0 {
		LogError("Schema is invalid: no weapons found")
		return loadSchemaFromFile() // Try to load from file as fallback
	}
	
	// Schema is valid, update the global schema
	schemaLock.Lock()
	schema = tempSchema
	schemaLock.Unlock()
	
	// Log schema details
	LogInfo("Schema loaded successfully with %d stickers, %d keychains, %d weapons", 
		len(schema.Stickers), len(schema.Keychains), len(schema.Weapons))
	
	// Log a few sticker entries as examples
	count := 0
	for id, sticker := range schema.Stickers {
		if count < 5 {
			LogDebug("Sample sticker: ID=%s, Name=%s", id, sticker.MarketHashName)
			count++
		} else {
			break
		}
	}
	
	// Log a few keychain entries as examples
	count = 0
	for id, keychain := range schema.Keychains {
		if count < 5 {
			LogDebug("Sample keychain: ID=%s, Name=%s", id, keychain.MarketHashName)
			count++
		} else {
			break
		}
	}
	
	// Save schema to a file for backup
	saveSchemaToFile(body)
	
	return nil
}

// loadSchemaFromFile attempts to load the schema from a local file
func loadSchemaFromFile() error {
	LogInfo("Attempting to load schema from local file...")
	
	// Try to load the latest schema file
	latestFile := "static/schema_latest.json"
	if _, err := os.Stat(latestFile); os.IsNotExist(err) {
		// Try to find any schema file in the static directory
		files, err := filepath.Glob("static/schema_*.json")
		if err != nil || len(files) == 0 {
			LogError("No schema files found in static directory")
			return fmt.Errorf("failed to load schema from API and no local files found")
		}
		
		// Sort files by name (which includes timestamp) to get the latest
		sort.Strings(files)
		latestFile = files[len(files)-1]
		LogInfo("Using most recent schema file: %s", latestFile)
	}
	
	// Read the file
	body, err := ioutil.ReadFile(latestFile)
	if err != nil {
		LogError("Failed to read schema file: %v", err)
		return fmt.Errorf("failed to read schema file: %v", err)
	}
	
	LogInfo("Read schema file with %d bytes", len(body))
	
	// Create a temporary schema to unmarshal into
	tempSchema := &Schema{}
	if err := json.Unmarshal(body, tempSchema); err != nil {
		LogError("Failed to unmarshal schema from file: %v", err)
		return fmt.Errorf("failed to unmarshal schema from file: %v", err)
	}
	
	// Validate the schema
	if tempSchema.Stickers == nil || len(tempSchema.Stickers) == 0 {
		LogError("Schema from file is invalid: no stickers found")
		return fmt.Errorf("schema from file is invalid: no stickers found")
	}
	
	if tempSchema.Keychains == nil || len(tempSchema.Keychains) == 0 {
		LogError("Schema from file is invalid: no keychains found")
		return fmt.Errorf("schema from file is invalid: no keychains found")
	}
	
	if tempSchema.Weapons == nil || len(tempSchema.Weapons) == 0 {
		LogError("Schema from file is invalid: no weapons found")
		return fmt.Errorf("schema from file is invalid: no weapons found")
	}
	
	// Schema is valid, update the global schema
	schemaLock.Lock()
	schema = tempSchema
	schemaLock.Unlock()
	
	LogInfo("Schema loaded successfully from file with %d stickers, %d keychains, %d weapons", 
		len(schema.Stickers), len(schema.Keychains), len(schema.Weapons))
	
	return nil
}

// saveSchemaToFile saves the schema to a file for backup
func saveSchemaToFile(data []byte) {
	// Create the directory if it doesn't exist
	if err := os.MkdirAll("static", 0755); err != nil {
		LogError("Failed to create static directory: %v", err)
		return
	}
	
	// Write the schema to a file
	filename := fmt.Sprintf("static/schema_%s.json", time.Now().Format("20060102_150405"))
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		LogError("Failed to write schema to file: %v", err)
		return
	}
	
	LogInfo("Schema saved to file: %s", filename)
	
	// Create a symlink to the latest schema
	latestFile := "static/schema_latest.json"
	if err := os.Remove(latestFile); err != nil && !os.IsNotExist(err) {
		LogError("Failed to remove old schema symlink: %v", err)
	}
	
	if err := os.Symlink(filename, latestFile); err != nil {
		LogError("Failed to create schema symlink: %v", err)
	}
}

// LoadPatternFiles loads pattern data from JSON files
func LoadPatternFiles() error {
	log.Println("Loading pattern files...")
	
	// Load Case Hardened patterns
	chData, err := loadJSONFile("static/ch-patterns.json")
	if err != nil {
		return fmt.Errorf("failed to load CH patterns: %v", err)
	}
	
	CHPatterns = make(map[string]map[string][]int)
	if err := json.Unmarshal(chData, &CHPatterns); err != nil {
		return fmt.Errorf("failed to unmarshal CH patterns: %v", err)
	}
	
	// Load Fade percentages
	fadeData, err := loadJSONFile("static/fade-percentages.json")
	if err != nil {
		return fmt.Errorf("failed to load Fade percentages: %v", err)
	}
	
	FadePercentages = make(map[string]map[string][]int)
	if err := json.Unmarshal(fadeData, &FadePercentages); err != nil {
		return fmt.Errorf("failed to unmarshal Fade percentages: %v", err)
	}
	
	// Load Marble Fade patterns
	marbleData, err := loadJSONFile("static/marble-fade-patterns.json")
	if err != nil {
		return fmt.Errorf("failed to load Marble Fade patterns: %v", err)
	}
	
	MarbleFadePatterns = make(map[string]map[string][]int)
	if err := json.Unmarshal(marbleData, &MarbleFadePatterns); err != nil {
		return fmt.Errorf("failed to unmarshal Marble Fade patterns: %v", err)
	}
	
	log.Println("Pattern files loaded successfully")
	return nil
}

// loadJSONFile loads a JSON file from the given path
func loadJSONFile(path string) ([]byte, error) {
	// Get the absolute path to the file
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}
	
	// Read the file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	
	return data, nil
}

// GetSchema returns the loaded schema
func GetSchema() *Schema {
	schemaLock.RLock()
	defer schemaLock.RUnlock() 
	
	return schema
}

// GetWearName returns the wear name based on the float value
func GetWearName(wear float64) string {
	if wear < 0.07 {
		return "Factory New"
	} else if wear < 0.15 {
		return "Minimal Wear"
	} else if wear < 0.38 {
		return "Field-Tested"
	} else if wear < 0.45 {
		return "Well-Worn"
	} else {
		return "Battle-Scarred"
	}
}

// GetPhaseName returns the phase name for a Doppler knife
func GetPhaseName(paintIndex int16) string {
	if phase, ok := Phase[paintIndex]; ok {
		return phase
	}
	return ""
}

// GetPatternName returns the pattern name for a specific weapon and seed
func GetPatternName(marketHashName string, paintSeed int16) string {
	// Check hardcoded patterns first
	if patterns, ok := PatternNames[marketHashName]; ok {
		if name, ok := patterns[paintSeed]; ok {
			return name
		}
	}
	
	// Check for Case Hardened patterns
	if strings.Contains(marketHashName, "Case Hardened") {
		// Extract the knife/weapon type
		weaponType := getWeaponTypeFromName(marketHashName)
		if weaponType != "" {
			if patterns, ok := CHPatterns[weaponType]; ok {
				for tier, seeds := range patterns {
					for _, seed := range seeds {
						if int16(seed) == paintSeed {
							return fmt.Sprintf("%s %s", tier, "Blue Gem")
						}
					}
				}
			}
		}
	}
	
	// Check for Marble Fade patterns
	if strings.Contains(marketHashName, "Marble Fade") {
		// Extract the knife/weapon type
		weaponType := getWeaponTypeFromName(marketHashName)
		if weaponType != "" {
			if patterns, ok := MarbleFadePatterns[weaponType]; ok {
				for patternName, seeds := range patterns {
					for _, seed := range seeds {
						if int16(seed) == paintSeed {
							return patternName
						}
					}
				}
			}
		}
	}

	// Check for Fade percentages
	if strings.Contains(marketHashName, "Fade") {
		// Extract the knife/weapon type
		weaponType := getWeaponTypeFromName(marketHashName)
		if weaponType != "" {
			if percentages, ok := FadePercentages[weaponType]; ok {
				for percentage, seeds := range percentages {
					for _, seed := range seeds {
						if int16(seed) == paintSeed {
							return fmt.Sprintf("%s%% Fade", percentage)
						}
					}
				}
			}
		}
	}
	
	return ""
}

// getWeaponTypeFromName extracts the weapon type from a market hash name
func getWeaponTypeFromName(marketHashName string) string {
	// Map of market hash name prefixes to pattern file keys
	nameToKey := map[string]string{
		"★ Karambit":           "karambit",
		"★ M9 Bayonet":         "m9",
		"★ Bayonet":            "bayonet",
		"★ Butterfly Knife":    "butterfly",
		"★ Falchion Knife":     "falchion",
		"★ Flip Knife":         "flip",
		"★ Gut Knife":          "gut",
		"★ Huntsman Knife":     "huntsman",
		"★ Shadow Daggers":     "shadow",
		"★ Bowie Knife":        "bowie",
		"★ Ursus Knife":        "ursus",
		"★ Navaja Knife":       "navaja",
		"★ Stiletto Knife":     "stiletto",
		"★ Talon Knife":        "talon",
		"★ Skeleton Knife":     "skeleton",
		"★ Nomad Knife":        "nomad",
		"★ Survival Knife":     "survival",
		"★ Paracord Knife":     "paracord",
		"★ Classic Knife":      "classic",
		"AK-47":                "ak47",
		"AWP":                  "awp",
		"Desert Eagle":         "deagle",
		"Glock-18":             "glock",
		"M4A1-S":               "m4a1s",
		"M4A4":                 "m4a4",
		"USP-S":                "usp",
	}
	
	for prefix, key := range nameToKey {
		if strings.HasPrefix(marketHashName, prefix) {
			return key
		}
	}
	
	return ""
}

// BuildMarketHashName builds the market hash name for a weapon
func BuildMarketHashName(defIndex int16, paintIndex int16, quality int16, isStatTrak bool, isSouvenir bool, paintWear float64) string {
	s := GetSchema()
	if s == nil {
		return ""
	}
	
	weapon, ok := s.Weapons[fmt.Sprintf("%d", defIndex)]
	if !ok {
		return ""
	}
	
	parts := []string{}
	
	// Add star for knives (quality 3)
	if quality == 3 {
		parts = append(parts, "★")
	}
	
	// Add StatTrak or Souvenir prefix
	if isStatTrak {
		parts = append(parts, "StatTrak™")
	} else if isSouvenir {
		parts = append(parts, "Souvenir")
	}
	
	// Add weapon name
	parts = append(parts, weapon.Name)
	
	// Add paint name if applicable
	if paintIndex > 0 {
		paint, ok := weapon.Paints[fmt.Sprintf("%d", paintIndex)]
		if ok {
			paintName := paint.Name
			
			// Handle Doppler phases
			phaseName := GetPhaseName(paintIndex)
			if phaseName != "" && paintName == "Doppler" {
				parts = append(parts, "| Doppler")
				
				// Add wear name if applicable
				if paintWear > 0 {
					parts = append(parts, fmt.Sprintf("(%s)", GetWearName(paintWear)))
				}
				
				// Add phase
				parts = append(parts, fmt.Sprintf("- %s", phaseName))
				return joinStrings(parts, " ")
			}
			
			// Regular skins
			parts = append(parts, fmt.Sprintf("| %s", paintName))
			
			// Add wear name if applicable
			if paintWear > 0 {
				parts = append(parts, fmt.Sprintf("(%s)", GetWearName(paintWear)))
			}
		}
	}
	
	return joinStrings(parts, " ")
}

// joinStrings joins strings with a separator
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += sep
		}
		result += part
	}
	return result
}

// StartSchemaUpdater starts a goroutine that periodically updates the schema
func StartSchemaUpdater() {
	go func() {
		// Load schema initially
		if err := LoadSchema(); err != nil {
			log.Printf("Failed to load initial schema: %v", err)
		}
		
		// Load pattern files
		if err := LoadPatternFiles(); err != nil {
			log.Printf("Failed to load pattern files: %v", err)
		}
		
		// Update schema every 24 hours
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		
		for range ticker.C {
			if err := LoadSchema(); err != nil {
				log.Printf("Failed to update schema: %v", err)
			} else {
				log.Println("Schema updated successfully")
			}
		}
	}()
} 