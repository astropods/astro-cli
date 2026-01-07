import type {
  Edge,
  Node,
  NodeDataType,
  NodeDefinition,
  NodeInputType,
  NodeOutputType,
} from "astro-types";
import {
  type EvalFn,
  NODE_DEFINITIONS,
  node_if,
  node_start,
} from "astro-nodes";
import { nanoid } from "nanoid";

type GraphMeta = {
  title: string;
  description: string;
};

type GraphContext = {
  meta: GraphMeta;
  nodes: Record<string, Node>;
  edges: Record<string, Edge>;
};

export type BranchBuilderFn<TInput, TOutput> = (
  branch: Graph<TInput>
) => Graph<TOutput>;

export type IfNodeData<TInput, TOutput> = {
  condition: (input: TInput) => boolean | Promise<boolean>;
  then: BranchBuilderFn<TInput, TOutput>;
  else: BranchBuilderFn<TInput, TOutput>;
};

export type NodeConfig<
  ND extends NodeDefinition<any, any, any> = NodeDefinition
> = {
  type: ND["type"];
  name: string;
  data: NodeDataType<ND>;
  /** Phantom field to carry input type - doesn't exist at runtime */
  readonly _input?: NodeInputType<ND>;
  /** Phantom field to carry output type - doesn't exist at runtime */
  readonly _output?: NodeOutputType<ND>;
};

/** Factories for non-generic nodes (input type doesn't matter) */
type StaticNodeFactories = {
  [K in keyof Omit<typeof NODE_DEFINITIONS, "evaluate">]: (
    data: NodeDataType<(typeof NODE_DEFINITIONS)[K]>
  ) => NodeConfig<(typeof NODE_DEFINITIONS)[K]>;
};

/** Bound factories - TIn is pre-bound from the graph's current output type */
type BoundFactories<TIn> = StaticNodeFactories & {
  /** Evaluate factory with input type pre-bound */
  evaluate: <TOut>(data: {
    fn: (input: TIn) => Promise<TOut>;
  }) => NodeConfig<NodeDefinition<TIn, TOut, { fn: EvalFn<TIn, TOut> }>>;
};

function createBoundFactories<TIn>(): BoundFactories<TIn> {
  const factories = {} as BoundFactories<TIn>;

  // Auto-generate factories for non-generic nodes
  for (const [key, definition] of Object.entries(NODE_DEFINITIONS)) {
    if (key === "evaluate") continue;
    (factories as any)[key] = (data: any) => ({
      type: definition.type,
      name: definition.meta.name,
      data,
    });
  }

  // Evaluate factory with TIn pre-bound
  factories.evaluate = <TOut>(data: { fn: (input: TIn) => Promise<TOut> }) =>
    ({
      type: "evaluate" as const,
      name: "Evaluate",
      data,
    } as NodeConfig<NodeDefinition<TIn, TOut, { fn: EvalFn<TIn, TOut> }>>);

  return factories;
}

/**
 * A chainable builder for constructing graphs.
 * Can represent the root graph or a branch.
 *
 * @template TInput - The data that will be passed into the start node (or the first node in a branch)
 */
class Graph<TInput = unknown> {
  private ctx: GraphContext;
  private lastNodeId: string | null;
  private nextPort: string;

  /** Bound factories with TInput pre-bound for type inference */
  private boundFactories = createBoundFactories<TInput>();

  constructor(
    ctx: GraphContext = {
      meta: { title: "", description: "" },
      nodes: {},
      edges: {},
    },
    startNodeId: string | null = null,
    initialPort: string = "output"
  ) {
    this.ctx = ctx;
    this.lastNodeId = startNodeId;
    this.nextPort = initialPort;

    // For top-level graphs, add a start node
    if (startNodeId === null) {
      const startNode: Node<typeof node_start> = {
        id: nanoid(8),
        name: "Start",
        type: "start",
        data: {},
      };
      this.ctx.nodes[startNode.id] = startNode;
      this.lastNodeId = startNode.id;
    }
  }

  private addNode(node: Node): void {
    this.ctx.nodes[node.id] = node;

    if (this.lastNodeId) {
      const edgeId = nanoid(8);
      this.ctx.edges[edgeId] = {
        source: this.lastNodeId,
        target: node.id,
        sourcePort: this.nextPort,
        targetPort: "input",
      };
    }

    this.lastNodeId = node.id;
    this.nextPort = "output";
  }

  private createBranch<T>(fromNodeId: string, fromPort: string): Graph<T> {
    return new Graph<T>(this.ctx, fromNodeId, fromPort);
  }

  // If is a special system node for control flow.
  if<TOutput>(data: IfNodeData<TInput, TOutput>, name: string): Graph<TOutput> {
    const { condition, then: thenBuilder, else: elseBuilder } = data;

    const node: Node<typeof node_if> = {
      id: nanoid(8),
      name: name ?? "If",
      type: "if",
      data: { condition },
    };

    this.addNode(node);

    const thenBranch = this.createBranch<TInput>(node.id, "then");
    thenBuilder(thenBranch);

    const elseBranch = this.createBranch<TInput>(node.id, "else");
    elseBuilder(elseBranch);

    return new Graph<TOutput>(this.ctx, node.id, "output");
  }

  /**
   * Run a node in the graph.
   *
   * @param configFn - Callback that receives bound factories and returns a node config.
   *                   The factories have the graph's current output type pre-bound for inference.
   * @param options - Name and optional transform. Transform is REQUIRED if the node's
   *                  expected input type doesn't match the graph's current output type.
   *
   * @example
   * // If the incoming input data doesn't match the node's expected input type, transform is required:
   * graph.run(
   *   (f) => f.generate({ model: "openai:4" }),
   *   { name: "Generate", transform: (input) => ({ prompt: input.text }) }
   * );
   *
   * // If the incoming input data matches the node's expected input type, transform is optional:
   * graph.run(
   *   (f) => f.evaluate({ fn: async (input) => ({ length: input.text.length }) }),
   *   { name: "Count Characters" }
   * );
   */
  run<TNodeInput, TOut>(
    configFn: (
      factories: BoundFactories<TInput>
    ) => NodeConfig<NodeDefinition<TNodeInput, TOut, any>>,
    options: TInput extends TNodeInput
      ? { name?: string; transform?: (input: TInput) => TNodeInput }
      : { name?: string; transform: (input: TInput) => TNodeInput }
  ): Graph<TOut> {
    const config = configFn(this.boundFactories);

    const node: Node = {
      id: nanoid(8),
      name: options?.name ?? config.name,
      type: config.type,
      data: config.data,
    };

    this.addNode(node);

    return new Graph<TOut>(this.ctx, node.id, "output");
  }

  meta(meta: { title: string; description: string }) {
    this.ctx.meta.title = meta.title;
    this.ctx.meta.description = meta.description;
    return this;
  }

  compile() {
    return {
      meta: this.ctx.meta,
      nodes: this.ctx.nodes,
      edges: this.ctx.edges,
    };
  }
}

export { Graph };
