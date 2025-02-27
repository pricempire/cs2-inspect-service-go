package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	log.Println("Loading schema from CSFloat...")
	
	resp, err := http.Get("https://csfloat.com/api/v1/schema")
	if err != nil {
		return fmt.Errorf("failed to fetch schema: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read schema response: %v", err)
	}
	
	LogInfo("Received schema response with %d bytes", len(body))
	
	schemaLock.Lock()
	defer schemaLock.Unlock()
	
	schema = &Schema{}
	if err := json.Unmarshal(body, schema); err != nil {
		return fmt.Errorf("failed to unmarshal schema: %v", err)
	}
	
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
	
	return nil
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
	
	// Debug log to check if schema is loaded
	if schema == nil {
		LogError("Schema is nil, make sure it's loaded properly")
	} else {
		LogDebug("Schema loaded with %d stickers, %d keychains, %d weapons", 
			len(schema.Stickers), len(schema.Keychains), len(schema.Weapons))
	}
	
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