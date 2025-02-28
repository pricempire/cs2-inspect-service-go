package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/proxy"
)

var (
	proxyMutex sync.RWMutex
	proxyCache = make(map[string]proxy.Dialer)
)

// GetProxyForAccount returns a proxy dialer for the given account
func GetProxyForAccount(username string, index int) (proxy.Dialer, error) {
	// Check if we have a cached proxy for this username
	proxyMutex.RLock()
	if dialer, ok := proxyCache[username]; ok {
		proxyMutex.RUnlock()
		return dialer, nil
	}
	proxyMutex.RUnlock()

	// Get proxy string from environment
	proxyStr := os.Getenv("PROXY_URL")
	if proxyStr == "" {
		// No proxy configured
		return nil, nil
	}

	// Replace [session] with username + index
	session := fmt.Sprintf("%s%d", username, index)
	proxyStr = strings.ReplaceAll(proxyStr, "[session]", session)

	LogInfo("Using proxy for %s: %s", username, proxyStr)

	// Parse the proxy URL
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %v", err)
	}

	// Create the appropriate dialer based on the proxy type
	var dialer proxy.Dialer
	switch proxyURL.Scheme {
	case "socks5":
		// SOCKS5 proxy
		auth := &proxy.Auth{}
		if proxyURL.User != nil {
			auth.User = proxyURL.User.Username()
			if password, ok := proxyURL.User.Password(); ok {
				auth.Password = password
			}
		}
		
		// If no auth info, set to nil
		if auth.User == "" {
			auth = nil
		}
		 
		dialer, err = proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %v", err)
		}
	case "http", "https":
		// HTTP proxy - not directly supported by golang.org/x/net/proxy
		// For HTTP proxies, we would need to implement a custom dialer
		return nil, fmt.Errorf("HTTP/HTTPS proxies are not supported yet")
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}

	// Cache the dialer
	proxyMutex.Lock()
	proxyCache[username] = dialer
	proxyMutex.Unlock()

	return dialer, nil
}

// ClearProxyCache clears the proxy cache
func ClearProxyCache() {
	proxyMutex.Lock()
	proxyCache = make(map[string]proxy.Dialer)
	proxyMutex.Unlock()
} 