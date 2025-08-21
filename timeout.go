package main

import (
	"fmt"
	"strconv"
)

func parseTimeoutString(timeout string) (int, error) {
	if timeout == "" {
		return 0, nil
	}

	seconds, err := strconv.Atoi(timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout value, must be a number")
	}

	if seconds < 0 {
		return 0, fmt.Errorf("timeout cannot be negative")
	}

	if seconds > 300 { // Max 5 minutes
		return 0, fmt.Errorf("timeout cannot exceed 300 seconds")
	}

	return seconds, nil
}
