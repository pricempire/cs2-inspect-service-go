package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// parseInspectLink parses a CS:GO inspect link
func parseInspectLink(link string) (paramA, paramD, paramS, paramM uint64, err error) {
	// Default values
	paramA, paramD, paramS, paramM = 0, 0, 0, 0

	// Clean up the link - remove any URL encoding for spaces
	link = strings.Replace(link, "%20", " ", -1)

	// First, try to match the full Steam inspect link format
	steamRegex := regexp.MustCompile(`steam://rungame/730/\d+/\+csgo_econ_action_preview\s([MS])(\d+)A(\d+)D(\d+)`)
	matches := steamRegex.FindStringSubmatch(link)
	
	if len(matches) == 5 {
		// We have a match for the full Steam inspect link
		paramType := matches[1]
		ownerStr := matches[2]
		paramAStr := matches[3]
		paramDStr := matches[4]
		
		// Parse the numeric values
		var err1, err2, err3 error
		paramA, err1 = strconv.ParseUint(paramAStr, 10, 64)
		paramD, err2 = strconv.ParseUint(paramDStr, 10, 64)
		owner, err3 := strconv.ParseUint(ownerStr, 10, 64)
		
		// Check for parsing errors
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid numeric parameters in inspect link")
		}
		
		// Set the appropriate owner parameter based on the type
		if paramType == "S" {
			paramS = owner
		} else { // paramType == "M"
			paramM = owner
		}
		
		return paramA, paramD, paramS, paramM, nil
	}
	
	// If we didn't match the full format, try to match just the parameters part
	paramsRegex := regexp.MustCompile(`([MS])(\d+)A(\d+)D(\d+)`)
	matches = paramsRegex.FindStringSubmatch(link)
	
	if len(matches) == 5 {
		// We have a match for just the parameters
		paramType := matches[1]
		ownerStr := matches[2]
		paramAStr := matches[3]
		paramDStr := matches[4]
		
		// Parse the numeric values
		var err1, err2, err3 error
		paramA, err1 = strconv.ParseUint(paramAStr, 10, 64)
		paramD, err2 = strconv.ParseUint(paramDStr, 10, 64)
		owner, err3 := strconv.ParseUint(ownerStr, 10, 64)
		
		// Check for parsing errors
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid numeric parameters in inspect link")
		}
		
		// Set the appropriate owner parameter based on the type
		if paramType == "S" {
			paramS = owner
		} else { // paramType == "M"
			paramM = owner
		}
		
		return paramA, paramD, paramS, paramM, nil
	}
	
	// If we get here, we couldn't parse the link
	return 0, 0, 0, 0, fmt.Errorf("invalid inspect link format")
}

// sendJSONResponse sends a JSON response to the client
func sendJSONResponse(w http.ResponseWriter, response interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
} 