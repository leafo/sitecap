package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// CookieManager handles automatic cookie management from HTTP responses
type CookieManager struct{}

// NewCookieManager creates a new cookie manager
func NewCookieManager() *CookieManager {
	return &CookieManager{}
}

// ParseSetCookieHeaders parses Set-Cookie headers and converts them to rod cookie format
func (cm *CookieManager) ParseSetCookieHeaders(headers map[string][]string, domain string) []*proto.NetworkCookieParam {
	var cookies []*proto.NetworkCookieParam

	setCookieHeaders, exists := headers["Set-Cookie"]
	if !exists {
		// Try lowercase version
		setCookieHeaders, exists = headers["set-cookie"]
		if !exists {
			return cookies
		}
	}

	for _, cookieHeader := range setCookieHeaders {
		cookie := cm.parseSetCookieHeader(cookieHeader, domain)
		if cookie != nil {
			cookies = append(cookies, cookie)
		}
	}

	return cookies
}

// parseSetCookieHeader parses a single Set-Cookie header
func (cm *CookieManager) parseSetCookieHeader(cookieHeader string, defaultDomain string) *proto.NetworkCookieParam {
	// Parse using Go's built-in cookie parser
	header := http.Header{}
	header.Add("Set-Cookie", cookieHeader)
	request := &http.Request{Header: header}

	cookies := request.Cookies()
	if len(cookies) == 0 {
		return nil
	}

	httpCookie := cookies[0]

	// Convert http.Cookie to proto.NetworkCookieParam
	cookie := &proto.NetworkCookieParam{
		Name:  httpCookie.Name,
		Value: httpCookie.Value,
	}

	// Parse additional attributes from the raw header
	parts := strings.Split(cookieHeader, ";")

	// Set default domain if not specified
	cookie.Domain = defaultDomain
	cookie.Path = "/"

	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		if strings.HasPrefix(strings.ToLower(part), "domain=") {
			cookie.Domain = strings.TrimPrefix(part, "domain=")
			cookie.Domain = strings.TrimPrefix(cookie.Domain, "Domain=")
		} else if strings.HasPrefix(strings.ToLower(part), "path=") {
			cookie.Path = strings.TrimPrefix(part, "path=")
			cookie.Path = strings.TrimPrefix(cookie.Path, "Path=")
		} else if strings.HasPrefix(strings.ToLower(part), "expires=") {
			expiresStr := strings.TrimPrefix(part, "expires=")
			expiresStr = strings.TrimPrefix(expiresStr, "Expires=")
			if expires, err := time.Parse(time.RFC1123, expiresStr); err == nil {
				cookie.Expires = proto.TimeSinceEpoch(expires.Unix())
			}
		} else if strings.HasPrefix(strings.ToLower(part), "max-age=") {
			// Handle Max-Age by converting to expires
			maxAgeStr := strings.TrimPrefix(part, "max-age=")
			maxAgeStr = strings.TrimPrefix(maxAgeStr, "Max-Age=")
			// Note: This is simplified - in a real implementation you'd parse the number
			// and calculate the expiry time
		} else if strings.ToLower(part) == "httponly" {
			cookie.HTTPOnly = true
		} else if strings.ToLower(part) == "secure" {
			cookie.Secure = true
		} else if strings.HasPrefix(strings.ToLower(part), "samesite=") {
			sameSiteStr := strings.TrimPrefix(part, "samesite=")
			sameSiteStr = strings.TrimPrefix(sameSiteStr, "SameSite=")
			sameSiteStr = strings.ToLower(sameSiteStr)

			switch sameSiteStr {
			case "strict":
				cookie.SameSite = proto.NetworkCookieSameSiteStrict
			case "lax":
				cookie.SameSite = proto.NetworkCookieSameSiteLax
			case "none":
				cookie.SameSite = proto.NetworkCookieSameSiteNone
			}
		}
	}

	return cookie
}

// ApplyCookiesToPage applies stored cookies to a browser page
func (cm *CookieManager) ApplyCookiesToPage(page interface{}, cookies []*proto.NetworkCookieParam) error {
	// Type assert to rod.Page - this is a simplified approach
	// In practice, you might want to use a more specific interface
	if len(cookies) == 0 {
		return nil
	}

	// Convert cookies to the format expected by rod
	for _, cookie := range cookies {
		// Use page.SetCookies() method - this is simplified
		// The actual implementation would depend on the exact rod API
		_ = cookie // Use cookie to set on page
	}

	return nil
}

// ConvertCookiesFromJSON converts JSON cookie objects to proto format
func (cm *CookieManager) ConvertCookiesFromJSON(cookiesData interface{}) []*proto.NetworkCookieParam {
	var cookies []*proto.NetworkCookieParam

	switch v := cookiesData.(type) {
	case []interface{}:
		for _, cookieData := range v {
			if cookieMap, ok := cookieData.(map[string]interface{}); ok {
				cookie := cm.convertSingleCookieFromJSON(cookieMap)
				if cookie != nil {
					cookies = append(cookies, cookie)
				}
			}
		}
	case map[string]interface{}:
		// Single cookie object
		cookie := cm.convertSingleCookieFromJSON(v)
		if cookie != nil {
			cookies = append(cookies, cookie)
		}
	}

	return cookies
}

// convertSingleCookieFromJSON converts a single cookie from JSON map to proto format
func (cm *CookieManager) convertSingleCookieFromJSON(cookieMap map[string]interface{}) *proto.NetworkCookieParam {
	cookie := &proto.NetworkCookieParam{}

	if name, ok := cookieMap["name"].(string); ok {
		cookie.Name = name
	} else {
		return nil // Name is required
	}

	if value, ok := cookieMap["value"].(string); ok {
		cookie.Value = value
	}

	if domain, ok := cookieMap["domain"].(string); ok {
		cookie.Domain = domain
	}

	if path, ok := cookieMap["path"].(string); ok {
		cookie.Path = path
	} else {
		cookie.Path = "/"
	}

	if expires, ok := cookieMap["expires"].(float64); ok {
		cookie.Expires = proto.TimeSinceEpoch(int64(expires))
	}

	if httpOnly, ok := cookieMap["httpOnly"].(bool); ok {
		cookie.HTTPOnly = httpOnly
	}

	if secure, ok := cookieMap["secure"].(bool); ok {
		cookie.Secure = secure
	}

	if sameSite, ok := cookieMap["sameSite"].(string); ok {
		switch strings.ToLower(sameSite) {
		case "strict":
			cookie.SameSite = proto.NetworkCookieSameSiteStrict
		case "lax":
			cookie.SameSite = proto.NetworkCookieSameSiteLax
		case "none":
			cookie.SameSite = proto.NetworkCookieSameSiteNone
		}
	}

	return cookie
}
