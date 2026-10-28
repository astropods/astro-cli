package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessagingEndpoint(t *testing.T) {
	urls := []serviceEndpointInfo{
		{Type: "agent", URL: "https://a.example.com"},
		{Type: "messaging", URL: "https://m.example.com"},
	}
	ep := messagingEndpoint(urls)
	require.NotNil(t, ep)
	assert.Equal(t, "https://m.example.com", ep.URL)
}
