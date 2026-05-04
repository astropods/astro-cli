import { redirect } from "react-router";
import type { Route } from "./+types/AgentDetailRedirect";

export async function loader({ params }: Route.LoaderArgs) {
  return redirect(`/${params.account}/agents/${params.deploymentId}/deployments`);
}

export default function AgentDetailRedirect() {
  return null;
}
