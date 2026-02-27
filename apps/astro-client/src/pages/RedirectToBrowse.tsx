import { Navigate } from "react-router";

export default function RedirectToBrowse() {
  return <Navigate to="/browse" replace />;
}
