import type { NodeDefinition } from "./node";

export type ConfigValue = string | boolean | number;

export type EngineContext<
  T extends Record<string, unknown> = Record<string, unknown>,
> = {
  output: (outputName: keyof T, data: any) => void;
  outputExternal: (outputName: string, data: any) => void;
  nodeDefinitions: Record<string, NodeDefinition>;
  config: Record<string, ConfigValue>;
};
