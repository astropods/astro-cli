import { AgentStep, AgentTool } from "astro-types";
import { Agent, ToolsInput } from "@mastra/core/agent";
import { DynamicArgument } from "@mastra/core/types";
import { Engine } from "astro-engine";
import { NODE_DEFINITIONS } from "astro-nodes";
import { Memory } from "@mastra/memory";
import { LibSQLStore } from "@mastra/libsql";

type AgentMeta = {
  title: string;
  description: string;
};

export class AstroAgent {
  private _meta: AgentMeta = { title: "", description: "" };

  private _model: Agent["model"] = "openai/gpt-5";

  private _instructions: string[] = [];

  private _tools: AgentTool[] = [];

  private _agent: Agent | null = null;

  model(model: Agent["model"]): AstroAgent {
    this._model = model;
    return this;
  }

  instructions(prompt: string): AstroAgent {
    this._instructions.push(prompt);
    return this;
  }

  tool(tool: AgentTool): AstroAgent {
    this._tools.push(tool);
    return this;
  }

  meta(meta: AgentMeta): AstroAgent {
    this._meta = meta;
    return this;
  }

  private createInMemoryStorage(): Memory {
    return new Memory({
      storage: new LibSQLStore({
        id: "memory",
        url: ":memory:",
      }),
    });
  }

  private convertTools(): DynamicArgument<ToolsInput> {
    const convertedTools = this._tools
      .map<[string, ToolsInput[string]] | undefined>((tool) => {
        if (tool.type === "graph") {
          return [
            tool.graph.meta.toolName,
            {
              name: tool.graph.meta.toolName,
              description:
                tool.graph.meta.toolDescription ?? tool.graph.meta.description,
              inputSchema: tool.graph.inputSchema,
              execute: async (input) => {
                const engine = new Engine(tool.graph, NODE_DEFINITIONS, {}, {});
                return engine.run(engine.getStartNodeId(), input);
              },
            },
          ];
        }
        return undefined;
      })
      .filter(
        (tool): tool is [string, ToolsInput[string]] => tool !== undefined
      );

    return Object.fromEntries(convertedTools);
  }

  private createAgent(): Agent {
    return new Agent({
      id: "",
      model: this._model,
      name: this._meta.title,
      description: this._meta.description,
      instructions: this._instructions,
      tools: this.convertTools(),
      memory: this.createInMemoryStorage(),
    });
  }

  async stream(config: {
    prompt: string;
    threadId: string;
    userId: string;
    model?: Agent["model"];
    onChunk?: (chunk: string) => void;
    onStepStart?: (step: AgentStep) => void;
    onStepEnd?: (step: AgentStep) => void;
    onReasoningStart?: () => void;
    onReasoningChunk?: (chunk: string) => void;
    onReasoningEnd?: () => void;
    onError?: (error: Error) => void;
    onFinish?: (result: string) => void;
  }): Promise<void> {
    // Recreate agent if model changed or not initialized
    const targetModel = config.model || this._model;
    if (!this._agent || (config.model && config.model !== this._model)) {
      this._model = targetModel;
      this._agent = this.createAgent();
    }

    const stream = await this._agent.stream(config.prompt, {
      threadId: config.threadId,
      resourceId: config.userId,
    });

    let result = "";
    let reasoning = "";

    for await (const chunk of stream.fullStream) {
      if (chunk.type === "tool-call") {
        const tool = this._tools.find((t) => {
          if (t.type === "graph") {
            return t.graph.meta.toolName === chunk.payload.toolName;
          }
          return false;
        });

        config.onStepStart?.({
          id: chunk.payload.toolCallId,
          name: tool?.graph.meta.title ?? chunk.payload.toolName,
          type: "tool",
        });
      }

      if (chunk.type === "tool-result") {
        const tool = this._tools.find((t) => {
          if (t.type === "graph") {
            return t.graph.meta.toolName === chunk.payload.toolName;
          }
          return false;
        });

        if (tool) {
          config.onStepEnd?.({
            id: chunk.payload.toolCallId,
            name: tool.graph.meta.title ?? chunk.payload.toolName,
            type: "tool",
          });
        }
      }

      if (chunk.type === "text-delta") {
        config.onChunk?.(chunk.payload.text);
        result += chunk.payload.text;
      }

      if (chunk.type === "reasoning-start") {
        config.onReasoningStart?.();
        reasoning = "";
      }

      if (chunk.type === "reasoning-delta") {
        config.onReasoningChunk?.(chunk.payload.text);
        reasoning += chunk.payload.text;
      }

      if (chunk.type === "reasoning-end") {
        config.onReasoningEnd?.();
      }

      if (chunk.type === "finish") {
        config.onFinish?.(result);
      }
    }
  }
}
