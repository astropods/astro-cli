import { describe, test, expect } from "bun:test";
import { Graph } from "./index";

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
});
