import type { MetaFunction } from "react-router";
import { Navigate } from "react-router";
import { hasExperiments } from "@/lib/experiments";

export const meta: MetaFunction = () => [{ title: "Experiments - Settings | Astro" }];

export default function ExperimentsSettings() {
  if (!hasExperiments) return <Navigate to="/settings/account" replace />;
  // Experiment rows render here once hasExperiments is true.
  return null;
}
