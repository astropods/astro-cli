import { Graph } from "astro-graph";

type ConfigExampleConfig = {
  slack_key: string;
  allow_admin_access: boolean;
};

export const configExampleAgent = new Graph<
  {
    message: string;
    recipient: string;
    sendAt: Date;
  },
  ConfigExampleConfig
>()
  .meta({
    title: "Config Example Agent",
    description: "An example agent that uses configs",
  })
  .run(
    (nodes) =>
      nodes.evaluate({
        fn: async (_input, config) => {
          console.log(config.allow_admin_access);
          // Implement the logic to send the message
        },
      }),
    { name: "Send Message" }
  );
