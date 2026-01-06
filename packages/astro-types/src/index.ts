export type NodeType = "start" | "if" | "evaluate" | "generate";

export type NodeData = {
  [key: string]: any;
};

export interface EngineContext<TOutput = unknown> {
  output: (outputName: string, data: TOutput) => void;
}

export interface NodeDefinition<
  TInput = unknown,
  TOutput = unknown,
  TData extends NodeData = Record<string, never>
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
  ND extends NodeDefinition<any, any, any> = NodeDefinition<any, any, any>
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

export type NodeDataType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<any, any, infer TData> ? TData : never;

/** Extract the expected input type from a NodeDefinition */
export type NodeInputType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<infer TInput, any, any> ? TInput : never;

/** Extract the output type from a NodeDefinition */
export type NodeOutputType<ND extends NodeDefinition<any, any, any>> =
  ND extends NodeDefinition<any, infer TOutput, any> ? TOutput : never;
