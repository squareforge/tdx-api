package main

import (
	"os"
	"strings"
)

const defaultAppVersion = "0.1.3"

func appVersion() string {
	if value := strings.TrimSpace(os.Getenv("TDX_WORKBENCH_VERSION")); value != "" {
		return value
	}
	return defaultAppVersion
}
