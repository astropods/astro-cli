import { Navigate, useParams } from "react-router";

export default function ConfigureRedirect() {
  const { account, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  return <Navigate to={`/${account}/agents/${deploymentId}/configure/deployment`} replace />;
}
