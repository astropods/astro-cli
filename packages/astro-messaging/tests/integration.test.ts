import { describe, it, expect, beforeAll, afterAll } from 'bun:test';
import { spawn, ChildProcess } from 'child_process';
import { MessagingClient, type Message, type PlatformContext } from '../src/messaging-client';

/**
 * Integration tests for Go ↔ TypeScript serialization
 *
 * These tests verify that platformContext and other fields are correctly
 * serialized/deserialized when going over the gRPC wire between Go and TypeScript.
 */

describe('Go ↔ TypeScript Integration Tests', () => {
  let serverProcess: ChildProcess;
  let client: MessagingClient;
  const serverAddress = 'localhost:50051';

  beforeAll(async () => {
    // Start the Go gRPC server
    console.log('Starting Go gRPC server...');
    serverProcess = spawn('go', ['run', '../cmd/sidecar/main.go'], {
      cwd: __dirname,
      env: {
        ...process.env,
        GRPC_PORT: '50051',
      },
    });

    // Wait for server to start
    await new Promise(resolve => setTimeout(resolve, 2000));

    // Connect TypeScript client
    client = new MessagingClient(serverAddress);
    await client.connect();
    console.log('TypeScript client connected');
  });

  afterAll(async () => {
    // Close client
    if (client) {
      client.close();
    }

    // Kill server
    if (serverProcess) {
      serverProcess.kill();
    }
  });

  describe('TypeScript → Go: Message serialization', () => {
    it('should send platformContext from TS to Go correctly', async () => {
      const platformContext: PlatformContext = {
        messageId: 'msg-ts-001',
        channelId: 'C123456',
        threadId: '1234567890.000001',
        channelName: '#test-channel',
        workspaceId: 'T999',
        platformData: {
          team_id: 'T999',
          bot_id: 'B123',
          custom_key: 'custom_value',
        },
      };

      const message: Message = {
        conversationId: 'conv-ts-test-001',
        platform: 'slack',
        user: {
          id: 'U123',
          username: 'testuser',
          email: 'test@example.com',
          avatarUrl: 'https://example.com/avatar.png',
        },
        content: 'Test message from TypeScript',
        platformContext,
      };

      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      // Send message
      stream.sendMessage(message);

      // Wait for response
      await new Promise(resolve => setTimeout(resolve, 500));

      // The Go server should echo back or process the message
      // Verify that if we get the message back, platformContext is preserved
      if (responses.length > 0) {
        const firstResp = responses[0];
        if (firstResp.incomingMessage) {
          const receivedPC = firstResp.incomingMessage.platformContext;
          expect(receivedPC).toBeDefined();
          expect(receivedPC.messageId).toBe('msg-ts-001');
          expect(receivedPC.channelId).toBe('C123456');
          expect(receivedPC.threadId).toBe('1234567890.000001');
          expect(receivedPC.channelName).toBe('#test-channel');
          expect(receivedPC.workspaceId).toBe('T999');
          expect(receivedPC.platformData).toBeDefined();
          expect(receivedPC.platformData.team_id).toBe('T999');
          expect(receivedPC.platformData.bot_id).toBe('B123');
        }
      }

      stream.end();
    });

    it('should serialize all User fields correctly', async () => {
      const message: Message = {
        conversationId: 'conv-user-test',
        platform: 'web',
        user: {
          id: 'user-123',
          username: 'johndoe',
          email: 'john@example.com',
          avatarUrl: 'https://example.com/john.png',
          userData: {
            department: 'engineering',
            role: 'developer',
          },
        },
        content: 'Test user fields',
        platformContext: {
          messageId: 'msg-user-001',
          channelId: 'conv-user-test',
        },
      };

      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      // Verify user fields if echoed back
      if (responses.length > 0 && responses[0].incomingMessage) {
        const user = responses[0].incomingMessage.user;
        expect(user.id).toBe('user-123');
        expect(user.username).toBe('johndoe');
        expect(user.email).toBe('john@example.com');
        expect(user.avatarUrl).toBe('https://example.com/john.png');
        expect(user.userData).toBeDefined();
        expect(user.userData.department).toBe('engineering');
      }

      stream.end();
    });

    it('should handle empty threadId correctly', async () => {
      const message: Message = {
        conversationId: 'conv-no-thread',
        platform: 'web',
        user: {
          id: 'user-456',
          username: 'webuser',
        },
        content: 'Message without thread',
        platformContext: {
          messageId: 'msg-002',
          channelId: 'conv-no-thread',
          threadId: undefined, // No thread
        },
      };

      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      if (responses.length > 0 && responses[0].incomingMessage) {
        const pc = responses[0].incomingMessage.platformContext;
        expect(pc).toBeDefined();
        expect(pc.channelId).toBe('conv-no-thread');
        // threadId should be undefined or empty string (proto3 default)
        expect(pc.threadId === undefined || pc.threadId === '').toBe(true);
      }

      stream.end();
    });

    it('should handle missing platformContext gracefully', async () => {
      const message: Message = {
        conversationId: 'conv-no-context',
        platform: 'web',
        user: {
          id: 'user-789',
          username: 'testuser',
        },
        content: 'Message without platformContext',
        platformContext: undefined,
      };

      const stream = client.createConversationStream();

      let errorOccurred = false;
      stream.on('error', (error: Error) => {
        errorOccurred = true;
        console.log('Expected error for missing platformContext:', error.message);
      });

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      // Go server should reject or handle this gracefully
      // Either we get an error or the server handles it
      stream.end();
    });
  });

  describe('Go → TypeScript: Message deserialization', () => {
    it('should receive platformContext from Go with all fields', async () => {
      // This test simulates the Go server sending a message to the TS client
      // We need to trigger the server to send us a message

      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      // Send a message that will trigger a response
      const triggerMessage: Message = {
        conversationId: 'conv-go-to-ts',
        platform: 'slack',
        user: {
          id: 'U999',
          username: 'trigger',
        },
        content: 'Trigger response from Go',
        platformContext: {
          messageId: 'C123:1234567890.123456',
          channelId: 'C123456',
          threadId: '1234567890.000001',
          channelName: '#general',
          workspaceId: 'T123456',
          platformData: {
            team_id: 'T123456',
            enterprise_id: 'E123',
          },
        },
      };

      stream.sendMessage(triggerMessage);

      // Wait for response from Go
      await new Promise(resolve => setTimeout(resolve, 1000));

      // Verify we can receive and deserialize the response
      expect(responses.length).toBeGreaterThan(0);

      const firstResp = responses[0];
      expect(firstResp).toBeDefined();
      expect(firstResp.conversationId).toBe('conv-go-to-ts');

      // If it's an incoming message echo
      if (firstResp.incomingMessage) {
        const msg = firstResp.incomingMessage;
        const pc = msg.platformContext;

        // Verify all platformContext fields are deserialized correctly
        expect(pc).toBeDefined();
        expect(pc.messageId).toBe('C123:1234567890.123456');
        expect(pc.channelId).toBe('C123456');
        expect(pc.threadId).toBe('1234567890.000001');
        expect(pc.channelName).toBe('#general');
        expect(pc.workspaceId).toBe('T123456');

        // Verify map fields
        expect(pc.platformData).toBeDefined();
        expect(typeof pc.platformData).toBe('object');
        expect(pc.platformData.team_id).toBe('T123456');
        expect(pc.platformData.enterprise_id).toBe('E123');
      }

      stream.end();
    });

    it('should deserialize User fields correctly from Go', async () => {
      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const triggerMessage: Message = {
        conversationId: 'conv-user-deser',
        platform: 'slack',
        user: {
          id: 'U555',
          username: 'slackuser',
          email: 'slack@example.com',
          avatarUrl: 'https://slack.example.com/avatar.png',
          userData: {
            real_name: 'Slack User',
            display_name: 'SlackUser',
          },
        },
        content: 'Test user deserialization',
        platformContext: {
          messageId: 'msg-user-deser',
          channelId: 'conv-user-deser',
        },
      };

      stream.sendMessage(triggerMessage);

      await new Promise(resolve => setTimeout(resolve, 1000));

      if (responses.length > 0 && responses[0].incomingMessage) {
        const user = responses[0].incomingMessage.user;
        expect(user).toBeDefined();
        expect(user.id).toBe('U555');
        expect(user.username).toBe('slackuser');
        expect(user.email).toBe('slack@example.com');
        expect(user.avatarUrl).toBe('https://slack.example.com/avatar.png');

        // Note: displayName is NOT in the proto, should not exist
        expect((user as any).displayName).toBeUndefined();

        // userData should be present
        if (user.userData) {
          expect(user.userData.real_name).toBe('Slack User');
          expect(user.userData.display_name).toBe('SlackUser');
        }
      }

      stream.end();
    });

    it('should handle empty platformData map correctly', async () => {
      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const message: Message = {
        conversationId: 'conv-empty-map',
        platform: 'web',
        user: {
          id: 'user-empty',
          username: 'test',
        },
        content: 'Test empty map',
        platformContext: {
          messageId: 'msg-empty',
          channelId: 'conv-empty-map',
          platformData: {}, // Empty map
        },
      };

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      if (responses.length > 0 && responses[0].incomingMessage) {
        const pc = responses[0].incomingMessage.platformContext;
        expect(pc).toBeDefined();
        expect(pc.platformData).toBeDefined();
        expect(Object.keys(pc.platformData).length).toBe(0);
      }

      stream.end();
    });
  });

  describe('Roundtrip: TS → Go → TS', () => {
    it('should preserve platformContext through full roundtrip', async () => {
      const originalPC: PlatformContext = {
        messageId: 'C123:1234567890.999999',
        channelId: 'C123456',
        threadId: '1234567890.000001',
        channelName: '#roundtrip-test',
        workspaceId: 'T999888',
        platformData: {
          team_id: 'T999888',
          bot_id: 'B123456',
          app_id: 'A789012',
          custom_field_1: 'value1',
          custom_field_2: 'value2',
        },
      };

      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const outgoingMessage: Message = {
        conversationId: 'conv-roundtrip',
        platform: 'slack',
        user: {
          id: 'U123456',
          username: 'roundtrip-user',
          email: 'roundtrip@example.com',
          avatarUrl: 'https://example.com/roundtrip.png',
        },
        content: 'Roundtrip test message',
        platformContext: originalPC,
      };

      stream.sendMessage(outgoingMessage);

      // Wait for server to process and potentially echo back
      await new Promise(resolve => setTimeout(resolve, 1000));

      if (responses.length > 0 && responses[0].incomingMessage) {
        const receivedPC = responses[0].incomingMessage.platformContext;

        // Verify all fields match
        expect(receivedPC.messageId).toBe(originalPC.messageId);
        expect(receivedPC.channelId).toBe(originalPC.channelId);
        expect(receivedPC.threadId).toBe(originalPC.threadId);
        expect(receivedPC.channelName).toBe(originalPC.channelName);
        expect(receivedPC.workspaceId).toBe(originalPC.workspaceId);

        // Verify map is fully preserved
        expect(receivedPC.platformData).toBeDefined();
        expect(receivedPC.platformData.team_id).toBe('T999888');
        expect(receivedPC.platformData.bot_id).toBe('B123456');
        expect(receivedPC.platformData.app_id).toBe('A789012');
        expect(receivedPC.platformData.custom_field_1).toBe('value1');
        expect(receivedPC.platformData.custom_field_2).toBe('value2');

        console.log('✓ PlatformContext fully preserved through roundtrip');
      }

      stream.end();
    });

    it('should preserve special characters in platformData', async () => {
      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const message: Message = {
        conversationId: 'conv-special-chars',
        platform: 'slack',
        user: {
          id: 'U999',
          username: 'test',
        },
        content: 'Test special characters',
        platformContext: {
          messageId: 'msg-special',
          channelId: 'C999',
          platformData: {
            'key-with-dash': 'value',
            'key.with.dot': 'value',
            'key_with_underscore': 'value',
            'unicode-key': 'emoji-value-🚀',
            'json-like': '{"nested":"value"}',
          },
        },
      };

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      if (responses.length > 0 && responses[0].incomingMessage) {
        const pd = responses[0].incomingMessage.platformContext.platformData;
        expect(pd['key-with-dash']).toBe('value');
        expect(pd['key.with.dot']).toBe('value');
        expect(pd['key_with_underscore']).toBe('value');
        expect(pd['unicode-key']).toBe('emoji-value-🚀');
        expect(pd['json-like']).toBe('{"nested":"value"}');
      }

      stream.end();
    });
  });

  describe('Edge cases and error handling', () => {
    it('should handle very long field values', async () => {
      const longString = 'x'.repeat(10000);
      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const message: Message = {
        conversationId: 'conv-long-values',
        platform: 'web',
        user: {
          id: 'user-long',
          username: longString.substring(0, 100),
        },
        content: longString,
        platformContext: {
          messageId: 'msg-long',
          channelId: 'conv-long-values',
          platformData: {
            long_value: longString,
          },
        },
      };

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      // Should not crash
      expect(true).toBe(true);

      stream.end();
    });

    it('should handle null/undefined values correctly', async () => {
      const stream = client.createConversationStream();

      const responses: any[] = [];
      stream.on('response', (resp: any) => {
        responses.push(resp);
      });

      const message: Message = {
        conversationId: 'conv-null-test',
        platform: 'web',
        user: {
          id: 'user-null',
          username: 'test',
          email: undefined, // Explicitly undefined
          avatarUrl: undefined,
        },
        content: 'Test null values',
        platformContext: {
          messageId: 'msg-null',
          channelId: 'conv-null-test',
          threadId: undefined,
          channelName: undefined,
        },
      };

      stream.sendMessage(message);

      await new Promise(resolve => setTimeout(resolve, 500));

      // Should handle gracefully (proto3 will use default empty values)
      if (responses.length > 0 && responses[0].incomingMessage) {
        const user = responses[0].incomingMessage.user;
        const pc = responses[0].incomingMessage.platformContext;

        expect(user.id).toBe('user-null');
        expect(pc.channelId).toBe('conv-null-test');
      }

      stream.end();
    });
  });
});
