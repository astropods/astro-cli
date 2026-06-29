import { redirect } from "react-router";
import { getCurrentUserForRequest } from "@/lib/api.server";
import { accountBlueprintsPath, explorePath } from "@/lib/routes";

export async function loader({ request }: { request: Request }) {
  try {
    await getCurrentUserForRequest(request);
    return redirect(accountBlueprintsPath);
  } catch {
    // Match root.tsx: auth lookup failures are treated as signed-out state so
    // public visitors and broken sessions land on public discovery.
    return redirect(explorePath);
  }
}

export default function RedirectForIndex() {
  return null;
}
