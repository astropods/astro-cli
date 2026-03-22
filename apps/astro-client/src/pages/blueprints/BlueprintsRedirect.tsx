import { Navigate } from "react-router";
import { blueprintsPaths } from "@/lib/routes";

export default function BlueprintsRedirect() {
  return <Navigate to={blueprintsPaths.discover} replace />;
}
