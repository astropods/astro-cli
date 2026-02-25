import { ProtectedRoute } from "../components/ProtectedRoute";

function YourAgentsContent() {
  return (
    <div className="max-w-[900px] p-6 md:p-8">
      <h1 className="text-2xl font-semibold">Your Agents</h1>
    </div>
  );
}

export default function YourAgents() {
  return (
    <ProtectedRoute>
      <YourAgentsContent />
    </ProtectedRoute>
  );
}
