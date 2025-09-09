package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type RequestHistoryEntry struct {
	ID          string           `json:"id"`
	ContextName string           `json:"context_name"`
	URL         string           `json:"url"`
	InputHTML   string           `json:"input_html,omitempty"` // HTML content for HTML-based requests
	Timestamp   time.Time        `json:"timestamp"`
	Duration    time.Duration    `json:"duration_ms"`
	RequestType string           `json:"request_type"` // screenshot, get_html, screenshot_html
	Config      *RequestConfig   `json:"config"`
	Response    *BrowserResponse `json:"response"`
	Error       string           `json:"error,omitempty"`
}

// RequestHistoryManager manages stored browser requests
type RequestHistoryManager struct {
	requests map[string]*RequestHistoryEntry
	mutex    sync.RWMutex
}

// NewRequestHistoryManager creates a new request history manager
func NewRequestHistoryManager() *RequestHistoryManager {
	return &RequestHistoryManager{
		requests: make(map[string]*RequestHistoryEntry),
	}
}

// NewRequestHistoryEntry creates a new request history entry with auto-generated ID
func NewRequestHistoryEntry(contextName, url, inputHTML, requestType string, config *RequestConfig, response *BrowserResponse, startTime time.Time, err error) *RequestHistoryEntry {
	entry := &RequestHistoryEntry{
		ID:          generateRequestID(),
		ContextName: contextName,
		URL:         url,
		InputHTML:   inputHTML,
		Timestamp:   startTime,
		Duration:    time.Since(startTime),
		RequestType: requestType,
		Config:      config,
		Response:    response,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	return entry
}

// StoreRequest stores a complete request with all its data
func (m *RequestHistoryManager) StoreRequest(entry *RequestHistoryEntry) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.requests[entry.ID] = entry
}

// GetRequest retrieves a stored request by ID
func (m *RequestHistoryManager) GetRequest(requestID string) (*RequestHistoryEntry, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	request, exists := m.requests[requestID]
	return request, exists
}

// GetLastRequest retrieves the most recent request for a context
func (m *RequestHistoryManager) GetLastRequest(contextName string, configManager *ContextConfigManager) (*RequestHistoryEntry, bool) {
	context, exists := configManager.GetContext(contextName)
	if !exists || len(context.RequestHistory) == 0 {
		return nil, false
	}

	lastRequestID := context.LastRequestID
	return m.GetRequest(lastRequestID)
}

// CreateRequestResponse creates a response structure for MCP calls
func (m *RequestHistoryManager) CreateRequestResponse(entry *RequestHistoryEntry, includeHTML, includeNetwork, includeConsole bool) map[string]interface{} {
	response := map[string]interface{}{
		"id":           entry.ID,
		"context_name": entry.ContextName,
		"url":          entry.URL,
		"timestamp":    entry.Timestamp,
		"duration":     entry.Duration.Milliseconds(),
		"request_type": entry.RequestType,
	}

	// Include input HTML if present
	if entry.InputHTML != "" {
		response["input_html"] = entry.InputHTML
	}

	if entry.Error != "" {
		response["error"] = entry.Error
		return response
	}

	// Extract information from the BrowserResponse
	if entry.Response != nil {

		// Convert cookies to expected format
		if len(entry.Response.Cookies) > 0 {
			cookies := make([]map[string]interface{}, len(entry.Response.Cookies))
			for i, cookie := range entry.Response.Cookies {
				cookies[i] = map[string]interface{}{
					"name":     cookie.Name,
					"value":    cookie.Value,
					"domain":   cookie.Domain,
					"path":     cookie.Path,
					"expires":  cookie.Expires,
					"httpOnly": cookie.HTTPOnly,
					"secure":   cookie.Secure,
					"sameSite": cookie.SameSite,
				}
			}
			response["set_cookies"] = cookies
		}

		if includeHTML && entry.Response.HTML != nil {
			response["html"] = *entry.Response.HTML
		}

		if includeNetwork {
			response["network_requests"] = entry.Response.NetworkRequests
		}

		if includeConsole {
			response["console_logs"] = entry.Response.ConsoleLogs
		}
	}

	return response
}

func generateRequestID() string {
	timestamp := time.Now().Format("20060102150405")

	// Generate 8 random bytes
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}

	return timestamp + "_" + hex.EncodeToString(randomBytes)
}
