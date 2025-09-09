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
	UserAgent       string
	LastRequestID   string
	RequestHistory  []string // Request IDs in chronological order
	CreatedAt       time.Time
	LastUsed        time.Time
}

// ViewportConfig represents viewport dimensions
type ViewportConfig struct {
	Width  int
	Height int
}

// ContextConfigManager manages browser context configurations
type ContextConfigManager struct {
	contexts map[string]*BrowserContextConfig
	mutex    sync.RWMutex
}

// NewContextConfigManager creates a new context configuration manager
func NewContextConfigManager() *ContextConfigManager {
	return &ContextConfigManager{
		contexts: make(map[string]*BrowserContextConfig),
	}
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
	if exists {
		// Update last used time
		go func() {
			m.mutex.Lock()
			context.LastUsed = time.Now()
			m.mutex.Unlock()
		}()
	}
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

// AddRequestToHistory adds a request ID to the context's history
func (m *ContextConfigManager) AddRequestToHistory(contextName string, requestID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if contextName == "" {
		contextName = "default"
	}

	if context, exists := m.contexts[contextName]; exists {
		context.RequestHistory = append(context.RequestHistory, requestID)
		context.LastRequestID = requestID
		context.LastUsed = time.Now()
	}
}

// UpdateCookies updates the cookies for a context (from set-cookie headers)
func (m *ContextConfigManager) UpdateCookies(contextName string, newCookies []*proto.NetworkCookieParam, merge bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if contextName == "" {
		contextName = "default"
	}

	context, exists := m.contexts[contextName]
	if !exists {
		// Create default context if it doesn't exist
		context = &BrowserContextConfig{
			Name:            contextName,
			DefaultViewport: ViewportConfig{Width: 1920, Height: 1080},
			DefaultTimeout:  30,
			Headers:         make(map[string]string),
			RequestHistory:  make([]string, 0),
			CreatedAt:       time.Now(),
		}
		m.contexts[contextName] = context
	}

	if merge && len(context.Cookies) > 0 {
		// Merge cookies - new cookies override existing ones with same name/domain
		cookieMap := make(map[string]*proto.NetworkCookieParam)

		// Add existing cookies
		for _, cookie := range context.Cookies {
			key := cookie.Name + "|" + cookie.Domain
			cookieMap[key] = cookie
		}

		// Add/override with new cookies
		for _, cookie := range newCookies {
			key := cookie.Name + "|" + cookie.Domain
			cookieMap[key] = cookie
		}

		// Convert back to slice
		context.Cookies = make([]*proto.NetworkCookieParam, 0, len(cookieMap))
		for _, cookie := range cookieMap {
			context.Cookies = append(context.Cookies, cookie)
		}
	} else {
		// Replace all cookies
		context.Cookies = newCookies
	}

	context.LastUsed = time.Now()
}
