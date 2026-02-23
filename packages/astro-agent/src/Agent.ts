import { AgentStep, AgentTool } from "@saswatds/astro-types";
import { Agent, ToolsInput } from "@mastra/core/agent";
import { Mastra } from "@mastra/core/mastra";
import type { ObservabilityEntrypoint } from "@mastra/core/observability";
import { DynamicArgument } from "@mastra/core/types";
import { Engine } from "@saswatds/astro-engine";
import { NODE_DEFINITIONS } from "@saswatds/astro-nodes";
import { Memory } from "@mastra/memory";
import { LibSQLStore } from "@mastra/libsql";
import { Observability, SamplingStrategyType } from "@mastra/observability";
import { OtelExporter } from "@mastra/otel-exporter";
import { createOllama } from "ollama-ai-provider-v2";

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

  private _observability?: ObservabilityEntrypoint;

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

  observability(config: ObservabilityEntrypoint): AstroAgent {
    this._observability = config;
    return this;
  }

  getConfig(): {
    systemPrompt: string;
    tools: {
      name: string;
      title: string;
      description: string;
      type: "graph" | "other";
      graph?: {
        nodes: { id: string; name: string; type: string }[];
        edges: { id: string; source: string; target: string }[];
      };
    }[];
  } {
    const toolInfos = this._tools
      .map((tool) => {
        if (tool.type === "graph") {
          const nodes = Object.entries(tool.graph.nodes).map(([id, node]) => ({
            id,
            name: (node as { name: string }).name,
            type: (node as { type: string }).type,
          }));
          const edges = Object.entries(tool.graph.edges).map(([id, edge]) => ({
            id,
            source: (edge as { source: string }).source,
            target: (edge as { target: string }).target,
          }));

          return {
            name: tool.graph.meta.toolName,
            title: tool.graph.meta.title,
            description:
              tool.graph.meta.toolDescription ?? tool.graph.meta.description,
            type: "graph" as const,
            graph: { nodes, edges },
          };
        }
        return undefined;
      })
      .filter(
        (
          tool
        ): tool is {
          name: string;
          title: string;
          description: string;
          type: "graph";
          graph: {
            nodes: { id: string; name: string; type: string }[];
            edges: { id: string; source: string; target: string }[];
          };
        } => tool !== undefined
      );

    return {
      systemPrompt: this._instructions.join("\n\n"),
      tools: toolInfos,
    };
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
              execute: async (input: Record<string, unknown>) => {
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

  /**
   * Build an Observability config automatically when OTEL_EXPORTER_OTLP_ENDPOINT is set.
   * Returns undefined if the env var is not present.
   */
  private autoObservability(
    serviceName: string
  ): ObservabilityEntrypoint | undefined {
    const exporterUrl = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;
    if (!exporterUrl) return undefined;

    // Mastra's OtelExporter passes the endpoint as the OTel SDK constructor
    // `url` option, which is used as-is (no signal path appended). We must
    // append `/v1/traces` ourselves so the SDK sends to the correct path.
    const tracesUrl = exporterUrl.replace(/\/+$/, "") + "/v1/traces";

    return new Observability({
      configs: {
        otel: {
          sampling: { type: SamplingStrategyType.ALWAYS },
          serviceName,
          exporters: [
            new OtelExporter({
              provider: {
                custom: {
                  endpoint: tracesUrl,
                  protocol: "http/protobuf",
                },
              },
            }),
          ],
        },
      },
    });
  }

  private resolveModel(): Agent["model"] {
    if (typeof this._model === "string" && this._model.startsWith("ollama/")) {
      const modelName = this._model.slice("ollama/".length);
      const baseURL =
        process.env.OLLAMA_BASE_URL || "http://localhost:11434/api";
      const ollama = createOllama({ baseURL });
      return ollama(modelName);
    }
    return this._model;
  }

  private createAgent(): Agent {
    const agentId = this._meta.title || "astro-agent";
    const agent = new Agent({
      id: agentId,
      model: this.resolveModel(),
      name: this._meta.title,
      description: this._meta.description,
      instructions: this._instructions,
      tools: this.convertTools(),
      memory: this.createInMemoryStorage(),
    });

    // Use explicit config if provided, otherwise auto-configure from env
    const obs = this._observability ?? this.autoObservability(agentId);
    if (obs) {
      const rawEndpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;
      const tracesEndpoint = rawEndpoint ? rawEndpoint.replace(/\/+$/, "") + "/v1/traces" : "custom";
      console.log(`📡 Observability enabled for "${agentId}" (source: ${this._observability ? "explicit config" : "auto from OTEL_EXPORTER_OTLP_ENDPOINT"})`);
      console.log(`   ✓ Traces endpoint: ${tracesEndpoint}`);
      console.log(`   ✓ Protocol: http/protobuf`);
      console.log(`   ✓ Sampling: always`);
      new Mastra({
        agents: { [agentId]: agent },
        observability: obs,
      });
    } else {
      console.log(`⚠️ Observability disabled for "${agentId}" — set OTEL_EXPORTER_OTLP_ENDPOINT or call .observability() to enable`);
    }

    return agent;
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

    const configuredMaxSteps = Number(process.env.ASTRO_AGENT_MAX_STEPS || "8");
    const maxSteps = Number.isFinite(configuredMaxSteps) && configuredMaxSteps > 0 ? configuredMaxSteps : 8;

    const stream = await this._agent.stream(config.prompt, {
      memory: {
        resource: config.userId,
        thread: config.threadId
      },
      maxSteps,
    });

    let result = "";
    let sawFinish = false;

    try {
      for await (const chunk of stream.fullStream as AsyncIterable<{ type: string; payload: Record<string, any> }>) {
        if (chunk.type === "tool-call") {
          const tool = this._tools.find((t) => t.type === "graph" && t.graph.meta.toolName === chunk.payload.toolName);

          config.onStepStart?.({
            id: chunk.payload.toolCallId,
            name: tool?.graph.meta.title ?? chunk.payload.toolName,
            type: "tool",
          });
        }

        if (chunk.type === "tool-result") {
          const tool = this._tools.find((t) => t.type === "graph" && t.graph.meta.toolName === chunk.payload.toolName);

          if (tool) {
            config.onStepEnd?.({
              id: chunk.payload.toolCallId,
              name: tool.graph.meta.title ?? chunk.payload.toolName,
              type: "tool",
            });
          }
        }

        if (chunk.type === "text-delta") {
          const text = typeof chunk.payload.text === "string" ? chunk.payload.text : "";
          if (text) {
            config.onChunk?.(text);
            result += text;
          }
        }

        if (chunk.type === "reasoning-start") {
          config.onReasoningStart?.();
        }

        if (chunk.type === "reasoning-delta") {
          const reasoningText = typeof chunk.payload.text === "string" ? chunk.payload.text : "";
          if (reasoningText) {
            config.onReasoningChunk?.(reasoningText);
          }
        }

        if (chunk.type === "reasoning-end") {
          config.onReasoningEnd?.();
        }

        if (chunk.type === "finish") {
          sawFinish = true;
          config.onFinish?.(result);
        }
      }
    } catch (error) {
      config.onError?.(error instanceof Error ? error : new Error(String(error)));
      throw error;
    }

    if (!sawFinish) {
      config.onFinish?.(result);
    }
  }
}
