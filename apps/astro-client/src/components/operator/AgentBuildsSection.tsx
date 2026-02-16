import { useState } from "react";
import {
  Loader2,
  Package,
  Globe,
  Lock,
  Tag,
  AlertTriangle,
  Code,
  ChevronDown,
} from "lucide-react";
import type { Agent, AgentVersion } from "../../lib/api";
import { useAgent } from "../../api/queries/agents";

export interface AgentBuildsSectionProps {
  accountName: string;
  agentName: string;
  onPublish: (agentName: string, buildId: string) => void;
}

export function AgentBuildsSection({ accountName, agentName, onPublish }: AgentBuildsSectionProps) {
  const { data: agent, isLoading } = useAgent(accountName, agentName);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={24} className="animate-spin text-stone-500" />
      </div>
    );
  }

  if (!agent || agent.versions.length === 0) {
    return (
      <div className="p-8 border border-stone-300 bg-stone-50 text-center">
        <Package size={32} className="mx-auto text-stone-400 mb-2" />
        <p className="text-stone-600 text-sm">No builds found</p>
      </div>
    );
  }

  return (
    <div className="border border-stone-300 bg-white">
      {agent.versions.map((version, index) => (
        <BuildRow
          key={version.build_id}
          version={version}
          isLatest={index === 0}
          agent={agent}
          onPublish={onPublish}
        />
      ))}
    </div>
  );
}

function BuildRow({
  version,
  isLatest,
  agent,
  onPublish,
}: {
  version: AgentVersion;
  isLatest: boolean;
  agent: Agent;
  onPublish: (agentName: string, buildId: string) => void;
}) {
  const [specOpen, setSpecOpen] = useState(false);
  const hasWarnings = version.validation_warnings && version.validation_warnings.length > 0;

  return (
    <div className="border-b border-stone-200 last:border-b-0">
      <div className="flex items-center gap-3 px-4 py-3">
        {/* Icon */}
        <Package size={16} className={isLatest ? "text-stone-700" : "text-stone-400"} />

        {/* Build info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm truncate">{version.build_id}</span>
            {isLatest && (
              <span className="px-1.5 py-0.5 text-xs bg-blue-50 border border-blue-200 text-blue-700 shrink-0">
                latest
              </span>
            )}
            {version.version ? (
              <span className="flex items-center gap-1 px-1.5 py-0.5 text-xs bg-green-50 border border-green-200 text-green-700 shrink-0">
                <Globe size={9} />
                {version.version}
              </span>
            ) : (
              <span className="flex items-center gap-1 px-1.5 py-0.5 text-xs bg-stone-100 border border-stone-200 text-stone-500 shrink-0">
                <Lock size={9} />
                private
              </span>
            )}
            {hasWarnings && (
              <span className="flex items-center gap-1 px-1.5 py-0.5 text-xs bg-amber-50 border border-amber-200 text-amber-700 shrink-0">
                <AlertTriangle size={9} />
                {version.validation_warnings!.length} warning{version.validation_warnings!.length !== 1 ? "s" : ""}
              </span>
            )}
          </div>
          <p className="text-xs text-stone-400 mt-0.5">
            {new Date(version.published_at).toLocaleString()}
          </p>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 shrink-0">
          <button
            onClick={() => setSpecOpen(!specOpen)}
            className="flex items-center gap-1 px-2 py-1 text-xs border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer text-stone-600"
          >
            {specOpen ? <ChevronDown size={12} /> : <Code size={12} />}
            Spec
          </button>
          {!version.version && (
            <button
              onClick={() => onPublish(agent.name, version.build_id)}
              className="flex items-center gap-1 px-2 py-1 text-xs border border-green-300 text-green-700 bg-white hover:bg-green-50 cursor-pointer"
            >
              <Tag size={10} />
              Publish
            </button>
          )}
        </div>
      </div>

      {/* Expandable spec */}
      {specOpen && (
        <div className="px-4 pb-3">
          <pre className="p-3 bg-stone-50 border border-stone-200 text-xs font-mono overflow-x-auto max-h-64 overflow-y-auto">
            {JSON.stringify(version.spec, null, 2)}
          </pre>
        </div>
      )}

      {/* Validation warnings detail */}
      {specOpen && hasWarnings && (
        <div className="px-4 pb-3">
          <div className="border border-amber-200 bg-amber-50 p-3">
            <ul className="space-y-1">
              {version.validation_warnings!.map((w, i) => (
                <li key={i} className="text-xs text-amber-800">
                  <span className="font-mono text-amber-600">{w.field}</span>{" "}
                  {w.message}
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  );
}
