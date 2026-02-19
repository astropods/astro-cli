import { Link } from "react-router";
import { Users, CheckCircle, Clock } from "lucide-react";
import { useAuth, getUserDisplayName } from "../lib/auth";
import { ProtectedRoute } from "../components/ProtectedRoute";

function YourAgentsContent() {
  const { user } = useAuth();

  // Placeholder metrics - these would come from actual user data
  const metrics = {
    activeAgents: 0,
    tasksCompleted: 0,
    hoursSaved: 0,
  };

  const hasAgents = metrics.activeAgents > 0;

  return (
    <div className="max-w-[900px] p-6 md:p-8">
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-semibold">Your Agents</h1>
          {user && (
            <p className="text-sm text-stone-600 mt-1">
              Welcome back, {getUserDisplayName(user)}
            </p>
          )}
        </div>
        <span className="px-2.5 py-1 border border-stone-300 text-sm">
          {metrics.activeAgents} Active
        </span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        <div className="border border-stone-300 p-4 flex items-center gap-3">
          <div className="w-10 h-10 border border-stone-300 flex items-center justify-center">
            <Users size={20} />
          </div>
          <div className="flex flex-col">
            <span className="text-2xl font-bold">{metrics.activeAgents}</span>
            <span className="text-sm text-stone-600">Active Agents</span>
          </div>
        </div>

        <div className="border border-stone-300 p-4 flex items-center gap-3">
          <div className="w-10 h-10 border border-stone-300 flex items-center justify-center">
            <CheckCircle size={20} />
          </div>
          <div className="flex flex-col">
            <span className="text-2xl font-bold">{metrics.tasksCompleted}</span>
            <span className="text-sm text-stone-600">Tasks Completed</span>
          </div>
        </div>

        <div className="border border-stone-300 p-4 flex items-center gap-3">
          <div className="w-10 h-10 border border-stone-300 flex items-center justify-center">
            <Clock size={20} />
          </div>
          <div className="flex flex-col">
            <span className="text-2xl font-bold">{metrics.hoursSaved}</span>
            <span className="text-sm text-stone-600">Hours Saved</span>
          </div>
        </div>
      </div>

      <section>
        <h2 className="text-base font-semibold mb-4">Your agents</h2>

        {hasAgents ? (
          <div>{/* Agent cards would go here when user has agents */}</div>
        ) : (
          <div className="border border-stone-300 text-center py-12 px-6">
            <div className="w-16 h-16 border border-stone-300 mx-auto mb-4 flex items-center justify-center">
              <Users size={32} className="text-stone-500" />
            </div>
            <h3 className="font-semibold mb-2">No agents yet</h3>
            <p className="text-stone-600 text-sm mb-4">
              Hire your first agent to start automating your workflows
            </p>
            <Link
              to="/hire"
              className="inline-block px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm no-underline hover:bg-stone-700"
            >
              Hire your first agent
            </Link>
          </div>
        )}
      </section>
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
