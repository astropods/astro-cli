package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/astro/messaging/pkg/client/go"
	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
)

func main() {
	// Get gRPC server address from environment or use default
	serverAddr := os.Getenv("GRPC_SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:9090"
	}

	log.Printf("Connecting to messaging service at %s...", serverAddr)

	// Create messaging client
	messagingClient, err := client.NewClient(serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer messagingClient.Close()

	log.Println("Connected successfully!")

	// Check health
	ctx := context.Background()
	health, err := messagingClient.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		log.Printf("Health check: %s", health.Status)
	}

	// Create bidirectional conversation stream
	stream, err := messagingClient.ProcessConversation(ctx)
	if err != nil {
		log.Fatalf("Failed to create conversation stream: %v", err)
	}
	defer stream.Close()

	log.Println("Conversation stream established. Waiting for messages...")

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start receiving messages in background
	go func() {
		err := stream.ReceiveAll(func(resp *pb.AgentResponse) error {
			return handleAgentResponse(resp)
		})
		if err != nil {
			log.Printf("Stream receive error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigCh
	log.Println("Shutting down gracefully...")
}

// handleAgentResponse processes incoming messages and generates responses
func handleAgentResponse(resp *pb.AgentResponse) error {
	// This is actually a message from the platform (confusing naming in proto)
	// In a real implementation, you would check if this is a context request
	// For demo purposes, we'll simulate handling a message

	log.Printf("Received response: conversation_id=%s", resp.ConversationId)

	// In a real agent, you would:
	// 1. Check if this is a context request (thread history)
	// 2. Process the message with your LLM
	// 3. Send back status updates, content, and prompts

	// For now, let's just log it
	switch payload := resp.Payload.(type) {
	case *pb.AgentResponse_ContextRequest:
		log.Printf("Context request received for conversation: %s", payload.ContextRequest.ConversationId)
		// You would fetch and return thread history here
	default:
		log.Printf("Received payload type: %T", payload)
	}

	return nil
}

// Example of how an agent would process a message
func processMessage(ctx context.Context, messagingClient *client.MessagingClient, msg *pb.Message) error {
	log.Printf("Processing message from %s: %s", msg.User.Username, msg.Content)

	// Create a message stream for responses
	stream, err := messagingClient.ProcessMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to create message stream: %w", err)
	}

	// Send status update: "Thinking..."
	// Note: In the actual implementation, you would send responses through the stream
	// This is just demonstrating the API

	// Simulate processing
	time.Sleep(500 * time.Millisecond)

	// Process with your LLM
	response := generateResponse(msg.Content)

	log.Printf("Generated response: %s", response)

	// Receive any responses from the stream
	return stream.ReceiveAll(func(resp *pb.AgentResponse) error {
		log.Printf("Received response: %v", resp)
		return nil
	})
}

// generateResponse is a simple mock LLM that generates responses
func generateResponse(input string) string {
	input = strings.ToLower(input)

	switch {
	case strings.Contains(input, "hello") || strings.Contains(input, "hi"):
		return "Hello! How can I help you today?"
	case strings.Contains(input, "help"):
		return "I'm a simple agent. I can respond to greetings and basic queries. Try asking me something!"
	case strings.Contains(input, "weather"):
		return "I don't have access to weather data, but it's probably nice outside! 🌤️"
	case strings.Contains(input, "time"):
		return fmt.Sprintf("The current time is %s", time.Now().Format("3:04 PM"))
	default:
		return "I heard you! I'm a simple agent, so I can only handle basic queries. Try asking about the weather or time."
	}
}

// Example helper functions that an agent might use

// sendStatus sends a status update to the platform
func sendStatus(conversationID string, status pb.StatusUpdate_Status, message string) *pb.AgentResponse {
	return &pb.AgentResponse{
		ConversationId: conversationID,
		Payload: &pb.AgentResponse_Status{
			Status: &pb.StatusUpdate{
				Status:        status,
				CustomMessage: message,
			},
		},
	}
}

// sendContent sends content to the platform
func sendContent(conversationID, content string) *pb.AgentResponse {
	return &pb.AgentResponse{
		ConversationId: conversationID,
		Payload: &pb.AgentResponse_Content{
			Content: &pb.ContentChunk{
				Type:    pb.ContentChunk_END,
				Content: content,
			},
		},
	}
}

// sendSuggestedPrompts sends suggested prompts to the platform
func sendSuggestedPrompts(conversationID string, prompts []string) *pb.AgentResponse {
	protoPrompts := make([]*pb.SuggestedPrompts_Prompt, 0, len(prompts))
	for i, p := range prompts {
		protoPrompts = append(protoPrompts, &pb.SuggestedPrompts_Prompt{
			Id:      fmt.Sprintf("prompt_%d", i),
			Title:   p,
			Message: p,
		})
	}

	return &pb.AgentResponse{
		ConversationId: conversationID,
		Payload: &pb.AgentResponse_Prompts{
			Prompts: &pb.SuggestedPrompts{
				Prompts: protoPrompts,
			},
		},
	}
}
