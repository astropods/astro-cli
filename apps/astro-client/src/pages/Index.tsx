import { Navigate } from "react-router";

export default function RedirectForIndex() {
  return <Navigate to="/blueprints" replace />;
}
