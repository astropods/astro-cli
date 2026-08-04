package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveBuildPlatform(t *testing.T) {
	tests := []struct {
		name         string
		serverURL    string
		agentCore    bool
		wantPlatform string
		wantSkipPush bool
	}{
		{
			name:         "remote server, default runtime -> amd64 and push",
			serverURL:    "https://api.astropod.ai",
			agentCore:    false,
			wantPlatform: "linux/amd64",
			wantSkipPush: false,
		},
		{
			name:         "local server, default runtime -> native and skip push",
			serverURL:    "http://localhost:8080",
			agentCore:    false,
			wantPlatform: nativePlatform(),
			wantSkipPush: true,
		},
		{
			name:         "remote server, agentcore -> arm64 and push",
			serverURL:    "https://api.astropod.ai",
			agentCore:    true,
			wantPlatform: "linux/arm64",
			wantSkipPush: false,
		},
		{
			name:         "local server, agentcore -> arm64 but still skip push",
			serverURL:    "http://127.0.0.1:8080",
			agentCore:    true,
			wantPlatform: "linux/arm64",
			wantSkipPush: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, skipPush := resolveBuildPlatform(tt.serverURL, tt.agentCore)
			assert.Equal(t, tt.wantPlatform, platform)
			assert.Equal(t, tt.wantSkipPush, skipPush)
		})
	}
}
