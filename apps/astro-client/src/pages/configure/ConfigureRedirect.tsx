import { redirect } from "react-router";
import type { Route } from "./+types/ConfigureRedirect";

export async function loader({ params }: Route.LoaderArgs) {
  return redirect(`/${params.account}/agents/${params.deploymentId}/configure/deployment`);
}

export default function ConfigureRedirect() {
  return null;
}
