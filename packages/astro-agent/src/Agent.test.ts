import { describe, test, expect, mock, beforeEach } from "bun:test";
import { Graph, z } from "astro-graph";
import type { AgentStep, AgentTool } from "astro-types";

// Track captured configs and stream calls
let capturedAgentConfig: any = null;
let streamCallArgs: any = null;
let mockStreamGenerator: AsyncGenerator<any, void, unknown> | null = null;

// Helper to set up what the mock stream yields
function setMockStreamChunks(chunks: any[]) {
  mockStreamGenerator = (async function* () {
    for (const chunk of chunks) {
      yield chunk;
    }
  })();
}

// Default chunks
const defaultChunks = [
  { type: "text-delta", payload: { text: "Hello" } },
  { type: "text-delta", payload: { text: " world" } },
  { type: "finish", payload: {} },
];

// Mock the Agent class and other exports from @mastra/core/agent
mock.module("@mastra/core/agent", () => ({
  Agent: class MockAgent {
    constructor(config: any) {
      capturedAgentConfig = config;
    }
    stream(prompt: string, options: any) {
      streamCallArgs = { prompt, options };
      return {
        fullStream:
          mockStreamGenerator ??
          (async function* () {
            for (const chunk of defaultChunks) {
              yield chunk;
            }
          })(),
      };
    }
  },
  // Must include all exports that the real module provides
  MessageList: class MockMessageList {},
  TripWire: class MockTripWire {},
  TypeDetector: class MockTypeDetector {},
  isSupportedLanguageModel: () => true,
  resolveThreadIdFromArgs: () => null,
  supportedLanguageModelSpecifications: [],
  tryGenerateWithJsonFallback: async () => ({}),
  tryStreamWithJsonFallback: async () => ({}),
  aiV5ModelMessageToV2PromptMessage: () => ({}),
  convertMessages: () => [],
}));

mock.module("@mastra/memory", () => ({
  Memory: class MockMemory {
    constructor() {}
  },
}));

// Must import AstroAgent after mocking
const { AstroAgent } = await import("./Agent");

describe("AstroAgent", () => {
  beforeEach(() => {
    capturedAgentConfig = null;
    streamCallArgs = null;
    mockStreamGenerator = null;
  });

  describe("builder pattern", () => {
    test("returns itself for chaining with model()", () => {
      const agent = new AstroAgent();
      const result = agent.model("openai/gpt-4o");
      expect(result).toBe(agent);
    });

    test("returns itself for chaining with instructions()", () => {
      const agent = new AstroAgent();
      const result = agent.instructions("You are a helpful assistant");
      expect(result).toBe(agent);
    });

    test("returns itself for chaining with tool()", () => {
      const agent = new AstroAgent();
      const mockTool: AgentTool = {
        type: "graph",
        graph: new Graph(z.object({ input: z.string() }))
          .meta({
            title: "Test Tool",
            description: "A test tool",
          })
          .compile(),
      };
      const result = agent.tool(mockTool);
      expect(result).toBe(agent);
    });

    test("returns itself for chaining with meta()", () => {
      const agent = new AstroAgent();
      const result = agent.meta({
        title: "Test Agent",
        description: "A test agent",
      });
      expect(result).toBe(agent);
    });

    test("supports fluent API chaining", () => {
      const mockTool: AgentTool = {
        type: "graph",
        graph: new Graph(z.object({ input: z.string() }))
          .meta({
            title: "Test Tool",
            description: "A test tool",
          })
          .compile(),
      };

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .instructions("You are helpful")
        .instructions("Be concise")
        .tool(mockTool)
        .meta({ title: "My Agent", description: "Description" });

      expect(agent).toBeInstanceOf(AstroAgent);
    });
  });

  describe("stream()", () => {
    test("streams text chunks and calls onChunk callback", async () => {
      const chunks: string[] = [];

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
        onChunk: (chunk) => chunks.push(chunk),
      });

      expect(chunks).toEqual(["Hello", " world"]);
    });

    test("calls onFinish with the complete result", async () => {
      let finalResult = "";

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
        onFinish: (result) => {
          finalResult = result;
        },
      });

      expect(finalResult).toBe("Hello world");
    });

    test("handles tool-call events and calls onStepStart", async () => {
      const steps: AgentStep[] = [];

      // Note: Graph.meta() auto-generates toolName from title as snake_case
      // "Search Tool" becomes "search_tool"
      const mockToolGraph = new Graph(z.object({ query: z.string() }))
        .meta({
          title: "Search Tool",
          description: "Searches the web",
        })
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => ({ results: [] }),
            }),
          { name: "Search" }
        )
        .compile();

      setMockStreamChunks([
        {
          type: "tool-call",
          payload: { toolCallId: "call-1", toolName: "search_tool" },
        },
        { type: "finish", payload: {} },
      ]);

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Search for something",
        threadId: "thread-1",
        userId: "user-1",
        onStepStart: (step) => steps.push(step),
      });

      expect(steps).toHaveLength(1);
      expect(steps[0]).toEqual({
        id: "call-1",
        name: "Search Tool",
        type: "tool",
      });
    });

    test("handles tool-result events and calls onStepEnd", async () => {
      const endedSteps: AgentStep[] = [];

      const mockToolGraph = new Graph(z.object({ query: z.string() }))
        .meta({
          title: "Search Tool",
          description: "Searches the web",
        })
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => ({ results: [] }),
            }),
          { name: "Search" }
        )
        .compile();

      setMockStreamChunks([
        {
          type: "tool-result",
          payload: {
            toolCallId: "call-1",
            toolName: "search_tool",
            result: { results: [] },
          },
        },
        { type: "finish", payload: {} },
      ]);

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Search for something",
        threadId: "thread-1",
        userId: "user-1",
        onStepEnd: (step) => endedSteps.push(step),
      });

      expect(endedSteps).toHaveLength(1);
      expect(endedSteps[0]).toEqual({
        id: "call-1",
        name: "Search Tool",
        type: "tool",
      });
    });

    test("handles reasoning events", async () => {
      let reasoningStarted = false;
      const reasoningChunks: string[] = [];
      let reasoningEnded = false;

      setMockStreamChunks([
        { type: "reasoning-start", payload: {} },
        { type: "reasoning-delta", payload: { text: "Let me think" } },
        { type: "reasoning-delta", payload: { text: "..." } },
        { type: "reasoning-end", payload: {} },
        { type: "text-delta", payload: { text: "Done" } },
        { type: "finish", payload: {} },
      ]);

      const agent = new AstroAgent()
        .model("openai/o1")
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Think about this",
        threadId: "thread-1",
        userId: "user-1",
        onReasoningStart: () => {
          reasoningStarted = true;
        },
        onReasoningChunk: (chunk) => reasoningChunks.push(chunk),
        onReasoningEnd: () => {
          reasoningEnded = true;
        },
      });

      expect(reasoningStarted).toBe(true);
      expect(reasoningChunks).toEqual(["Let me think", "..."]);
      expect(reasoningEnded).toBe(true);
    });

    test("works without any callbacks", async () => {
      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .meta({ title: "Test", description: "Test" });

      // Should not throw
      await expect(
        agent.stream({
          prompt: "Hello",
          threadId: "thread-1",
          userId: "user-1",
        })
      ).resolves.toBeUndefined();
    });
  });

  describe("tool conversion", () => {
    test("converts graph tools to Mastra tool format", async () => {
      // "WebSearch" becomes "websearch"
      const mockToolGraph = new Graph(z.object({ query: z.string() }))
        .meta({
          title: "WebSearch",
          description: "Searches the web",
          toolDescription: "Use this to search the internet",
        })
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => ({ found: true }),
            }),
          { name: "Search" }
        )
        .compile();

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Search for cats",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig).not.toBeNull();
      expect(capturedAgentConfig.tools).toBeDefined();
      expect(capturedAgentConfig.tools.websearch).toBeDefined();
      expect(capturedAgentConfig.tools.websearch.name).toBe("websearch");
      expect(capturedAgentConfig.tools.websearch.description).toBe(
        "Use this to search the internet"
      );
    });

    test("uses graph description as fallback for tool description", async () => {
      const mockToolGraph = new Graph(z.object({ query: z.string() }))
        .meta({
          title: "WebSearch",
          description: "Default description from graph",
          // no toolDescription provided
        })
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => ({ found: true }),
            }),
          { name: "Search" }
        )
        .compile();

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Search",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.tools.websearch.description).toBe(
        "Default description from graph"
      );
    });

    test("handles multiple tools", async () => {
      // "Search" becomes "search", "Calculator" becomes "calculator"
      const searchTool = new Graph(z.object({ query: z.string() }))
        .meta({
          title: "Search",
          description: "Search the web",
        })
        .compile();

      const calculateTool = new Graph(
        z.object({ a: z.number(), b: z.number() })
      )
        .meta({
          title: "Calculator",
          description: "Perform calculations",
        })
        .compile();

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: searchTool })
        .tool({ type: "graph", graph: calculateTool })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Help me",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(Object.keys(capturedAgentConfig.tools)).toHaveLength(2);
      expect(capturedAgentConfig.tools.search).toBeDefined();
      expect(capturedAgentConfig.tools.calculator).toBeDefined();
    });

    test("tool execute function runs the graph engine", async () => {
      let toolExecuted = false;
      let receivedInput: any = null;

      const mockToolGraph = new Graph(z.object({ value: z.number() }))
        .meta({
          title: "Double",
          description: "Doubles a number",
        })
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => {
                toolExecuted = true;
                receivedInput = input;
                return { result: input.value * 2 };
              },
            }),
          { name: "Double" }
        )
        .compile();

      const agent = new AstroAgent()
        .model("openai/gpt-4o")
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Double 5",
        threadId: "thread-1",
        userId: "user-1",
      });

      // Get the execute function from the captured config and call it
      await capturedAgentConfig.tools.double.execute({
        value: 5,
      });

      // Verify the tool function was called with correct input
      expect(toolExecuted).toBe(true);
      expect(receivedInput).toEqual({ value: 5 });
    });
  });

  describe("agent configuration", () => {
    test("passes correct config to underlying Agent", async () => {
      const agent = new AstroAgent()
        .model("anthropic/claude-3-opus")
        .instructions("Be helpful")
        .instructions("Be concise")
        .meta({ title: "My Agent", description: "A helpful assistant" });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.model).toBe("anthropic/claude-3-opus");
      expect(capturedAgentConfig.name).toBe("My Agent");
      expect(capturedAgentConfig.description).toBe("A helpful assistant");
      expect(capturedAgentConfig.instructions).toEqual([
        "Be helpful",
        "Be concise",
      ]);
      expect(capturedAgentConfig.memory).toBeDefined();
    });

    test("uses default model when not specified", async () => {
      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.model).toBe("openai/gpt-5");
    });

    test("passes threadId and resourceId to stream call", async () => {
      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      await agent.stream({
        prompt: "Hello there",
        threadId: "my-thread-123",
        userId: "user-456",
      });

      expect(streamCallArgs.prompt).toBe("Hello there");
      expect(streamCallArgs.options.memory.thread).toBe("my-thread-123");
      expect(streamCallArgs.options.memory.resource).toBe("user-456");
    });
  });

  describe("edge cases", () => {
    test("handles empty instructions array", async () => {
      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.instructions).toEqual([]);
    });

    test("handles empty tools array", async () => {
      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.tools).toEqual({});
    });

    test("handles tool-result for non-existent tool gracefully", async () => {
      const endedSteps: AgentStep[] = [];

      setMockStreamChunks([
        {
          type: "tool-result",
          payload: {
            toolCallId: "call-1",
            toolName: "nonExistentTool",
            result: {},
          },
        },
        { type: "finish", payload: {} },
      ]);

      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      // Should not throw and should not call onStepEnd for non-existent tool
      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
        onStepEnd: (step) => endedSteps.push(step),
      });

      expect(endedSteps).toHaveLength(0);
    });

    test("handles tool-call for non-existent tool with fallback name", async () => {
      const startedSteps: AgentStep[] = [];

      setMockStreamChunks([
        {
          type: "tool-call",
          payload: { toolCallId: "call-1", toolName: "unknownTool" },
        },
        { type: "finish", payload: {} },
      ]);

      const agent = new AstroAgent().meta({
        title: "Test",
        description: "Test",
      });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
        onStepStart: (step) => startedSteps.push(step),
      });

      // Should use toolName as fallback when tool not found
      expect(startedSteps).toHaveLength(1);
      expect(startedSteps[0].name).toBe("unknownTool");
    });

    test("accumulates multiple instruction calls", async () => {
      const agent = new AstroAgent()
        .instructions("First instruction")
        .instructions("Second instruction")
        .instructions("Third instruction")
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.instructions).toEqual([
        "First instruction",
        "Second instruction",
        "Third instruction",
      ]);
    });

    test("meta can be overwritten", async () => {
      const agent = new AstroAgent()
        .meta({ title: "First Title", description: "First Desc" })
        .meta({ title: "Second Title", description: "Second Desc" });

      await agent.stream({
        prompt: "Hello",
        threadId: "thread-1",
        userId: "user-1",
      });

      expect(capturedAgentConfig.name).toBe("Second Title");
      expect(capturedAgentConfig.description).toBe("Second Desc");
    });

    test("tool with input schema is used correctly", async () => {
      const mockToolGraph = new Graph(
        z.object({
          query: z.string(),
          maxResults: z.number().optional(),
        })
      )
        .meta({
          title: "Search",
          description: "Search tool",
        })
        .compile();

      const agent = new AstroAgent()
        .tool({ type: "graph", graph: mockToolGraph })
        .meta({ title: "Test", description: "Test" });

      await agent.stream({
        prompt: "Search",
        threadId: "thread-1",
        userId: "user-1",
      });

      // Check that the inputSchema is passed through
      expect(capturedAgentConfig.tools.search.inputSchema).toBeDefined();
    });
  });
});
