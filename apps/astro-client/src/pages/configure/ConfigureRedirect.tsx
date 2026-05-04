// NOTE: This file is no longer routed (replaced by agent-detail routes).
// Kept temporarily as reference while the new page is built.

import { redirect } from "react-router";

export async function loader({ params }: { params: Record<string, string | undefined> }) {
  return redirect(`/${params.account}/agents/${params.deploymentId}/configure/deployment`);
}

export default function ConfigureRedirect() {
  return null;
}
