import { redirect } from "react-router";

export async function loader() {
  return redirect("/blueprints");
}

export default function RedirectForIndex() {
  return null;
}
