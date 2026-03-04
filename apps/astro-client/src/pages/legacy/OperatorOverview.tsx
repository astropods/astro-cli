/**
 * @deprecated Legacy operator overview page (route: operator).
 * Do not add new features here.
 */
import { Link } from "react-router";
import {
  RefreshCw,
  Server,
  AlertCircle,
  ChevronRight,
} from "lucide-react";
import { useAuth } from "../../lib/auth";
import { useProfile } from "../../api/queries/accounts";

export default function OperatorOverview() {
  const { isAuthenticated, login } = useAuth();
  const { data: profileData, refetch: refetchProfile, isLoading } = useProfile();
  const currentAccount = profileData?.accounts?.[0];
  const accountName = currentAccount?.name ?? "";
  const agentSummaries = currentAccount?.agents ?? [];

  return (
    <div className="p-6 md:p-8">
      <div className="flex justify-between items-start mb-6">
        <div>
          <h1 className="text-2xl font-semibold mb-1">Home</h1>
          <p className="text-stone-600 text-sm">
            Manage builds and deploy agents
          </p>
        </div>
        <button
          onClick={() => refetchProfile()}
          disabled={isLoading}
          className="flex items-center gap-2 px-4 py-2 border border-stone-300 bg-white text-sm text-stone-700 hover:bg-stone-50 cursor-pointer disabled:opacity-50"
        >
          <RefreshCw size={16} className={isLoading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      {!isAuthenticated && (
        <div className="mb-4 p-3 bg-yellow-50 border border-yellow-200 text-yellow-800 text-sm flex items-center gap-2">
          <AlertCircle size={16} />
          <span>
            You need to{" "}
            <button onClick={login} className="underline font-medium bg-transparent border-none cursor-pointer text-yellow-800">
              sign in
            </button>{" "}
            to manage and deploy agents.
          </span>
        </div>
      )}

      {isAuthenticated && (
        <div className="max-w-2xl">
          {agentSummaries.length === 0 ? (
            <div className="p-8 border border-stone-300 text-center">
              <Server size={48} className="mx-auto text-stone-400 mb-4" />
              <h3 className="text-lg font-medium mb-2">No agents registered</h3>
              <p className="text-stone-600 text-sm">
                Use the CLI to register your first agent build.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {agentSummaries.map((summary) => (
                <Link
                  key={summary.name}
                  to={`/u/${accountName}/${summary.name}`}
                  className="flex items-center gap-3 p-4 border border-stone-300 bg-white hover:bg-stone-50 transition-colors no-underline text-inherit"
                >
                  <Server size={20} className="text-stone-500 shrink-0" />
                  <div className="flex-1 min-w-0">
                    <h3 className="font-semibold">
                      <span className="font-normal text-stone-500">{accountName}/</span>
                      {summary.name}
                    </h3>
                    <p className="text-sm text-stone-500">
                      {summary.build_count} build{summary.build_count !== 1 ? "s" : ""}
                    </p>
                  </div>
                  <ChevronRight size={20} className="text-stone-400 shrink-0" />
                </Link>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
