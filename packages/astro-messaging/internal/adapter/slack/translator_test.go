package slack

import (
	"testing"

	"github.com/slack-go/slack/slackevents"
)

func TestTranslateMessageEvent(t *testing.T) {
	ev := &slackevents.MessageEvent{
		Type:      "message",
		User:      "U123456",
		Text:      "Hello, how can you help me?",
		Channel:   "C123456",
		TimeStamp: "1234567890.123456",
	}

	msg := TranslateMessageEvent(ev)

	if msg.Platform != "slack" {
		t.Errorf("Expected platform 'slack', got '%s'", msg.Platform)
	}

	if msg.UserID != "U123456" {
		t.Errorf("Expected user ID 'U123456', got '%s'", msg.UserID)
	}

	if msg.ChannelID != "C123456" {
		t.Errorf("Expected channel ID 'C123456', got '%s'", msg.ChannelID)
	}

	if msg.ConversationID != "C123456" {
		t.Errorf("Expected conversation ID 'C123456', got '%s'", msg.ConversationID)
	}

	if msg.Content != ev.Text {
		t.Errorf("Expected content '%s', got '%s'", ev.Text, msg.Content)
	}
}

func TestTranslateMessageEvent_WithThread(t *testing.T) {
	ev := &slackevents.MessageEvent{
		Type:            "message",
		User:            "U123456",
		Text:            "Thread reply",
		Channel:         "C123456",
		TimeStamp:       "1234567890.999999",
		ThreadTimeStamp: "1234567890.123456",
	}

	msg := TranslateMessageEvent(ev)

	expectedConversationID := "C123456-1234567890.123456"
	if msg.ConversationID != expectedConversationID {
		t.Errorf("Expected conversation ID '%s', got '%s'", expectedConversationID, msg.ConversationID)
	}

	if msg.ThreadID != "1234567890.123456" {
		t.Errorf("Expected thread ID '1234567890.123456', got '%s'", msg.ThreadID)
	}
}

func TestStripMentions(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<@U123456> Hello!", "Hello!"},            // Trims leading/trailing spaces
		{"Hey <@U123456> how are you?", "Hey  how are you?"}, // Doesn't trim internal spaces
		{"No mentions here", "No mentions here"},
		{"<@U123> <@U456> Multiple mentions", "Multiple mentions"}, // Trims leading/trailing spaces
	}

	for _, test := range tests {
		result := stripMentions(test.input)
		if result != test.expected {
			t.Errorf("For input '%s', expected '%s', got '%s'", test.input, test.expected, result)
		}
	}
}

func TestFormatMessageID(t *testing.T) {
	channelID := "C123456"
	timestamp := "1234567890.123456"

	messageID := FormatMessageID(channelID, timestamp)
	expected := "C123456:1234567890.123456"

	if messageID != expected {
		t.Errorf("Expected message ID '%s', got '%s'", expected, messageID)
	}
}

func TestParseMessageID(t *testing.T) {
	messageID := "C123456:1234567890.123456"

	channelID, timestamp := ParseMessageID(messageID)

	if channelID != "C123456" {
		t.Errorf("Expected channel ID 'C123456', got '%s'", channelID)
	}

	if timestamp != "1234567890.123456" {
		t.Errorf("Expected timestamp '1234567890.123456', got '%s'", timestamp)
	}
}

func TestParseMessageID_Invalid(t *testing.T) {
	messageID := "invalid-format"

	channelID, timestamp := ParseMessageID(messageID)

	// New behavior: returns empty strings for invalid format
	if channelID != "" {
		t.Errorf("Expected empty channel ID, got '%s'", channelID)
	}

	if timestamp != "" {
		t.Errorf("Expected empty timestamp, got '%s'", timestamp)
	}
}
