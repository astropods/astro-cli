# Simple Agent Example

This is a simple example agent that demonstrates how to use the gRPC messaging client to build AI agents that work with the astro-messaging service.

## Features Demonstrated

- Connecting to the gRPC messaging service
- Health check
- Bidirectional conversation streaming
- Processing incoming messages
- Sending responses (status updates, content, suggested prompts)
- Graceful shutdown

## Usage

### 1. Start the messaging service

```bash
# From the root of astro-messaging
cd packages/astro-messaging

# Set required environment variables
export SLACK_BOT_TOKEN="xoxb-your-bot-token"
export SLACK_APP_TOKEN="xapp-your-app-token"
export SLACK_ENABLED=true
export GRPC_ENABLED=true

# Start the sidecar
go run cmd/sidecar/main.go
```

The service will start:
- gRPC server on `:9090` (default)
- HTTP API server on `:8081` (default)
- Slack adapter in Socket Mode

### 2. Run the example agent

```bash
# From the examples/simple-agent directory
cd examples/simple-agent

# Run the agent
go run main.go
```

The agent will:
1. Connect to the gRPC server at `localhost:9090`
2. Perform a health check
3. Establish a bidirectional conversation stream
4. Wait for messages from the platform

### 3. Test it in Slack

1. Add your bot to a Slack channel
2. Mention the bot: `@YourBot hello`
3. Watch the logs to see the message flow

## Architecture

```
Slack App
    ↓
Socket Mode Connection
    ↓
Slack Adapter (adapter.go)
    ↓
gRPC Server (server.go)
    ↓
Bidirectional Stream
    ↓
Your Agent (this example)
```

## Message Flow

### Incoming Message (Slack → Agent)

1. User sends message in Slack
2. Slack Socket Mode delivers event to adapter
3. Adapter translates to proto message
4. gRPC server forwards to agent via stream
5. Agent processes message

### Outgoing Response (Agent → Slack)

1. Agent generates response
2. Agent sends proto response via stream
3. gRPC server forwards to adapter
4. Adapter calls Slack AI APIs:
   - `assistant.threads.setStatus` (status updates)
   - `assistant.threads.setSuggestedPrompts` (quick replies)
   - `chat.postMessage` (final content)

## Extending This Example

### Add LLM Integration

```go
import "github.com/your-org/llm-client"

func generateResponse(input string) string {
    // Replace mock with real LLM
    client := llm.NewClient(os.Getenv("LLM_API_KEY"))
    response, err := client.Complete(input)
    if err != nil {
        return "Sorry, I encountered an error."
    }
    return response
}
```

### Send Status Updates

```go
// Send "Thinking..." status while processing
statusResp := sendStatus(conversationID, pb.StatusUpdate_THINKING, "")
stream.Send(&pb.ConversationRequest{
    Request: &pb.ConversationRequest_Response{
        Response: statusResp,
    },
})

// Process with LLM
response := generateResponse(msg.Content)

// Send final content
contentResp := sendContent(conversationID, response)
stream.Send(&pb.ConversationRequest{
    Request: &pb.ConversationRequest_Response{
        Response: contentResp,
    },
})
```

### Add Context Awareness

```go
// Fetch thread history for context
history, err := messagingClient.GetThreadHistory(ctx, conversationID, 50)
if err != nil {
    log.Printf("Failed to get history: %v", err)
} else {
    // Use history for context in your LLM prompt
    context := buildContextFromHistory(history.Messages)
    response := llm.CompleteWithContext(msg.Content, context)
}
```

### Add Suggested Prompts

```go
// Send suggested follow-up prompts
prompts := []string{
    "Tell me more",
    "What else can you do?",
    "Help",
}
promptResp := sendSuggestedPrompts(conversationID, prompts)
stream.Send(&pb.ConversationRequest{
    Request: &pb.ConversationRequest_Response{
        Response: promptResp,
    },
})
```

## Environment Variables

- `GRPC_SERVER_ADDR` - gRPC server address (default: `localhost:9090`)

## See Also

- [gRPC AI Messaging Spec](../../docs/grpc-ai-messaging-spec.md)
- [Slack Adapter Spec](../../docs/slack-adapter-spec.md)
- [Implementation Status](../../docs/implementation-status.md)
- [Client Library Documentation](../../pkg/client/go/messaging_client.go)
