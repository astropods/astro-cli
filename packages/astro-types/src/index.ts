import { z } from "zod";

export type NodeType = "start" | "if" | "evaluate" | "generateText";

export type NodeData = {
  [key: string]: any;
};

/**
 * Valid runtime config value types.
 */
export type ConfigValue = string | boolean | number;

export type EngineContext<
  T extends Record<string, unknown> = Record<string, unknown>,
> = {
  output: (outputName: keyof T, data: any) => void;
  outputExternal: (outputName: string, data: any) => void;
  nodeDefinitions: Record<string, NodeDefinition>;
  config: Record<string, ConfigValue>;
};

export interface NodeDefinition<
  TInput = unknown,
  TOutput extends Record<string, unknown> = Record<string, unknown>,
  TData extends NodeData = Record<string, never>,
> {
  type: NodeType;
  meta: {
    name: string;
    description: string;
  };
  private?: boolean;
  execute: (
    input: TInput,
    data: TData,
    context: EngineContext<TOutput>
  ) => Promise<void>;
}

export type Node<
  ND extends NodeDefinition<any, any, any> = NodeDefinition<any, any, any>,
> = {
  id: string;
  name: string;
  type: ND["type"];
  data: NodeDataType<ND>;
};

export type Edge = {
  source: string;
  target: string;
  sourcePort: string;
  targetPort: string;
};

export type GraphMeta = {
  title: string;
  description: string;
  toolName: string;
  toolDescription: string | null;
};

export type GraphContext = {
  meta: GraphMeta;
  nodes: Record<string, Node>;
  edges: Record<string, Edge>;
};

/**
 * A compiled graph with phantom types for input/output/config.
 * Can be used as a module in other graphs.
 *
 * @template TInputSchema - The Zod object schema type for input validation (must be a z.object())
 * @template TOutput - The type the graph produces as output
 * @template TConfigSchema - The Zod schema type for config validation
 */
export type CompiledGraph<
  TInputSchema extends z.ZodObject<z.ZodRawShape> = z.ZodObject<z.ZodRawShape>,
  TOutput = unknown,
  TConfigSchema extends z.ZodType = z.ZodType<{}>,
> = {
  meta: GraphMeta;
  nodes: Record<string, Node>;
  edges: Record<string, Edge>;
  /** The Zod schema for validating graph input */
  inputSchema: TInputSchema;
  /** The Zod schema for validating graph config */
  configSchema: TConfigSchema;
  /** Phantom field to carry output type - doesn't exist at runtime */
  readonly _output?: TOutput;
};

export type NodeDataType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<any, any, infer TData> ? TData : never;

/** Extract the expected input type from a NodeDefinition */
export type NodeInputType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<infer TInput, any, any> ? TInput : never;

/** Extract the output type from a NodeDefinition */
export type NodeOutputType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<any, infer TOutput, any> ? TOutput : never;

export interface AgentTool {
  type: "graph";
  graph: CompiledGraph;
}

export type AgentStep = {
  id: string;
  name: string;
  type: "tool" | "agent";
};
