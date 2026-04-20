import { redirect } from "react-router";

export async function loader() {
  return redirect("/settings/account");
}

export default function SettingsRedirect() {
  return null;
}
