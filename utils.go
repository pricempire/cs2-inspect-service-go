package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
)

// parseInspectLink parses a CS:GO inspect link
func parseInspectLink(link string) (paramA, paramD uint64, owner uint64, err error) {
	r := regexp.MustCompile(`S(\d+)A(\d+)D(\d+)`)
	matches := r.FindStringSubmatch(link)
	if len(matches) != 4 {
		return 0, 0, 0, fmt.Errorf("invalid inspect link format")
	}

	ownerTemp, _ := strconv.ParseUint(matches[1], 10, 64)
	owner = ownerTemp
	paramA, _ = strconv.ParseUint(matches[2], 10, 64)
	paramD, _ = strconv.ParseUint(matches[3], 10, 64)

	return paramA, paramD, owner, nil
}

// sendJSONResponse sends a JSON response
func sendJSONResponse(w http.ResponseWriter, response InspectResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
} 