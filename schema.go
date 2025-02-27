package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
	
	schemaLock.Lock()
	defer schemaLock.Unlock()
	
	schema = &Schema{}
	if err := json.Unmarshal(body, schema); err != nil {
		return fmt.Errorf("failed to unmarshal schema: %v", err)
	}
	
	log.Println("Schema loaded successfully")
	return nil
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
	if patterns, ok := PatternNames[marketHashName]; ok {
		if name, ok := patterns[paintSeed]; ok {
			return name
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