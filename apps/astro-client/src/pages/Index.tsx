import { Navigate } from "react-router";

export default function RedirectForIndex() {
  return <Navigate to="/browse" replace />;
}
