package main

import (
	"net/url"
	"path/filepath"
	"strings"
)

func ParseDomainWhitelist(whitelist string) ([]string, error) {
	if whitelist == "" {
		return nil, nil
	}

	domains := strings.Split(whitelist, ",")
	var processed []string

	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			processed = append(processed, domain)
		}
	}

	return processed, nil
}

func isDomainWhitelisted(requestURL string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true // No whitelist means allow all
	}

	parsed, err := url.Parse(requestURL)
	if err != nil {
		return false
	}

	hostname := parsed.Hostname()

	for _, pattern := range whitelist {
		// Support glob patterns like *.example.com
		matched, err := filepath.Match(pattern, hostname)
		if err != nil {
			continue
		}
		if matched {
			return true
		}

		// Also check if hostname ends with the pattern (for subdomain matching)
		if strings.HasPrefix(pattern, ".") {
			if strings.HasSuffix(hostname, pattern) || hostname == strings.TrimPrefix(pattern, ".") {
				return true
			}
		}
	}

	return false
}
