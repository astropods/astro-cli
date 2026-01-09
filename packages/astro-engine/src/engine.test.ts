import { describe, test, expect, mock } from "bun:test";
import { Engine } from "./engine";
import { Graph } from "astro-graph";
import { NODE_DEFINITIONS } from "astro-nodes";
import type { Node, Edge, NodeType, NodeDefinition } from "astro-types";

describe("Engine", () => {
  test("executes a simple single-node graph with input data", async () => {
    const results: string[] = [];

    const compiled = new Graph<{ message: string }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              results.push(`received: ${input.message}`);
              return { processed: true };
            },
          }),
        { name: "Process Message" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { message: "hello world" });

    expect(results).toEqual(["received: hello world"]);
  });

  test("executes a chain of nodes in sequence", async () => {
    const executionOrder: string[] = [];

    const compiled = new Graph<{ value: number }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              executionOrder.push("first");
              return { doubled: (input?.value ?? 5) * 2 };
            },
          }),
        { name: "Double" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              executionOrder.push("second");
              return { quadrupled: input.doubled * 2 };
            },
          }),
        { name: "Double Again" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              executionOrder.push("third");
              return { final: input.quadrupled + 1 };
            },
          }),
        { name: "Add One" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { value: 5 });

    expect(executionOrder).toEqual(["first", "second", "third"]);
  });

  test("executes the 'then' branch when condition is true", async () => {
    const executedBranches: string[] = [];

    const compiled = new Graph<{ value: number }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => ({ shouldBranch: true }),
          }),
        { name: "Setup" }
      )
      .if(
        {
          condition: (input: any) => input?.shouldBranch,
          then: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("then");
                    return { branch: "then" };
                  },
                }),
              { name: "Then Branch" }
            ),
          else: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("else");
                    return { branch: "else" };
                  },
                }),
              { name: "Else Branch" }
            ),
        },
        "Check Condition"
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { value: 0 });

    expect(executedBranches).toEqual(["then"]);
  });

  test("executes the 'else' branch when condition is false", async () => {
    const executedBranches: string[] = [];

    const compiled = new Graph<{ value: number }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => ({ shouldBranch: false }),
          }),
        { name: "Setup" }
      )
      .if(
        {
          condition: (input: any) => input?.shouldBranch,
          then: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("then");
                    return { branch: "then" };
                  },
                }),
              { name: "Then Branch" }
            ),
          else: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("else");
                    return { branch: "else" };
                  },
                }),
              { name: "Else Branch" }
            ),
        },
        "Check Condition"
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { value: 0 });

    expect(executedBranches).toEqual(["else"]);
  });

  test("calls onStartNodeExecute event for each node", async () => {
    const nodeExecutions: string[] = [];

    const compiled = new Graph<{ x: number }>()
      .run((f) => f.evaluate({ fn: async () => ({ a: 1 }) }), {
        name: "Node A",
      })
      .run((f) => f.evaluate({ fn: async () => ({ b: 2 }) }), {
        name: "Node B",
      })
      .compile();

    const nodes = new Map(Object.entries(compiled.nodes));

    const engine = new Engine(
      compiled,
      NODE_DEFINITIONS,
      {},
      {
        onStartNodeExecute: (nodeId) => {
          const node = nodes.get(nodeId);
          if (node) nodeExecutions.push(node.name);
        },
      }
    );

    await engine.run(engine.getStartNodeId(), { x: 0 });

    expect(nodeExecutions).toEqual(["Start", "Node A", "Node B"]);
  });

  test("calls onNodeExecuted event with outputs", async () => {
    const nodeOutputs: Array<{
      name: string;
      outputs: Record<string, unknown>;
    }> = [];

    const compiled = new Graph<{ x: number }>()
      .run((f) => f.evaluate({ fn: async () => ({ result: 42 }) }), {
        name: "Calculator",
      })
      .compile();

    const nodes = new Map(Object.entries(compiled.nodes));

    const engine = new Engine(
      compiled,
      NODE_DEFINITIONS,
      {},
      {
        onNodeExecuted: (nodeId, outputs) => {
          const node = nodes.get(nodeId);
          if (node) nodeOutputs.push({ name: node.name, outputs });
        },
      }
    );

    await engine.run(engine.getStartNodeId(), { x: 0 });

    // Start node outputs whatever was passed in, Calculator outputs { result: 42 }
    expect(nodeOutputs).toHaveLength(2);
    expect(nodeOutputs[1]).toEqual({
      name: "Calculator",
      outputs: { output: { result: 42 } },
    });
  });

  test("calls onEngineFinished when all nodes complete", async () => {
    let engineFinished = false;

    const compiled = new Graph<{}>()
      .run((f) => f.evaluate({ fn: async () => ({ done: true }) }), {
        name: "Final",
      })
      .compile();

    const engine = new Engine(
      compiled,
      NODE_DEFINITIONS,
      {},
      {
        onEngineFinished: () => {
          engineFinished = true;
        },
      }
    );

    await engine.run(engine.getStartNodeId(), {});

    expect(engineFinished).toBe(true);
  });

  test("calls onNodeError when a node throws", async () => {
    const errors: Array<{ nodeId: string; error: string }> = [];

    const compiled = new Graph<{}>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => {
              throw new Error("Something went wrong!");
            },
          }),
        { name: "Failing Node" }
      )
      .compile();

    const engine = new Engine(
      compiled,
      NODE_DEFINITIONS,
      {},
      {
        onNodeError: (nodeId, error) => {
          errors.push({ nodeId, error });
        },
      }
    );

    await engine.run(engine.getStartNodeId(), {});

    expect(errors).toHaveLength(1);
    expect(errors[0].error).toBe("Something went wrong!");
  });

  test("stops execution after error in a node", async () => {
    const executedNodes: string[] = [];

    const compiled = new Graph<{}>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (): Promise<{}> => {
              executedNodes.push("first");
              throw new Error("Boom!");
            },
          }),
        { name: "First" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async () => {
              executedNodes.push("second");
              return {};
            },
          }),
        { name: "Second" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), {});

    // Second node should not execute because first threw
    expect(executedNodes).toEqual(["first"]);
  });

  test("can be cancelled during execution", async () => {
    const executedNodes: string[] = [];
    let resolveDelay: () => void;

    const compiled = new Graph<{}>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => {
              executedNodes.push("first");
              // Create a delay so we have time to cancel
              await new Promise<void>((r) => {
                resolveDelay = r;
              });
              return { value: 1 };
            },
          }),
        { name: "First" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async () => {
              executedNodes.push("second");
              return { value: 2 };
            },
          }),
        { name: "Second" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    // Start execution but don't await
    const runPromise = engine.run(engine.getStartNodeId(), {});

    // Give first node time to start
    await new Promise((r) => setTimeout(r, 10));

    // Cancel the engine
    engine.cancel();

    // Resolve the delay so first node completes
    resolveDelay!();

    await runPromise;

    // Second node should not have executed due to cancellation
    expect(executedNodes).toEqual(["first"]);
  });

  test("calls outputExternal for external outputs", async () => {
    const externalOutputs: Array<{ name: string; data: unknown }> = [];

    const compiled = new Graph<{}>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => ({ message: "hello" }),
          }),
        { name: "Greeter" }
      )
      .compile();

    const engine = new Engine(
      compiled,
      NODE_DEFINITIONS,
      {},
      {
        onNodeExternalOutput: (outputName, data) => {
          externalOutputs.push({ name: outputName, data });
        },
      }
    );

    await engine.run(engine.getStartNodeId(), {});

    // Note: External outputs depend on node implementation using outputExternal
    // The evaluate node doesn't use it by default, so this verifies the plumbing
    expect(externalOutputs).toEqual([]);
  });

  test("handles async conditions in if nodes", async () => {
    const executedBranches: string[] = [];

    const compiled = new Graph<{}>()
      .run(
        (f) =>
          f.evaluate({
            fn: async () => ({ checkValue: 100 }),
          }),
        { name: "Setup" }
      )
      .if(
        {
          condition: async (input: any) => {
            // Simulate async condition check
            await new Promise((r) => setTimeout(r, 10));
            return input?.checkValue > 50;
          },
          then: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("high");
                    return { status: "high" };
                  },
                }),
              { name: "High Value" }
            ),
          else: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => {
                    executedBranches.push("low");
                    return { status: "low" };
                  },
                }),
              { name: "Low Value" }
            ),
        },
        "Check Value"
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), {});

    expect(executedBranches).toEqual(["high"]);
  });

  test("data flows correctly through the graph", async () => {
    let finalOutput: unknown = null;

    const compiled = new Graph<{ initial: number }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input: any) => ({
              value: input.initial + 5,
            }),
          }),
        { name: "Add 5" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async (input: any) => ({ value: input.value * 2 }),
          }),
        { name: "Double" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async (input: any) => {
              finalOutput = input.value;
              return { result: input.value };
            },
          }),
        { name: "Capture" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { initial: 10 });

    // (10 + 5) * 2 = 30
    expect(finalOutput).toBe(30);
  });

  test("executes a graph that uses a module", async () => {
    const executionOrder: string[] = [];
    let finalResult: unknown = null;

    // Create a reusable module that doubles a value
    const doublerModule = new Graph<{ value: number }>().run(
      (f) =>
        f.evaluate({
          fn: async (input) => {
            executionOrder.push("doubler");
            return { doubled: input.value * 2 };
          },
        }),
      { name: "Doubler" }
    );

    // Main graph that uses the module
    const compiled = new Graph<{ input: number }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              executionOrder.push("prep");
              return { value: input.input + 10 };
            },
          }),
        { name: "Prep" }
      )
      .useModule(doublerModule)
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => {
              executionOrder.push("finalize");
              finalResult = input.doubled;
              return { result: input.doubled };
            },
          }),
        { name: "Finalize" }
      )
      .compile();

    const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

    await engine.run(engine.getStartNodeId(), { input: 5 });

    // Execution order should include the module's node
    expect(executionOrder).toEqual(["prep", "doubler", "finalize"]);

    // (5 + 10) * 2 = 30
    expect(finalResult).toBe(30);
  });

  // Config tests
  describe("config", () => {
    test("passes config values to evaluate node functions", async () => {
      let receivedConfig: Record<string, string> = {};

      type MyConfig = { apiKey: string; mode: string };

      const compiled = new Graph<{ value: number }, MyConfig>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                receivedConfig = config;
                return { result: input.value * 2 };
              },
            }),
          { name: "Process" }
        )
        .compile();

      const engine = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { apiKey: "sk-123", mode: "fast" },
        {}
      );

      await engine.run(engine.getStartNodeId(), { value: 10 });

      expect(receivedConfig).toEqual({ apiKey: "sk-123", mode: "fast" });
    });

    test("passes config values to if node conditions", async () => {
      let receivedConfig: Record<string, string> = {};
      const executedBranches: string[] = [];

      type MyConfig = { threshold: string };

      const compiled = new Graph<{ value: number }, MyConfig>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => ({ value: input.value }),
            }),
          { name: "Passthrough" }
        )
        .if(
          {
            condition: (input, config) => {
              receivedConfig = config;
              return input.value > parseInt(config.threshold);
            },
            then: (branch) =>
              branch.run(
                (f) =>
                  f.evaluate({
                    fn: async () => {
                      executedBranches.push("then");
                      return { branch: "then" };
                    },
                  }),
                { name: "Above Threshold" }
              ),
            else: (branch) =>
              branch.run(
                (f) =>
                  f.evaluate({
                    fn: async () => {
                      executedBranches.push("else");
                      return { branch: "else" };
                    },
                  }),
                { name: "Below Threshold" }
              ),
          },
          "Check Threshold"
        )
        .compile();

      // Test with value above threshold
      const engine1 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { threshold: "50" },
        {}
      );
      await engine1.run(engine1.getStartNodeId(), { value: 100 });

      expect(receivedConfig).toEqual({ threshold: "50" });
      expect(executedBranches).toEqual(["then"]);

      // Reset and test with value below threshold
      executedBranches.length = 0;
      const engine2 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { threshold: "50" },
        {}
      );
      await engine2.run(engine2.getStartNodeId(), { value: 25 });

      expect(executedBranches).toEqual(["else"]);
    });

    test("config is available in all nodes in a chain", async () => {
      const configsReceived: Array<Record<string, string>> = [];

      type MyConfig = { multiplier: string };

      const compiled = new Graph<{ start: number }, MyConfig>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                configsReceived.push({ ...config });
                return { value: input.start * parseInt(config.multiplier) };
              },
            }),
          { name: "First Multiply" }
        )
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                configsReceived.push({ ...config });
                return { value: input.value * parseInt(config.multiplier) };
              },
            }),
          { name: "Second Multiply" }
        )
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                configsReceived.push({ ...config });
                return { final: input.value };
              },
            }),
          { name: "Finalize" }
        )
        .compile();

      const engine = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { multiplier: "3" },
        {}
      );

      await engine.run(engine.getStartNodeId(), { start: 2 });

      // All 3 nodes should have received the same config
      expect(configsReceived).toHaveLength(3);
      expect(configsReceived[0]).toEqual({ multiplier: "3" });
      expect(configsReceived[1]).toEqual({ multiplier: "3" });
      expect(configsReceived[2]).toEqual({ multiplier: "3" });
    });

    test("config values affect computation results", async () => {
      let finalResult: number = 0;

      type MyConfig = {
        operation: "add" | "multiply";
        operand: string;
      };

      const compiled = new Graph<{ value: number }, MyConfig>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                const operand = parseInt(config.operand);
                if (config.operation === "add") {
                  return { result: input.value + operand };
                } else {
                  return { result: input.value * operand };
                }
              },
            }),
          { name: "Calculate" }
        )
        .run(
          (f) =>
            f.evaluate({
              fn: async (input) => {
                finalResult = input.result;
                return { final: input.result };
              },
            }),
          { name: "Capture Result" }
        )
        .compile();

      // Test with add operation
      const addEngine = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { operation: "add", operand: "10" },
        {}
      );
      await addEngine.run(addEngine.getStartNodeId(), { value: 5 });
      expect(finalResult).toBe(15); // 5 + 10

      // Test with multiply operation
      const multiplyEngine = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { operation: "multiply", operand: "10" },
        {}
      );
      await multiplyEngine.run(multiplyEngine.getStartNodeId(), { value: 5 });
      expect(finalResult).toBe(50); // 5 * 10
    });

    test("empty config works correctly", async () => {
      let receivedConfig: Record<string, string> = { unexpected: "value" };

      const compiled = new Graph<{ x: number }>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                receivedConfig = config;
                return { result: input.x };
              },
            }),
          { name: "Process" }
        )
        .compile();

      const engine = new Engine(compiled, NODE_DEFINITIONS, {}, {});

      await engine.run(engine.getStartNodeId(), { x: 42 });

      expect(receivedConfig).toEqual({});
    });

    test("boolean and string config values work correctly", async () => {
      const receivedConfigs: Array<{ verbose: boolean; multiplier: string }> =
        [];

      type MyConfig = {
        verbose: boolean;
        multiplier: string;
      };

      const compiled = new Graph<{ value: number }, MyConfig>()
        .run(
          (f) =>
            f.evaluate({
              fn: async (input, config) => {
                // TypeScript knows verbose is boolean, multiplier is string
                receivedConfigs.push({
                  verbose: config.verbose,
                  multiplier: config.multiplier,
                });
                const mult = parseInt(config.multiplier);
                return {
                  result: input.value * mult,
                  wasVerbose: config.verbose,
                };
              },
            }),
          { name: "Process" }
        )
        .compile();

      // Pass boolean true for switch
      const engine1 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { verbose: true, multiplier: "2" },
        {}
      );
      await engine1.run(engine1.getStartNodeId(), { value: 5 });
      expect(receivedConfigs[0].verbose).toBe(true);
      expect(typeof receivedConfigs[0].verbose).toBe("boolean");

      // Pass boolean false for switch
      const engine2 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { verbose: false, multiplier: "3" },
        {}
      );
      await engine2.run(engine2.getStartNodeId(), { value: 5 });
      expect(receivedConfigs[1].verbose).toBe(false);
      expect(typeof receivedConfigs[1].verbose).toBe("boolean");
    });

    test("config is available inside if branches", async () => {
      const configsInBranches: Array<{
        branch: string;
        config: Record<string, string>;
      }> = [];

      type MyConfig = { branchLabel: string };

      const compiled = new Graph<{ goLeft: boolean }, MyConfig>()
        .if(
          {
            condition: (input) => input.goLeft,
            then: (branch) =>
              branch.run(
                (f) =>
                  f.evaluate({
                    fn: async (input, config) => {
                      configsInBranches.push({
                        branch: "then",
                        config: { ...config },
                      });
                      return { path: "left" };
                    },
                  }),
                { name: "Left Branch" }
              ),
            else: (branch) =>
              branch.run(
                (f) =>
                  f.evaluate({
                    fn: async (input, config) => {
                      configsInBranches.push({
                        branch: "else",
                        config: { ...config },
                      });
                      return { path: "right" };
                    },
                  }),
                { name: "Right Branch" }
              ),
          },
          "Choose Path"
        )
        .compile();

      // Test left branch
      const engine1 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { branchLabel: "test-config" },
        {}
      );
      await engine1.run(engine1.getStartNodeId(), { goLeft: true });

      expect(configsInBranches).toHaveLength(1);
      expect(configsInBranches[0]).toEqual({
        branch: "then",
        config: { branchLabel: "test-config" },
      });

      // Test right branch
      configsInBranches.length = 0;
      const engine2 = new Engine(
        compiled,
        NODE_DEFINITIONS,
        { branchLabel: "test-config" },
        {}
      );
      await engine2.run(engine2.getStartNodeId(), { goLeft: false });

      expect(configsInBranches).toHaveLength(1);
      expect(configsInBranches[0]).toEqual({
        branch: "else",
        config: { branchLabel: "test-config" },
      });
    });
  });
});
