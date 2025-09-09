package main

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseViewportString(viewport string) (int, int, error) {
	if viewport == "" {
		return 0, 0, nil
	}

	parts := strings.Split(viewport, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid viewport format, expected WxH")
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil || width <= 0 {
		return 0, 0, fmt.Errorf("invalid viewport width")
	}

	height, err := strconv.Atoi(parts[1])
	if err != nil || height <= 0 {
		return 0, 0, fmt.Errorf("invalid viewport height")
	}

	return width, height, nil
}
