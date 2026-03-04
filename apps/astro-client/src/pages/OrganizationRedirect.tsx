import { Navigate } from "react-router";

export default function OrganizationRedirect() {
  return <Navigate to="/organization/new" replace />;
}
