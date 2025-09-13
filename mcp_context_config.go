package main

import (
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// BrowserContextConfig stores browser configuration for a named context
type BrowserContextConfig struct {
	Name            string
	DefaultViewport ViewportConfig
	DefaultTimeout  int
	DomainWhitelist []string
	Cookies         []*proto.NetworkCookieParam
	Headers         map[string]string
	LastRequestID   string
	RequestHistory  []string // Request IDs in chronological order
	CreatedAt       time.Time
	LastUsed        time.Time
	mutex           sync.RWMutex
}

func DefaultBrowserContextConfig() *BrowserContextConfig {
	// Start with default values
	viewport := ViewportConfig{Width: 1366, Height: 854}
	timeout := 30
	var domainWhitelist []string
	headers := make(map[string]string)

	// Apply global CLI flags if they were set
	if globalViewport != "" {
		if width, height, err := ParseViewportString(globalViewport); err == nil {
			viewport = ViewportConfig{Width: width, Height: height}
		}
	}

	if globalTimeout > 0 {
		timeout = globalTimeout
	}

	if globalDomains != "" {
		if domains, err := ParseDomainWhitelist(globalDomains); err == nil {
			domainWhitelist = domains
		}
	}

	if globalCustomHeaders != nil {
		headers = globalCustomHeaders
	}

	return &BrowserContextConfig{
		Name:            "default",
		DefaultViewport: viewport,
		DefaultTimeout:  timeout,
		DomainWhitelist: domainWhitelist,
		Cookies:         []*proto.NetworkCookieParam{},
		Headers:         headers,
		RequestHistory:  []string{},
	}
}

// ViewportConfig represents viewport dimensions
type ViewportConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// UpdateCookies updates the cookies for this context
func (c *BrowserContextConfig) UpdateCookies(newCookies []*proto.NetworkCookieParam, merge bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if merge && len(c.Cookies) > 0 {
		// Merge cookies - new cookies override existing ones with same name/domain
		cookieMap := make(map[string]*proto.NetworkCookieParam)

		// Add existing cookies
		for _, cookie := range c.Cookies {
			key := cookie.Name + "|" + cookie.Domain
			cookieMap[key] = cookie
		}

		// Add/override with new cookies
		for _, cookie := range newCookies {
			key := cookie.Name + "|" + cookie.Domain
			cookieMap[key] = cookie
		}

		// Convert back to slice
		c.Cookies = make([]*proto.NetworkCookieParam, 0, len(cookieMap))
		for _, cookie := range cookieMap {
			c.Cookies = append(c.Cookies, cookie)
		}
	} else {
		// Replace all cookies
		c.Cookies = newCookies
	}

	c.LastUsed = time.Now()
}

// AddRequestToHistory adds a request ID to this context's history
func (c *BrowserContextConfig) AddRequestToHistory(requestID string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.RequestHistory = append(c.RequestHistory, requestID)
	c.LastRequestID = requestID
	c.LastUsed = time.Now()
}

// ContextConfigManager manages named browser contexts that hold settings about
// rendering and persistent data like cookies, headers, etc.
type ContextConfigManager struct {
	contexts map[string]*BrowserContextConfig
	mutex    sync.RWMutex
}

func NewContextConfigManager() *ContextConfigManager {
	context_manager := &ContextConfigManager{
		contexts: make(map[string]*BrowserContextConfig),
	}

	context_manager.CreateOrUpdateContext("default", DefaultBrowserContextConfig())

	return context_manager
}

// CreateOrUpdateContext creates or updates a browser context configuration
func (m *ContextConfigManager) CreateOrUpdateContext(name string, config *BrowserContextConfig) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if existing, exists := m.contexts[name]; exists {
		// Update existing context, preserve request history
		config.RequestHistory = existing.RequestHistory
		config.LastRequestID = existing.LastRequestID
		config.CreatedAt = existing.CreatedAt
	} else {
		// New context
		config.CreatedAt = time.Now()
		config.RequestHistory = make([]string, 0)
	}

	config.Name = name
	config.LastUsed = time.Now()
	m.contexts[name] = config
}

// GetContext retrieves a browser context configuration
func (m *ContextConfigManager) GetContext(name string) (*BrowserContextConfig, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if name == "" {
		name = "default"
	}

	context, exists := m.contexts[name]
	return context, exists
}

// ListContexts returns all context names and basic info
func (m *ContextConfigManager) ListContexts() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]interface{})
	for name, context := range m.contexts {
		result[name] = map[string]interface{}{
			"created_at":    context.CreatedAt,
			"last_used":     context.LastUsed,
			"request_count": len(context.RequestHistory),
			"viewport":      context.DefaultViewport,
			"timeout":       context.DefaultTimeout,
			"cookies":       context.Cookies,
			"headers":       context.Headers,
		}
	}
	return result
}

// DeleteContext removes a browser context configuration
func (m *ContextConfigManager) DeleteContext(name string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.contexts[name]; exists {
		delete(m.contexts, name)
		return true
	}
	return false
}
