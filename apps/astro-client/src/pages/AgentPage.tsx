import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate, useLocation, Link } from "react-router-dom";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  ArrowLeft,
  Rocket,
  Loader2,
  Trash2,
  Activity,
  BarChart3,
  AlertTriangle,
  X,
  AlertCircle,
  Package,
  BookOpen,
} from "lucide-react";
import type { DeployResponse } from "../lib/api";
import { useAuth } from "../lib/auth";
import { usePublishAgent, useAgent } from "../api/queries/agents";
import { useDeployments, useUndeployAgent } from "../api/queries/deployments";
import { AgentBuildsSection } from "../components/operator/AgentBuildsSection";
import { DeploymentCard } from "../components/operator/DeploymentCard";
import { DeployResultModal } from "../components/operator/DeployResultModal";
import { PublishModal } from "../components/operator/PublishModal";
import { PlaygroundChat } from "../components/operator/PlaygroundChat";
import { ObservabilityTab } from "../components/operator/ObservabilityTab";

type Tab = "overview" | "builds" | "deployments" | "observability";

export function AgentPage() {
  const { account, agent: agentName } = useParams<{ account: string; agent: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const { isAuthenticated, login, accounts } = useAuth();
  const userAccount = accounts[0]?.name ?? "";

  // Measure offset from top of viewport to pin the page height
  const containerRef = useRef<HTMLDivElement>(null);
  const [topOffset, setTopOffset] = useState(0);
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => setTopOffset(el.getBoundingClientRect().top);
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, []);

  // Fetch full agent data for readme/description
  const { data: agentData, isLoading: agentLoading } = useAgent(account ?? "", agentName ?? "");

  // Deploy result from navigation state (set by DeployPage after successful deploy)
  const [deployResult, setDeployResult] = useState<DeployResponse | null>(
    (location.state as { deployResult?: DeployResponse } | null)?.deployResult ?? null
  );

  // If we arrived with a deploy result, default to deployments tab
  const [activeTab, setActiveTab] = useState<Tab>(
    (location.state as { deployResult?: DeployResponse } | null)?.deployResult ? "deployments" : "overview"
  );

  // Clear location state after reading it so refreshing doesn't re-show the modal
  useEffect(() => {
    if (location.state?.deployResult) {
      window.history.replaceState({}, document.title);
    }
  }, []);

  // Publish modal state
  const [publishTarget, setPublishTarget] = useState<{ name: string; buildId: string } | null>(null);

  // Undeploy confirm state
  const [undeployConfirm, setUndeployConfirm] = useState<string | null>(null);

  // Deployments filtered to this agent
  const { data: deploymentsData, isLoading: deploymentsLoading, refetch: refetchDeployments } = useDeployments(userAccount, isAuthenticated);
  const allDeployments = deploymentsData?.deployments ?? [];
  const deployments = allDeployments.filter((dep) => dep.name === agentName);

  // Mutations
  const undeployMutation = useUndeployAgent(userAccount);
  const publishMutation = usePublishAgent(userAccount);

  const handleUndeploy = (name: string) => {
    setUndeployConfirm(name);
  };

  const confirmUndeploy = async () => {
    if (!undeployConfirm) return;
    try {
      await undeployMutation.mutateAsync({ account: userAccount, name: undeployConfirm });
      setUndeployConfirm(null);
    } catch (err) {
      console.error("Failed to undeploy:", err);
      const apiErr = err as { details?: string; error?: string; message?: string };
      alert(apiErr.details || apiErr.error || apiErr.message || "Failed to undeploy agent");
    }
  };

  const handlePublish = async (version: string) => {
    if (!publishTarget) return;
    try {
      await publishMutation.mutateAsync({
        name: publishTarget.name,
        build_id: publishTarget.buildId,
        version,
      });
      setPublishTarget(null);
    } catch (err) {
      const apiErr = err as { details?: string; error?: string };
      alert(apiErr.details || apiErr.error || "Failed to publish");
    }
  };

  if (!account || !agentName) return null;

  const latest = agentData?.versions[0];
  const readme = latest?.readme;
  const description = latest?.spec?.meta?.description;

  const tabs: { id: Tab; label: string; icon: React.ReactNode; count?: number }[] = [
    { id: "overview", label: "Overview", icon: <BookOpen size={16} /> },
    { id: "builds", label: "Builds", icon: <Package size={16} /> },
    { id: "deployments", label: "Deployments", icon: <Activity size={16} />, count: deployments.length },
    { id: "observability", label: "Observability", icon: <BarChart3 size={16} /> },
  ];

  return (
    <div
      ref={containerRef}
      className="flex overflow-hidden"
      style={{ height: topOffset ? `calc(100dvh - ${topOffset}px)` : '100dvh' }}
    >
    <div className="flex-1 min-w-0 flex flex-col">
      {/* Header — fixed */}
      <div className="px-6 md:px-8 pt-6 md:pt-8 shrink-0">
        <div className="flex items-center justify-between mb-4">
          <div>
            <Link
              to="/operator"
              className="flex items-center gap-1 text-sm text-stone-500 hover:text-stone-800 no-underline mb-2"
            >
              <ArrowLeft size={16} />
              Back to Home
            </Link>
            <h1 className="text-2xl font-semibold">
              <span className="font-normal text-stone-500">{account}/</span>
              {agentName}
            </h1>
            {description && (
              <p className="text-sm text-stone-500 mt-1">{description}</p>
            )}
          </div>
          <button
            onClick={() => navigate(`/operator/deploy/${account}/${agentName}`)}
            className="px-4 py-2 border border-stone-800 text-sm bg-stone-800 text-white hover:bg-stone-700 cursor-pointer flex items-center gap-2"
          >
            <Rocket size={16} />
            Deploy
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
              to manage agents.
            </span>
          </div>
        )}

        {isAuthenticated && (
          <div className="flex items-center gap-0 border-b border-stone-300">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-1.5 px-4 py-2.5 text-sm border-b-2 -mb-px cursor-pointer bg-transparent ${
                  activeTab === tab.id
                    ? "border-stone-800 text-stone-900 font-medium"
                    : "border-transparent text-stone-500 hover:text-stone-700 hover:border-stone-300"
                }`}
              >
                {tab.icon}
                {tab.label}
                {tab.count !== undefined && tab.count > 0 && (
                  <span className="ml-1 px-1.5 py-0.5 text-xs bg-stone-100 border border-stone-200 rounded-full">
                    {tab.count}
                  </span>
                )}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Tab content — scrollable */}
      {isAuthenticated && (
        <div className="flex-1 min-h-0 overflow-y-auto px-6 md:px-8 py-6">
          {activeTab === "overview" && (
            <div className="max-w-3xl">
              {agentLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 size={24} className="animate-spin text-stone-500" />
                </div>
              ) : readme ? (
                <div className="border border-stone-300 bg-white">
                  <div className="px-4 py-3 border-b border-stone-300 bg-stone-50">
                    <h3 className="text-sm font-medium flex items-center gap-1.5">
                      <BookOpen size={14} className="text-stone-500" />
                      README
                    </h3>
                  </div>
                  <div className="p-6">
                    <div className="prose prose-sm prose-stone max-w-none">
                      <Markdown remarkPlugins={[remarkGfm]}>{readme}</Markdown>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="p-8 border border-stone-300 bg-stone-50 text-center">
                  <BookOpen size={32} className="mx-auto text-stone-400 mb-2" />
                  <p className="text-stone-600 text-sm">No README available</p>
                  <p className="text-stone-500 text-xs mt-1">
                    Add a README to your agent spec to display it here
                  </p>
                </div>
              )}
            </div>
          )}

          {activeTab === "builds" && (
            <AgentBuildsSection
              accountName={account}
              agentName={agentName}
              onPublish={(name, buildId) => setPublishTarget({ name, buildId })}
            />
          )}

          {activeTab === "observability" && (
            <ObservabilityTab account={account} agentName={agentName} />
          )}

          {activeTab === "deployments" && (
            <>
              {deploymentsLoading ? (
                <div className="flex items-center justify-center py-8 border border-stone-300 bg-stone-50">
                  <Loader2 size={24} className="animate-spin text-stone-500" />
                </div>
              ) : deployments.length === 0 ? (
                <div className="p-6 border border-stone-300 bg-stone-50 text-center">
                  <Activity size={32} className="mx-auto text-stone-400 mb-2" />
                  <p className="text-stone-600 text-sm">Not currently deployed</p>
                  <p className="text-stone-500 text-xs mt-1">
                    Click Deploy above to get started
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {deployments.map((dep) => (
                    <DeploymentCard
                      key={`${dep.name}:${dep.build_id}`}
                      accountName={userAccount}
                      deployment={dep}
                      onUndeploy={handleUndeploy}
                      onRefresh={() => refetchDeployments()}
                      isUndeploying={undeployMutation.isPending && undeployMutation.variables?.name === dep.name}
                    />
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Modals — fixed position, outside scroll context */}
      {deployResult && (
        <DeployResultModal
          result={deployResult}
          onClose={() => setDeployResult(null)}
        />
      )}

      {publishTarget && (
        <PublishModal
          accountName={account}
          agentName={publishTarget.name}
          buildId={publishTarget.buildId}
          onClose={() => setPublishTarget(null)}
          onPublish={handlePublish}
          isPublishing={publishMutation.isPending}
        />
      )}

      {undeployConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white border border-stone-300 w-full max-w-[400px] flex flex-col">
            <div className="flex items-center justify-between p-4 border-b border-stone-300">
              <div className="flex items-center gap-2">
                <AlertTriangle size={20} className="text-red-600" />
                <h2 className="text-lg font-semibold">Confirm Undeploy</h2>
              </div>
              <button
                className="bg-transparent border-none cursor-pointer p-1"
                onClick={() => setUndeployConfirm(null)}
                disabled={undeployMutation.isPending}
              >
                <X size={20} />
              </button>
            </div>

            <div className="p-4">
              <p className="text-sm text-stone-600">
                Are you sure you want to undeploy <strong className="font-mono">{undeployConfirm}</strong>? This will stop all running pods and remove the deployment.
              </p>
            </div>

            <div className="flex gap-2 p-4 border-t border-stone-300">
              <button
                onClick={() => setUndeployConfirm(null)}
                disabled={undeployMutation.isPending}
                className="flex-1 px-4 py-2 border border-stone-300 text-sm bg-white hover:bg-stone-50 cursor-pointer disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                onClick={confirmUndeploy}
                disabled={undeployMutation.isPending}
                className="flex-1 px-4 py-2 border border-red-600 text-sm bg-red-600 text-white hover:bg-red-500 cursor-pointer disabled:opacity-50 flex items-center justify-center gap-2"
              >
                {undeployMutation.isPending ? (
                  <>
                    <Loader2 size={16} className="animate-spin" />
                    Undeploying...
                  </>
                ) : (
                  <>
                    <Trash2 size={16} />
                    Undeploy
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
    {isAuthenticated && <PlaygroundChat deployments={deployments} />}
    </div>
  );
}
