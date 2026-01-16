import { z } from "zod";
import type { Node } from "./node";

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
