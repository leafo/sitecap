package main

import (
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// StoredRequest contains comprehensive information about a browser request
type StoredRequest struct {
	ID              string                      `json:"id"`
	ContextName     string                      `json:"context_name"`
	URL             string                      `json:"url"`
	Timestamp       time.Time                   `json:"timestamp"`
	StatusCode      int                         `json:"status_code"`
	ResponseHeaders map[string][]string         `json:"response_headers"`
	SetCookies      []*proto.NetworkCookieParam `json:"set_cookies"`
	HTML            string                      `json:"html,omitempty"`
	NetworkRequests []NetworkRequestInfo        `json:"network_requests,omitempty"`
	ConsoleLogs     []ConsoleMessage            `json:"console_logs,omitempty"`
	Screenshot      []byte                      `json:"screenshot,omitempty"`
	Error           string                      `json:"error,omitempty"`
	Duration        time.Duration               `json:"duration"`
	RequestType     string                      `json:"request_type"` // screenshot, get_html, etc.
}

// NetworkRequestInfo contains information about individual network requests
type NetworkRequestInfo struct {
	URL             string              `json:"url"`
	Method          string              `json:"method"`
	StatusCode      int                 `json:"status_code"`
	RequestHeaders  map[string]string   `json:"request_headers"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	ResponseBody    string              `json:"response_body,omitempty"`
	Duration        time.Duration       `json:"duration"`
	Timestamp       time.Time           `json:"timestamp"`
	Failed          bool                `json:"failed"`
	ErrorText       string              `json:"error_text,omitempty"`
}

// ConsoleMessage represents a console log message
type ConsoleMessage struct {
	Level      string    `json:"level"` // log, warn, error, info, debug
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source,omitempty"`
	Line       int       `json:"line,omitempty"`
	Column     int       `json:"column,omitempty"`
	StackTrace string    `json:"stack_trace,omitempty"`
}

// RequestHistoryManager manages stored browser requests
type RequestHistoryManager struct {
	requests map[string]*StoredRequest
	mutex    sync.RWMutex
}

// NewRequestHistoryManager creates a new request history manager
func NewRequestHistoryManager() *RequestHistoryManager {
	return &RequestHistoryManager{
		requests: make(map[string]*StoredRequest),
	}
}

// StoreRequest stores a complete request with all its data
func (m *RequestHistoryManager) StoreRequest(request *StoredRequest) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.requests[request.ID] = request
}

// GetRequest retrieves a stored request by ID
func (m *RequestHistoryManager) GetRequest(requestID string) (*StoredRequest, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	request, exists := m.requests[requestID]
	return request, exists
}

// GetLastRequest retrieves the most recent request for a context
func (m *RequestHistoryManager) GetLastRequest(contextName string, configManager *ContextConfigManager) (*StoredRequest, bool) {
	context, exists := configManager.GetContext(contextName)
	if !exists || len(context.RequestHistory) == 0 {
		return nil, false
	}

	lastRequestID := context.LastRequestID
	return m.GetRequest(lastRequestID)
}

// CreateRequestResponse creates a response structure for MCP calls
func (m *RequestHistoryManager) CreateRequestResponse(request *StoredRequest, includeHTML, includeNetwork, includeConsole bool) map[string]interface{} {
	response := map[string]interface{}{
		"id":               request.ID,
		"context_name":     request.ContextName,
		"url":              request.URL,
		"timestamp":        request.Timestamp,
		"status_code":      request.StatusCode,
		"response_headers": request.ResponseHeaders,
		"duration":         request.Duration.Milliseconds(),
		"request_type":     request.RequestType,
	}

	if request.Error != "" {
		response["error"] = request.Error
	}

	if len(request.SetCookies) > 0 {
		cookies := make([]map[string]interface{}, len(request.SetCookies))
		for i, cookie := range request.SetCookies {
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

	if includeHTML && request.HTML != "" {
		response["html"] = request.HTML
	}

	if includeNetwork && len(request.NetworkRequests) > 0 {
		response["network_requests"] = request.NetworkRequests
	}

	if includeConsole && len(request.ConsoleLogs) > 0 {
		response["console_logs"] = request.ConsoleLogs
	}

	return response
}

// GenerateRequestID generates a unique request ID
func GenerateRequestID() string {
	return time.Now().Format("20060102150405") + "_" + randomString(8)
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}
