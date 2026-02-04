import type { NodeDefinition, NodeType } from "@saswatds/astro-types";

export type EvalFn<TInput = unknown, TOutput = unknown, TConfig = unknown> = (
  input: TInput,
  config: TConfig
) => Promise<TOutput>;

export type ConditionFn<TInput = unknown, TConfig = unknown> = (
  input: TInput,
  config: TConfig
) => boolean | Promise<boolean>;

// #region Node Definitions

export const node_start: NodeDefinition<
  any,
  { output: any },
  Record<string, never>
> = {
  type: "start",
  meta: {
    name: "Start",
    description: "The start node of the graph.",
  },
  private: true,
  execute: async (input, data, ctx) => {
    ctx.output("output", input);
  },
};

export const node_evaluate: NodeDefinition<
  any,
  { output: any },
  { fn: EvalFn<any, any, any> }
> = {
  type: "evaluate",
  meta: {
    name: "Evaluate",
    description: "Evaluates a function and returns the result.",
  },
  execute: async (input, data, ctx) => {
    const result = await data.fn(input, ctx.config);
    ctx.output("output", result);
  },
};

export const node_generate_text: NodeDefinition<
  { prompt: string; system?: string },
  { output: string | object },
  { model: string; schema?: any }
> = {
  type: "generateText",
  meta: {
    name: "Generate",
    description: "Generates a response using a model.",
  },
  execute: async (input, data, ctx) => {
    // TODO: Implement AI generation logic here.
    const result = "Implement me";
    ctx.output("output", result);
  },
};

export const node_if: NodeDefinition<
  any,
  { then: any; else: any },
  { condition: ConditionFn<any, any> }
> = {
  type: "if",
  meta: {
    name: "If",
    description: "Conditionally executes a branch.",
  },
  private: true,
  execute: async (input, data, ctx) => {
    const result = await data.condition(input, ctx.config);
    if (result) {
      ctx.output("then", input);
    } else {
      ctx.output("else", input);
    }
  },
};

// #endregion

export const NODE_DEFINITIONS: Record<
  NodeType,
  NodeDefinition<any, any, any>
> = {
  start: node_start,
  evaluate: node_evaluate,
  generateText: node_generate_text,
  if: node_if,
};
