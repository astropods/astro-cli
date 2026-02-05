package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go [serialize|deserialize]")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "serialize":
		serializeFromGo()
	case "deserialize":
		deserializeFromTS()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

// serializeFromGo creates protobuf messages in Go and serializes them to JSON
func serializeFromGo() {
	// Create PlatformContext
	pc := &pb.PlatformContext{
		MessageId:   "C123:1234567890.123456",
		ChannelId:   "C123456",
		ThreadId:    "1234567890.000001",
		ChannelName: "#test-channel",
		WorkspaceId: "T999",
		PlatformData: map[string]string{
			"team_id":    "T999",
			"bot_id":     "B123",
			"custom_key": "custom_value",
		},
	}

	// Create User
	user := &pb.User{
		Id:        "U123456",
		Username:  "testuser",
		Email:     "test@example.com",
		AvatarUrl: "https://example.com/avatar.png",
		UserData: map[string]string{
			"department": "engineering",
			"role":       "developer",
		},
	}

	// Create ThreadMessage
	now := time.Date(2026, 2, 5, 21, 0, 0, 0, time.UTC)
	tm := &pb.ThreadMessage{
		MessageId: "msg-001",
		User: &pb.User{
			Id:       "U999",
			Username: "test",
		},
		Content:         "Test message",
		Timestamp:       timestamppb.New(now),
		WasEdited:       true,
		IsDeleted:       false,
		OriginalContent: "Original",
		PlatformData: map[string]string{
			"key": "value",
		},
	}

	// Create Message with all fields
	msg := &pb.Message{
		Id:             "msg-full-001",
		Platform:       "slack",
		ConversationId: "conv-001",
		Content:        "Test message from Go",
		Timestamp:      timestamppb.New(now),
		PlatformContext: pc,
		User:           user,
	}

	// Serialize using protojson (what gRPC uses)
	marshaler := protojson.MarshalOptions{
		UseProtoNames:   false, // Use JSON names (camelCase)
		EmitUnpopulated: true,  // Include fields with default values (for testing)
		Indent:          "  ",
	}

	// Serialize each type
	pcJSON, _ := marshaler.Marshal(pc)
	userJSON, _ := marshaler.Marshal(user)
	tmJSON, _ := marshaler.Marshal(tm)
	msgJSON, _ := marshaler.Marshal(msg)

	// Create output structure
	output := map[string]json.RawMessage{
		"platformContext": pcJSON,
		"user":            userJSON,
		"threadMessage":   tmJSON,
		"message":         msgJSON,
	}

	// Write to file
	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	if err := os.WriteFile("test-data/go-serialized.json", outputJSON, 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Serialized messages from Go to test-data/go-serialized.json")
	fmt.Printf("PlatformContext JSON:\n%s\n\n", string(pcJSON))
	fmt.Printf("User JSON:\n%s\n\n", string(userJSON))
	fmt.Printf("ThreadMessage JSON:\n%s\n\n", string(tmJSON))
	fmt.Printf("Message JSON:\n%s\n\n", string(msgJSON))
}

// deserializeFromTS reads JSON that was created by TypeScript and deserializes it in Go
func deserializeFromTS() {
	data, err := os.ReadFile("test-data/ts-serialized.json")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		fmt.Println("Run the TypeScript serialization test first to generate ts-serialized.json")
		os.Exit(1)
	}

	var input map[string]json.RawMessage
	if err := json.Unmarshal(data, &input); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: false,
	}

	// Deserialize PlatformContext
	if pcJSON, ok := input["platformContext"]; ok {
		pc := &pb.PlatformContext{}
		if err := unmarshaler.Unmarshal(pcJSON, pc); err != nil {
			fmt.Printf("❌ Failed to deserialize PlatformContext: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Deserialized PlatformContext from TypeScript:")
		fmt.Printf("  messageId: %s\n", pc.MessageId)
		fmt.Printf("  channelId: %s\n", pc.ChannelId)
		fmt.Printf("  threadId: %s\n", pc.ThreadId)
		fmt.Printf("  channelName: %s\n", pc.ChannelName)
		fmt.Printf("  workspaceId: %s\n", pc.WorkspaceId)
		fmt.Printf("  platformData: %v\n\n", pc.PlatformData)
	}

	// Deserialize User
	if userJSON, ok := input["user"]; ok {
		user := &pb.User{}
		if err := unmarshaler.Unmarshal(userJSON, user); err != nil {
			fmt.Printf("❌ Failed to deserialize User: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Deserialized User from TypeScript:")
		fmt.Printf("  id: %s\n", user.Id)
		fmt.Printf("  username: %s\n", user.Username)
		fmt.Printf("  email: %s\n", user.Email)
		fmt.Printf("  avatarUrl: %s\n", user.AvatarUrl)
		fmt.Printf("  userData: %v\n\n", user.UserData)

		// Verify no displayName field was received (it shouldn't exist)
		var rawUser map[string]interface{}
		json.Unmarshal(userJSON, &rawUser)
		if _, hasDisplayName := rawUser["displayName"]; hasDisplayName {
			fmt.Println("❌ ERROR: User has displayName field (shouldn't exist in proto!)")
			os.Exit(1)
		}
	}

	// Deserialize ThreadMessage
	if tmJSON, ok := input["threadMessage"]; ok {
		tm := &pb.ThreadMessage{}
		if err := unmarshaler.Unmarshal(tmJSON, tm); err != nil {
			fmt.Printf("❌ Failed to deserialize ThreadMessage: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Deserialized ThreadMessage from TypeScript:")
		fmt.Printf("  messageId: %s\n", tm.MessageId)
		fmt.Printf("  wasEdited: %v\n", tm.WasEdited)
		fmt.Printf("  isDeleted: %v\n", tm.IsDeleted)
		fmt.Printf("  content: %s\n\n", tm.Content)

		// Verify correct field names
		var rawTM map[string]interface{}
		json.Unmarshal(tmJSON, &rawTM)
		if _, hasWasDeleted := rawTM["wasDeleted"]; hasWasDeleted {
			fmt.Println("❌ ERROR: ThreadMessage has wasDeleted field (should be isDeleted!)")
			os.Exit(1)
		}
	}

	// Deserialize full Message
	if msgJSON, ok := input["message"]; ok {
		msg := &pb.Message{}
		if err := unmarshaler.Unmarshal(msgJSON, msg); err != nil {
			fmt.Printf("❌ Failed to deserialize Message: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Deserialized Message from TypeScript:")
		fmt.Printf("  id: %s\n", msg.Id)
		fmt.Printf("  platform: %s\n", msg.Platform)
		fmt.Printf("  conversationId: %s\n", msg.ConversationId)
		fmt.Printf("  content: %s\n", msg.Content)
		if msg.PlatformContext != nil {
			fmt.Printf("  platformContext.channelId: %s\n", msg.PlatformContext.ChannelId)
		}
		if msg.User != nil {
			fmt.Printf("  user.username: %s\n", msg.User.Username)
		}
		fmt.Println()
	}

	fmt.Println("✓ All TypeScript → Go deserialization successful!")
}
