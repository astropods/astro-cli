import { Navigate } from "react-router";

export default function SettingsRedirect() {
  return <Navigate to="/settings/account" replace />;
}
