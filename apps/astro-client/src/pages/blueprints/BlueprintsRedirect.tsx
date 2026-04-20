import { redirect } from "react-router";
import { blueprintsPaths } from "@/lib/routes";

export async function loader() {
  return redirect(blueprintsPaths.discover);
}

export default function BlueprintsRedirect() {
  return null;
}
