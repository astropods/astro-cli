import {
  NodeDefinition,
  Node,
  Edge,
  NodeType,
  EngineContext,
  ConfigValue,
} from "astro-types";
import type { CompiledGraph } from "astro-graph";
import { type z } from "zod";

export class Engine<
  TInputSchema extends z.ZodType = z.ZodType,
  TConfigSchema extends z.ZodType = z.ZodType<{}>
> {
  private nodes: Map<string, Node>;
  private edges: Map<string, Edge>;
  private nodeDefs: Record<NodeType, NodeDefinition>;
  private config: Record<string, ConfigValue>;
  private status: "idle" | "running" | "paused" = "idle";
  private cancelled: boolean = false;
  private pendingTasks: Map<string, null> = new Map();
  private runFinishedResolver: (() => void) | null = null;
  private onStartNodeExecute?: (
    nodeId: string,
    inputs: Record<string, unknown>
  ) => void;
  private onNodeExecuted?: (
    nodeId: string,
    outputs: Record<string, unknown>
  ) => void;
  private onNodeExternalOutput?: (outputName: string, data: any) => void;
  private onNodeError?: (nodeId: string, error: string) => void;
  private onEngineFinished?: () => void;

  constructor(
    graph: CompiledGraph<TInputSchema, unknown, TConfigSchema>,
    nodeDefs: Record<NodeType, NodeDefinition>,
    config: z.infer<TConfigSchema>,
    events: {
      onStartNodeExecute?: (
        nodeId: string,
        inputs: Record<string, unknown>
      ) => void;
      onNodeExternalOutput?: (outputName: string, data: any) => void;
      onNodeExecuted?: (
        nodeId: string,
        outputs: Record<string, unknown>
      ) => void;
      onNodeError?: (nodeId: string, error: string) => void;
      onEngineFinished?: () => void;
    }
  ) {
    this.nodes = new Map(Object.entries(graph.nodes));
    this.edges = new Map(Object.entries(graph.edges));
    this.nodeDefs = nodeDefs;
    this.config = config as Record<string, ConfigValue>;
    this.onStartNodeExecute = events.onStartNodeExecute;
    this.onNodeExternalOutput = events.onNodeExternalOutput;
    this.onNodeExecuted = events.onNodeExecuted;
    this.onNodeError = events.onNodeError;
    this.onEngineFinished = events.onEngineFinished;
  }

  public cancel() {
    if (this.status === "running") {
      this.cancelled = true;
      this.status = "idle";
    }
  }

  public getStartNodeId(): string {
    const startNode = Array.from(this.nodes.values()).find(
      (n) => n.type === "start"
    );
    if (!startNode) throw new Error("No start node found");
    return startNode.id;
  }

  public async run(startNodeId: string, input: z.infer<TInputSchema>) {
    return new Promise<void>((resolve, _reject) => {
      this.runFinishedResolver = resolve;

      const startNode = this.nodes.get(startNodeId);
      if (!startNode) {
        throw new Error("Start node not found");
      }

      this.status = "running";
      this.cancelled = false;

      const handleNodeOutput = (
        nodeId: string,
        outputName: string,
        data: any
      ) => {
        const outgoingEdges = this.getOutgoingEdges(nodeId).filter(
          (edge) => edge.sourcePort === outputName
        );

        for (const edge of outgoingEdges) {
          const targetNode = this.nodes.get(edge.target);

          if (!targetNode) return;

          // Pass data directly to the next node (unwrapped)
          this.executeNode(edge.target, data, {
            output: (outputName, d) =>
              handleNodeOutput(edge.target, outputName, d),
            outputExternal: (outputName, d) =>
              this.onNodeExternalOutput?.(outputName, d),
            nodeDefinitions: this.nodeDefs,
            config: this.config,
          });
        }
      };

      // Kick-off the first node with the provided input.
      this.executeNode(startNodeId, input, {
        output: (outputName, data) =>
          handleNodeOutput(startNodeId, outputName, data),
        outputExternal: (outputName, data) =>
          this.onNodeExternalOutput?.(outputName, data),
        nodeDefinitions: this.nodeDefs,
        config: this.config,
      });
    });
  }

  private getOutgoingEdges(nodeId: string) {
    const node = this.nodes.get(nodeId);

    if (!node) {
      throw new Error("Node not found");
    }

    // If is a special system node, all other nodes get an "output" port.
    const nodeOutputs = node.type === "if" ? ["then", "else"] : ["output"];

    return Array.from(this.edges.values()).filter(
      (edge) => edge.source === nodeId && nodeOutputs.includes(edge.sourcePort)
    );
  }

  private getIncomingEdges(nodeId: string) {
    return Array.from(this.edges.values()).filter(
      (edge) => edge.target === nodeId
    );
  }

  private async executeNode(
    nodeId: string,
    input: unknown,
    ctx: EngineContext
  ) {
    if (this.cancelled) {
      return;
    }
    const taskId = crypto.randomUUID();

    this.pendingTasks.set(taskId, null);

    const node = this.nodes.get(nodeId);

    if (!node) {
      throw new Error("Node not found, could not execute node");
    }

    const blockDef = this.nodeDefs[node.type];

    if (!blockDef) {
      throw new Error("Block definition not found, could not execute node");
    }

    try {
      this.onStartNodeExecute?.(nodeId, input as Record<string, unknown>);

      const nodeOutput: Record<string, any> = {};

      const context: EngineContext = {
        ...ctx,
        config: this.config,
        output: (outputName, data) => {
          nodeOutput[outputName] = data;
          ctx.output(outputName, data);
        },
      };

      await blockDef.execute(input, node.data, context);

      if (this.cancelled) {
        return;
      }

      this.onNodeExecuted?.(nodeId, nodeOutput);
    } catch (error) {
      this.onNodeError?.(
        nodeId,
        (error as Error)?.message ??
          "An unknown error occurred while running this block."
      );
    } finally {
      this.pendingTasks.delete(taskId);
      this.checkForPendingTasks();
    }
  }

  private checkForPendingTasks() {
    if (this.pendingTasks.size === 0) {
      this.onEngineFinished?.();
      this.runFinishedResolver?.();
      this.runFinishedResolver = null;
    }
  }
}
