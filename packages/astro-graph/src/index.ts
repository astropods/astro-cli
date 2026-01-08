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

/**
 * A compiled graph with phantom types for input/output.
 * Can be used as a module in other graphs.
 *
 * @template TInput - The type the graph accepts as input
 * @template TOutput - The type the graph produces as output
 */
export type CompiledGraph<TInput = unknown, TOutput = unknown> = {
  meta: GraphMeta;
  nodes: Record<string, Node>;
  edges: Record<string, Edge>;
  /** Phantom field to carry input type - doesn't exist at runtime */
  readonly _input?: TInput;
  /** Phantom field to carry output type - doesn't exist at runtime */
  readonly _output?: TOutput;
};

/**
 * Branch builder function for if/else branches.
 * Takes a branch graph and returns it after adding nodes.
 */
export type BranchBuilderFn<TBranchInput, TBranchOutput> = (
  branch: Graph<TBranchInput, TBranchInput>
) => Graph<TBranchInput, TBranchOutput>;

/**
 * Data for an if node with separate types for each branch.
 * The resulting type after the if is TThen | TElse.
 */
export type IfNodeData<TInput, TThen, TElse> = {
  condition: (input: TInput) => boolean | Promise<boolean>;
  then: BranchBuilderFn<TInput, TThen>;
  else: BranchBuilderFn<TInput, TElse>;
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
  [K in keyof typeof NODE_DEFINITIONS as K extends "evaluate"
    ? never
    : (typeof NODE_DEFINITIONS)[K] extends { private: true }
    ? never
    : K]: (
    data: NodeDataType<(typeof NODE_DEFINITIONS)[K]>
  ) => NodeConfig<(typeof NODE_DEFINITIONS)[K]>;
};

/** Bound factories - TIn is pre-bound from the graph's current output type */
type BoundFactories<TIn> = StaticNodeFactories & {
  /** Evaluate factory with input type pre-bound */
  evaluate: <TOut>(data: {
    fn: (input: TIn) => Promise<TOut>;
  }) => NodeConfig<
    NodeDefinition<TIn, { output: TOut }, { fn: EvalFn<TIn, TOut> }>
  >;
};

function createBoundFactories<TIn>(): BoundFactories<TIn> {
  const factories = {} as BoundFactories<TIn>;

  // Auto-generate factories for non-generic, non-private nodes
  for (const [key, definition] of Object.entries(NODE_DEFINITIONS)) {
    if (key === "evaluate" || definition.private) continue;
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
    } as NodeConfig<
      NodeDefinition<TIn, { output: TOut }, { fn: EvalFn<TIn, TOut> }>
    >);

  return factories;
}

/**
 * A chainable builder for constructing graphs.
 * Can represent the root graph or a branch.
 *
 * @template TStart - The type the graph accepts as input (stable through the chain)
 * @template TCurrent - The type at the current position in the chain (changes with each operation)
 */
class Graph<TStart = unknown, TCurrent = TStart> {
  private ctx: GraphContext;
  private lastNodeId: string | null;
  private nextPort: string;

  /** Bound factories with TCurrent pre-bound for type inference */
  private boundFactories = createBoundFactories<TCurrent>();

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

  private createBranch<TBranchStart>(
    fromNodeId: string,
    fromPort: string
  ): Graph<TBranchStart, TBranchStart> {
    return new Graph<TBranchStart, TBranchStart>(
      this.ctx,
      fromNodeId,
      fromPort
    );
  }

  /**
   * Conditional branching node.
   * The output type becomes TThen | TElse (union of both branch outputs).
   *
   * @param data - Condition function and branch builders
   * @param name - Display name for the if node
   * @returns Graph with output type as union of both branches
   */
  if<TThen, TElse>(
    data: IfNodeData<TCurrent, TThen, TElse>,
    name: string
  ): Graph<TStart, TThen | TElse> {
    const { condition, then: thenBuilder, else: elseBuilder } = data;

    const node: Node<typeof node_if> = {
      id: nanoid(8),
      name: name ?? "If",
      type: "if",
      data: { condition },
    };

    this.addNode(node);

    const thenBranch = this.createBranch<TCurrent>(node.id, "then");
    thenBuilder(thenBranch);

    const elseBranch = this.createBranch<TCurrent>(node.id, "else");
    elseBuilder(elseBranch);

    // Return type is union of both branches
    return new Graph<TStart, TThen | TElse>(this.ctx, node.id, "output");
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
      factories: BoundFactories<TCurrent>
    ) => NodeConfig<NodeDefinition<TNodeInput, { output: TOut }, any>>,
    options: TCurrent extends TNodeInput
      ? { name?: string; transform?: (input: TCurrent) => TNodeInput }
      : { name?: string; transform: (input: TCurrent) => TNodeInput }
  ): Graph<TStart, TOut> {
    const config = configFn(this.boundFactories);

    const node: Node = {
      id: nanoid(8),
      name: options?.name ?? config.name,
      type: config.type,
      data: config.data,
    };

    this.addNode(node);

    // Preserve TStart, update TCurrent to TOut
    return new Graph<TStart, TOut>(this.ctx, node.id, "output");
  }

  /**
   * Use another graph as a module within this graph.
   * The module's nodes and edges are inlined, and types flow through.
   * Modules referenced in a graph have their nodes and edges inlined into the parent graph.
   *
   * @param module - A graph to use as a module
   * @returns Graph with output type from the module
   *
   * @example
   * const textProcessor = new Graph<{ text: string }>()
   *   .run((f) => f.evaluate({ fn: async (input) => ({ length: input.text.length }) }));
   *
   * const mainGraph = new Graph<{ rawText: string }>()
   *   .run((f) => f.evaluate({ fn: async (input) => ({ text: input.rawText }) }))
   *   .useModule(textProcessor)
   *   .compile();
   */
  useModule<TModuleInput, TModuleOutput>(
    module: Graph<TModuleInput, TModuleOutput>
  ): TCurrent extends TModuleInput ? Graph<TStart, TModuleOutput> : never {
    // Get the module's compiled representation
    const compiled = module.compile();

    // Find the start node of the module
    const moduleStartNode = Object.values(compiled.nodes).find(
      (n) => n.type === "start"
    );

    if (!moduleStartNode) {
      // In theory this should never happen but just in case ¯\_(ツ)_/¯
      throw new Error("Module must have a start node");
    }

    // Find all nodes after the start node (excluding the start node itself)
    const moduleNodes = Object.values(compiled.nodes).filter(
      (n) => n.type !== "start"
    );

    // Create ID mapping for the module nodes (to avoid conflicts)
    const idMap = new Map<string, string>();
    idMap.set(moduleStartNode.id, this.lastNodeId!); // Map module's start to our current last node

    for (const moduleNode of moduleNodes) {
      const newId = nanoid(8);
      idMap.set(moduleNode.id, newId);

      // Add the node with new ID
      const newNode: Node = {
        ...moduleNode,
        id: newId,
      };
      this.ctx.nodes[newId] = newNode;
    }

    // Add edges with remapped IDs
    for (const edge of Object.values(compiled.edges)) {
      // Skip edges from the module's start node - we'll connect from our last node instead
      const sourceId = idMap.get(edge.source);
      const targetId = idMap.get(edge.target);

      if (!sourceId || !targetId) continue;

      // If this edge is from the module's start node, use our current port
      const sourcePort =
        edge.source === moduleStartNode.id ? this.nextPort : edge.sourcePort;

      const edgeId = nanoid(8);
      this.ctx.edges[edgeId] = {
        source: sourceId,
        target: targetId,
        sourcePort,
        targetPort: edge.targetPort,
      };
    }

    // Find terminal nodes in the module (nodes with no outgoing edges within the module)
    const moduleEdges = Object.values(compiled.edges);
    const nodesWithOutgoing = new Set(moduleEdges.map((e) => e.source));
    const terminalNodes = moduleNodes.filter(
      (n) => !nodesWithOutgoing.has(n.id)
    );

    // Set the last node to be the terminal node of the module
    // (For now, assume single terminal node - could be extended for multiple)
    if (terminalNodes.length > 0) {
      this.lastNodeId = idMap.get(terminalNodes[0].id)!;
    }

    this.nextPort = "output";

    return this as unknown as TCurrent extends TModuleInput
      ? Graph<TStart, TModuleOutput>
      : never;
  }

  /**
   * Set metadata for the graph.
   */
  meta(meta: { title: string; description: string }): Graph<TStart, TCurrent> {
    this.ctx.meta.title = meta.title;
    this.ctx.meta.description = meta.description;
    return this;
  }

  /**
   * Compile the graph into a serializable format.
   * The compiled graph carries type information and can be used as a module.
   *
   * @returns Compiled graph with input type TStart and output type TCurrent
   */
  compile(): CompiledGraph<TStart, TCurrent> {
    return {
      meta: this.ctx.meta,
      nodes: this.ctx.nodes,
      edges: this.ctx.edges,
    };
  }
}

export { Graph };
