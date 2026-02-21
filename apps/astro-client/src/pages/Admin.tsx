import { useState, useMemo, useEffect, useCallback } from "react";
import { api, ApiClient, type AdminAgentDeployment } from "../lib/api";
import { ApiClientProvider } from "../lib/api-context";
import { RefreshCw, Lock, AlertCircle, Activity, LogOut } from "lucide-react";
import { DeploymentCard } from "../components/operator/DeploymentCard";

const SESSION_KEY = "astro_admin";
const SESSION_TTL_MS = 30 * 60 * 1000; // 30 minutes

interface StoredSession {
  username: string;
  password: string;
  expiresAt: number;
}

function saveSession(username: string, password: string) {
  const session: StoredSession = {
    username,
    password,
    expiresAt: Date.now() + SESSION_TTL_MS,
  };
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
}

function loadSession(): { username: string; password: string } | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY);
    if (!raw) return null;
    const session: StoredSession = JSON.parse(raw);
    if (Date.now() > session.expiresAt) {
      sessionStorage.removeItem(SESSION_KEY);
      return null;
    }
    return { username: session.username, password: session.password };
  } catch {
    sessionStorage.removeItem(SESSION_KEY);
    return null;
  }
}

function clearSession() {
  sessionStorage.removeItem(SESSION_KEY);
}

export default function Admin() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [credentials, setCredentials] = useState<{
    username: string;
    password: string;
  } | null>(null);
  const [deployments, setDeployments] = useState<AdminAgentDeployment[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [restoring, setRestoring] = useState(true);

  const adminClient = useMemo(() => {
    if (!credentials) return null;
    const authHeader = `Basic ${btoa(`${credentials.username}:${credentials.password}`)}`;
    return new ApiClient("", "", { Authorization: authHeader });
  }, [credentials]);

  const fetchDeployments = useCallback(async (user: string, pass: string) => {
    setLoading(true);
    setError("");
    try {
      const res = await api.adminListDeployments(user, pass);
      setDeployments(res.deployments || []);
      setCredentials({ username: user, password: pass });
      saveSession(user, pass);
    } catch (e: unknown) {
      const err = e as { error?: string; error_description?: string };
      if (err.error_description?.includes("401") || err.error === "request_failed") {
        setError("Invalid credentials");
        setCredentials(null);
        clearSession();
      } else {
        setError(err.error_description || err.error || "Request failed");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  // Restore session on mount
  useEffect(() => {
    const saved = loadSession();
    if (saved) {
      fetchDeployments(saved.username, saved.password).finally(() => setRestoring(false));
    } else {
      setRestoring(false);
    }
  }, [fetchDeployments]);

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault();
    fetchDeployments(username, password);
  };

  const handleRefresh = () => {
    if (credentials) {
      fetchDeployments(credentials.username, credentials.password);
    }
  };

  const handleLogout = () => {
    clearSession();
    setCredentials(null);
    setDeployments([]);
    setUsername("");
    setPassword("");
  };

  if (restoring) {
    return (
      <div className="flex items-center justify-center py-24">
        <RefreshCw size={20} className="animate-spin text-stone-400" />
      </div>
    );
  }

  if (!credentials || !adminClient) {
    return (
      <div className="p-6 md:p-8 max-w-md mx-auto mt-16">
        <div className="border border-stone-300 bg-white p-8">
          <div className="flex items-center gap-2 mb-6">
            <Lock size={20} className="text-stone-500" />
            <h1 className="text-xl font-semibold">Admin</h1>
          </div>

          {error && (
            <div className="flex items-center gap-2 p-3 mb-4 bg-red-50 border border-red-200 text-red-700 text-sm">
              <AlertCircle size={16} />
              {error}
            </div>
          )}

          <form onSubmit={handleLogin}>
            <label className="block text-sm text-stone-600 mb-1">Username</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full border border-stone-300 px-3 py-2 text-sm mb-4 outline-none focus:border-stone-500"
              autoComplete="username"
              required
            />
            <label className="block text-sm text-stone-600 mb-1">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full border border-stone-300 px-3 py-2 text-sm mb-6 outline-none focus:border-stone-500"
              autoComplete="current-password"
              required
            />
            <button
              type="submit"
              disabled={loading}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 border border-stone-300 bg-white text-sm text-stone-700 hover:bg-stone-50 disabled:opacity-50 cursor-pointer"
            >
              {loading && <RefreshCw size={14} className="animate-spin" />}
              Sign In
            </button>
          </form>
        </div>
      </div>
    );
  }

  return (
    <ApiClientProvider value={adminClient}>
      <div className="p-6 md:p-8">
        <div className="flex justify-between items-center mb-6">
          <div>
            <h1 className="text-2xl font-semibold">All Deployments</h1>
            <p className="text-stone-600 text-sm mt-1">
              {deployments.length} active deployment{deployments.length !== 1 ? "s" : ""} across all accounts
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleRefresh}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-2 border border-stone-300 bg-white text-sm text-stone-700 hover:bg-stone-50 disabled:opacity-50 cursor-pointer"
            >
              <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
              Refresh
            </button>
            <button
              onClick={handleLogout}
              className="flex items-center gap-2 px-4 py-2 border border-stone-300 bg-white text-sm text-stone-500 hover:bg-stone-50 cursor-pointer"
              title="Sign out"
            >
              <LogOut size={14} />
            </button>
          </div>
        </div>

        {error && (
          <div className="flex items-center gap-2 p-3 mb-4 bg-red-50 border border-red-200 text-red-700 text-sm">
            <AlertCircle size={16} />
            {error}
          </div>
        )}

        {deployments.length === 0 ? (
          <div className="p-8 border border-stone-300 text-center">
            <Activity size={32} className="mx-auto text-stone-400 mb-2" />
            <p className="text-stone-500">No active deployments found.</p>
          </div>
        ) : (
          <div className="space-y-3">
            {deployments.map((d) => (
              <DeploymentCard
                key={`${d.account_name}:${d.name}:${d.build_id}`}
                accountName={d.account_name}
                deployment={d}
                onRefresh={handleRefresh}
              />
            ))}
          </div>
        )}
      </div>
    </ApiClientProvider>
  );
}
