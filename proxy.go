package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var (
	proxyMutex sync.RWMutex
	proxyCache = make(map[string]proxy.Dialer)
)

// HTTPProxyDialer implements proxy.Dialer for HTTP proxies
type HTTPProxyDialer struct {
	proxyURL *url.URL
	forward  proxy.Dialer
	timeout  time.Duration
}

// Dial connects to the address through the HTTP proxy
func (d *HTTPProxyDialer) Dial(network, addr string) (net.Conn, error) {
	LogInfo("HTTP proxy: Connecting to %s via proxy %s", addr, d.proxyURL.Host)
	
	// Create a dialer for the proxy
	proxyDialer := &net.Dialer{
		Timeout:   d.timeout,
		KeepAlive: 30 * time.Second,
	}
	
	// Connect to the proxy server
	LogInfo("HTTP proxy: Establishing connection to proxy server %s", d.proxyURL.Host)
	conn, err := proxyDialer.Dial("tcp", d.proxyURL.Host)
	if err != nil {
		LogError("HTTP proxy: Failed to connect to proxy: %v", err)
		return nil, fmt.Errorf("failed to connect to HTTP proxy: %v", err)
	}
	
	// Create a CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	
	// Add proxy authentication if needed
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		
		LogInfo("HTTP proxy: Adding authentication for user: %s", username)
		auth := fmt.Sprintf("%s:%s", username, password)
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		connectReq += fmt.Sprintf("Proxy-Authorization: %s\r\n", basicAuth)
	}
	
	// Finish the request
	connectReq += "\r\n"
	
	// Send the CONNECT request
	LogInfo("HTTP proxy: Sending CONNECT request to %s", addr)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		LogError("HTTP proxy: Failed to write CONNECT request: %v", err)
		conn.Close()
		return nil, fmt.Errorf("failed to write CONNECT request: %v", err)
	}
	
	// Read the response
	LogInfo("HTTP proxy: Reading CONNECT response")
	buf := make([]byte, 1024)
	deadline := time.Now().Add(d.timeout)
	conn.SetReadDeadline(deadline)
	
	n, err := conn.Read(buf)
	if err != nil {
		LogError("HTTP proxy: Failed to read CONNECT response: %v", err)
		conn.Close()
		return nil, fmt.Errorf("failed to read CONNECT response: %v", err)
	}
	
	// Reset the read deadline
	conn.SetReadDeadline(time.Time{})
	
	// Parse the response
	response := string(buf[:n])
	LogInfo("HTTP proxy: Received response: %s", strings.TrimSpace(response))
	
	// Check if the connection was established
	// HTTP proxies return 200 status code when connection is established
	// The exact format can vary between proxy implementations
	if !strings.Contains(response, "HTTP/1.1 200") && !strings.Contains(response, "HTTP/1.0 200") {
		LogError("HTTP proxy: Connection failed with response: %s", strings.TrimSpace(response))
		conn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", strings.TrimSpace(response))
	}
	
	LogInfo("HTTP proxy: Connection to %s established successfully via proxy", addr)
	
	// Some proxies might include headers in the response that we need to skip
	// Find the end of the headers (double CRLF)
	if headerEnd := strings.Index(response, "\r\n\r\n"); headerEnd > 0 {
		// If there's any data after the headers, we need to handle it
		if headerEnd+4 < len(response) {
			extraData := buf[headerEnd+4:n]
			LogInfo("HTTP proxy: Found %d bytes of data after headers", len(extraData))
			
			// Create a new connection that handles the extra data
			// This is a bit of a hack, but it's necessary for some proxies
			origConn := conn
			conn = &preReadConn{
				Conn:    origConn,
				preRead: extraData,
			}
		}
	}
	
	// Connection established
	return conn, nil
}

// preReadConn is a net.Conn that has some data pre-read
type preReadConn struct {
	net.Conn
	preRead []byte
	preReadDone bool
}

// Read reads data from the connection
func (c *preReadConn) Read(b []byte) (int, error) {
	// If we have pre-read data, return that first
	if !c.preReadDone && len(c.preRead) > 0 {
		n := copy(b, c.preRead)
		if n >= len(c.preRead) {
			c.preReadDone = true
		} else {
			c.preRead = c.preRead[n:]
		}
		return n, nil
	}
	
	// Otherwise, read from the underlying connection
	return c.Conn.Read(b)
}

// NewHTTPProxyDialer creates a new HTTP proxy dialer
func NewHTTPProxyDialer(proxyURL *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
	return &HTTPProxyDialer{
		proxyURL: proxyURL,
		forward:  forward,
		timeout:  30 * time.Second,
	}, nil
}

// GetProxyForAccount returns a proxy dialer for the given account
func GetProxyForAccount(username string, index int) (proxy.Dialer, error) {
	// Check if we have a cached proxy for this username
	proxyMutex.RLock()
	if dialer, ok := proxyCache[username]; ok {
		proxyMutex.RUnlock()
		LogDebug("Using cached proxy for account %s", username)
		return dialer, nil
	}
	proxyMutex.RUnlock()

	// Get proxy string from environment
	proxyStr := os.Getenv("PROXY_URL")
	if proxyStr == "" {
		// No proxy configured
		LogDebug("No proxy configured (PROXY_URL environment variable not set)")
		return nil, nil
	}

	// Replace [session] with username + index
	session := fmt.Sprintf("%s%d", username, index)
	proxyStr = strings.ReplaceAll(proxyStr, "[session]", session)
	LogInfo("Using proxy URL: %s for account %s", proxyStr, username)

	// Parse the proxy URL
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		LogError("Invalid proxy URL: %v", err)
		return nil, fmt.Errorf("invalid proxy URL: %v", err)
	}

	// Create the appropriate dialer based on the proxy type
	var dialer proxy.Dialer
	switch proxyURL.Scheme {
	case "socks5":
		// SOCKS5 proxy
		LogInfo("Creating SOCKS5 proxy dialer for %s", proxyURL.Host)
		auth := &proxy.Auth{}
		if proxyURL.User != nil {
			auth.User = proxyURL.User.Username()
			if password, ok := proxyURL.User.Password(); ok {
				auth.Password = password
			}
			LogInfo("Using SOCKS5 authentication for user: %s", auth.User)
		}
		
		// If no auth info, set to nil
		if auth.User == "" {
			LogInfo("No authentication provided for SOCKS5 proxy")
			auth = nil
		}
		 
		dialer, err = proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
		if err != nil {
			LogError("Failed to create SOCKS5 dialer: %v", err)
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %v", err)
		}
	case "http", "https":
		// HTTP proxy - using our custom implementation
		LogInfo("Creating HTTP proxy dialer for %s", proxyURL.Host)
		dialer, err = NewHTTPProxyDialer(proxyURL, proxy.Direct)
		if err != nil {
			LogError("Failed to create HTTP proxy dialer: %v", err)
			return nil, fmt.Errorf("failed to create HTTP proxy dialer: %v", err)
		}
	default:
		LogError("Unsupported proxy scheme: %s", proxyURL.Scheme)
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}

	// Cache the dialer
	LogInfo("Caching proxy dialer for account %s", username)
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