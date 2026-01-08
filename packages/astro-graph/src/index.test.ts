import { describe, test, expect } from "bun:test";
import { Graph, type CompiledGraph } from "./index";

describe("Graph", () => {
  test("creates a graph with a single evaluate node", () => {
    const graph = new Graph();
    const evaluateFn = async () => {};

    const result = graph
      .run((f) => f.evaluate({ fn: evaluateFn }), { name: "My Node" })
      .compile();

    const nodes = Object.values(result.nodes);
    const edges = Object.values(result.edges);

    // Should have 2 nodes: start + execute
    expect(nodes).toHaveLength(2);

    const startNode = nodes.find((n) => n.type === "start");
    const evaluateNode = nodes.find((n) => n.type === "evaluate");

    expect(startNode).toBeDefined();
    expect(evaluateNode).toBeDefined();
    expect(evaluateNode!.name).toBe("My Node");
    expect(evaluateNode!.data.fn).toBe(evaluateFn);

    // Should have 1 edge from start to execute
    expect(edges).toHaveLength(1);
    expect(edges[0].source).toBe(startNode!.id);
    expect(edges[0].target).toBe(evaluateNode!.id);
  });

  test("chains multiple evaluate nodes with edges", () => {
    const graph = new Graph();

    const result = graph
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "First" })
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "Second" })
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "Third" })
      .compile();

    const nodes = Object.values(result.nodes);
    const edges = Object.values(result.edges);

    // Should have 4 nodes: start + 3 execute nodes
    expect(nodes).toHaveLength(4);
    // Should have 3 edges: start->First, First->Second, Second->Third
    expect(edges).toHaveLength(3);

    // Edges should connect nodes sequentially
    const nodeIds = nodes.map((n) => n.id);
    expect(edges[0].source).toBe(nodeIds[0]);
    expect(edges[0].target).toBe(nodeIds[1]);
    expect(edges[1].source).toBe(nodeIds[1]);
    expect(edges[1].target).toBe(nodeIds[2]);
    expect(edges[2].source).toBe(nodeIds[2]);
    expect(edges[2].target).toBe(nodeIds[3]);
  });

  test("creates an if node with then and else branches", () => {
    const graph = new Graph();
    const condition = () => true;

    const result = graph
      .if(
        {
          condition,
          then: (branch) =>
            branch.run((f) => f.evaluate({ fn: async () => {} }), {
              name: "Then Action",
            }),
          else: (branch) =>
            branch.run((f) => f.evaluate({ fn: async () => {} }), {
              name: "Else Action",
            }),
        },
        "Check Condition"
      )
      .compile();

    const nodes = Object.values(result.nodes);
    const edges = Object.values(result.edges);

    // Should have 4 nodes: start, if, then-execute, else-execute
    expect(nodes).toHaveLength(4);

    const startNode = nodes.find((n) => n.type === "start");
    const ifNode = nodes.find((n) => n.type === "if");
    expect(startNode).toBeDefined();
    expect(ifNode).toBeDefined();
    expect(ifNode!.name).toBe("Check Condition");

    // Should have 3 edges: start->if, if->then, if->else
    expect(edges).toHaveLength(3);

    const startEdge = edges.find((e) => e.source === startNode!.id);
    const thenEdge = edges.find((e) => e.sourcePort === "then");
    const elseEdge = edges.find((e) => e.sourcePort === "else");

    expect(startEdge).toBeDefined();
    expect(startEdge!.target).toBe(ifNode!.id);
    expect(thenEdge).toBeDefined();
    expect(elseEdge).toBeDefined();
    expect(thenEdge!.source).toBe(ifNode!.id);
    expect(elseEdge!.source).toBe(ifNode!.id);
  });

  test("types flow through chained blocks before and inside if branches", () => {
    // This test verifies that types flow correctly:
    // 1. From initial graph input through chained blocks
    // 2. Into the if condition
    // 3. Through blocks inside each branch

    const result = new Graph<{ text: string }>()
      // First transform: { text: string } -> { words: string[], count: number }
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({
              words: input.text.split(" "),
              count: input.text.length,
            }),
          }),
        { name: "Parse Text" }
      )
      // Second transform: { words, count } -> { isLong: boolean, words }
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({
              isLong: input.count > 10,
              words: input.words,
            }),
          }),
        { name: "Check Length" }
      )
      // If block receives { isLong, words } and branches can access it
      .if(
        {
          condition: (input) => input.isLong,
          then: (branch) =>
            // Then branch: { isLong, words } -> { result: string }
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async (input) => ({
                    result: `Long text with ${input.words.length} words`,
                  }),
                }),
              { name: "Handle Long Text" }
            ),
          else: (branch) =>
            // Else branch: { isLong, words } -> { result: string }
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async (input) => ({
                    result: `Short text: ${input.words.join("-")}`,
                  }),
                }),
              { name: "Handle Short Text" }
            ),
        },
        "Is Long Text?"
      )
      .compile();

    const nodes = Object.values(result.nodes);
    const edges = Object.values(result.edges);

    // Should have 6 nodes: start, Parse Text, Check Length, if, then-branch, else-branch
    expect(nodes).toHaveLength(6);

    const startNode = nodes.find((n) => n.type === "start");
    const parseNode = nodes.find((n) => n.name === "Parse Text");
    const checkNode = nodes.find((n) => n.name === "Check Length");
    const ifNode = nodes.find((n) => n.type === "if");
    const thenNode = nodes.find((n) => n.name === "Handle Long Text");
    const elseNode = nodes.find((n) => n.name === "Handle Short Text");

    expect(startNode).toBeDefined();
    expect(parseNode).toBeDefined();
    expect(checkNode).toBeDefined();
    expect(ifNode).toBeDefined();
    expect(thenNode).toBeDefined();
    expect(elseNode).toBeDefined();

    // Should have 5 edges: start->parse, parse->check, check->if, if->then, if->else
    expect(edges).toHaveLength(5);

    // Verify the chain before if
    const startToParseEdge = edges.find(
      (e) => e.source === startNode!.id && e.target === parseNode!.id
    );
    const parseToCheckEdge = edges.find(
      (e) => e.source === parseNode!.id && e.target === checkNode!.id
    );
    const checkToIfEdge = edges.find(
      (e) => e.source === checkNode!.id && e.target === ifNode!.id
    );

    expect(startToParseEdge).toBeDefined();
    expect(parseToCheckEdge).toBeDefined();
    expect(checkToIfEdge).toBeDefined();

    // Verify the if branches
    const thenEdge = edges.find(
      (e) => e.source === ifNode!.id && e.sourcePort === "then"
    );
    const elseEdge = edges.find(
      (e) => e.source === ifNode!.id && e.sourcePort === "else"
    );

    expect(thenEdge).toBeDefined();
    expect(thenEdge!.target).toBe(thenNode!.id);
    expect(elseEdge).toBeDefined();
    expect(elseEdge!.target).toBe(elseNode!.id);
  });

  test("edges have correct port configuration", () => {
    const graph = new Graph();

    const result = graph
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "A" })
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "B" })
      .compile();

    const edges = Object.values(result.edges);

    // All standard edges should use output->input ports
    edges.forEach((edge) => {
      expect(edge.sourcePort).toBe("output");
      expect(edge.targetPort).toBe("input");
    });
  });

  test("nodes have unique ids", () => {
    const graph = new Graph();

    const result = graph
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "A" })
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "B" })
      .run((f) => f.evaluate({ fn: async () => {} }), { name: "C" })
      .compile();

    const nodeIds = Object.keys(result.nodes);
    const uniqueIds = new Set(nodeIds);

    // Should have 4 nodes: start + 3 execute nodes
    expect(nodeIds).toHaveLength(4);
    expect(uniqueIds.size).toBe(nodeIds.length);
  });

  test("if branches can have different output types (union type)", () => {
    // This test verifies that branches can produce different types
    // and the result is a union type

    const result = new Graph<{ value: number }>()
      .if(
        {
          condition: (input) => input.value > 10,
          then: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => ({ status: "high" as const, multiplier: 2 }),
                }),
              { name: "High Value" }
            ),
          else: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({
                  fn: async () => ({ status: "low" as const, offset: 5 }),
                }),
              { name: "Low Value" }
            ),
        },
        "Check Value"
      )
      .compile();

    // Type assertion: result should be CompiledGraph with union output type
    // At compile time, this verifies the union type works
    type ExpectedOutput =
      | { status: "high"; multiplier: number }
      | { status: "low"; offset: number };

    // This line would fail to compile if types don't match
    const _typeCheck: CompiledGraph<{ value: number }, ExpectedOutput> = result;

    const nodes = Object.values(result.nodes);

    // Should have 4 nodes: start, if, then-branch, else-branch
    expect(nodes).toHaveLength(4);

    const thenNode = nodes.find((n) => n.name === "High Value");
    const elseNode = nodes.find((n) => n.name === "Low Value");

    expect(thenNode).toBeDefined();
    expect(elseNode).toBeDefined();
  });

  test("compiled graph carries both input and output types", () => {
    const graph = new Graph<{ input: string }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({ length: input.input.length }),
          }),
        { name: "Get Length" }
      )
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({ doubled: input.length * 2 }),
          }),
        { name: "Double" }
      )
      .compile();

    // Type check: the compiled graph should have the correct types
    type InputType = { input: string };
    type OutputType = { doubled: number };

    // This assignment verifies the types at compile time
    const _typeCheck: CompiledGraph<InputType, OutputType> = graph;

    expect(Object.values(graph.nodes)).toHaveLength(3); // start + 2 nodes
  });

  test("useModule inlines a graph and types flow through", () => {
    // Create a reusable module
    const textLengthModule = new Graph<{ text: string }>().run(
      (f) =>
        f.evaluate({
          fn: async (input) => ({ length: input.text.length }),
        }),
      { name: "Count Length" }
    );

    // Use the module in another graph
    const mainGraph = new Graph<{ rawInput: string }>()
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({ text: input.rawInput.trim() }),
          }),
        { name: "Prep Input" }
      )
      .useModule(textLengthModule)
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({ isLong: input.length > 10 }),
          }),
        { name: "Check Long" }
      )
      .compile();

    // Type check: main graph should flow from rawInput to isLong
    const _typeCheck: CompiledGraph<{ rawInput: string }, { isLong: boolean }> =
      mainGraph;

    const nodes = Object.values(mainGraph.nodes);
    const edges = Object.values(mainGraph.edges);

    // Should have: start, Prep Input, Count Length, Check Long
    expect(nodes).toHaveLength(4);

    // Verify the module's node was inlined
    const countLengthNode = nodes.find((n) => n.name === "Count Length");
    expect(countLengthNode).toBeDefined();

    // Verify edges connect properly
    expect(edges.length).toBeGreaterThanOrEqual(3);
  });

  test("chaining after if preserves start type and uses union as current", () => {
    const result = new Graph<{ text: string }>()
      .if(
        {
          condition: (input) => input.text.length > 5,
          then: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({ fn: async () => ({ type: "long" as const }) }),
              { name: "Long" }
            ),
          else: (branch) =>
            branch.run(
              (f) =>
                f.evaluate({ fn: async () => ({ type: "short" as const }) }),
              { name: "Short" }
            ),
        },
        "Check"
      )
      // After if, the input type for the next node should be the union
      .run(
        (f) =>
          f.evaluate({
            fn: async (input) => ({
              // input.type is "long" | "short"
              message: `Result was ${input.type}`,
            }),
          }),
        { name: "Format" }
      )
      .compile();

    // The final type should preserve the original start type
    // and have the output of the last node
    const _typeCheck: CompiledGraph<{ text: string }, { message: string }> =
      result;

    const nodes = Object.values(result.nodes);
    // start, if, long-branch, short-branch, format
    expect(nodes).toHaveLength(5);

    const formatNode = nodes.find((n) => n.name === "Format");
    expect(formatNode).toBeDefined();
  });
});
