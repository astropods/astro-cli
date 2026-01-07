import type { NodeDefinition } from "astro-types";

export type EvalFn<TInput = unknown, TOutput = unknown> = (
  input: TInput
) => Promise<TOutput>;

export type ConditionFn<TInput = unknown> = (
  input: TInput
) => boolean | Promise<boolean>;

// #region Node Definitions

export const node_start: NodeDefinition<any, any, Record<string, never>> = {
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

export const node_evaluate: NodeDefinition<any, any, { fn: EvalFn<any, any> }> =
  {
    type: "evaluate",
    meta: {
      name: "Evaluate",
      description: "Evaluates a function and returns the result.",
    },
    execute: async (input, data, ctx) => {
      const result = await data.fn(input);
      ctx.output("output", result);
    },
  };

export const node_generate_text: NodeDefinition<
  { prompt: string; system?: string },
  string | object,
  { model: string; schema?: any }
> = {
  type: "generate",
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
  any,
  { condition: ConditionFn<any> }
> = {
  type: "if",
  meta: {
    name: "If",
    description: "Conditionally executes a branch.",
  },
  private: true,
  execute: async (input, data, ctx) => {
    const result = await data.condition(input);
    if (result) {
      ctx.output("then", result);
    } else {
      ctx.output("else", result);
    }
  },
};

// #endregion

export const NODE_DEFINITIONS = {
  start: node_start,
  evaluate: node_evaluate,
  generateText: node_generate_text,
  if: node_if,
};
