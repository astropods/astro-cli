import { redirect } from "react-router";

export async function loader() {
  return redirect("/organization/new");
}

export default function OrganizationRedirect() {
  return null;
}
