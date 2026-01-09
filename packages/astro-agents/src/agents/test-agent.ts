import { Graph } from "astro-graph";

type ExampleAgentConfig = {
  some_string: string;
  some_number: number;
};

export const exampleAgent = new Graph<
  {
    message: string;
    recipient: string;
    sendAt: Date;
  },
  ExampleAgentConfig
>()
  .meta({
    title: "Example Agent",
    description:
      "An example agent that sends a message to a recipient at a given time",
  })
  .run(
    (nodes) =>
      nodes.evaluate({
        fn: async (_input, config) => {
          config.some_number;
          // Implement the logic to send the message
        },
      }),
    { name: "Send Message" }
  );
