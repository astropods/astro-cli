import { useState } from "react";
import { X, Loader2, Globe } from "lucide-react";

export interface PublishModalProps {
  accountName: string;
  agentName: string;
  buildId: string;
  onClose: () => void;
  onPublish: (version: string) => Promise<void>;
  isPublishing: boolean;
}

export function PublishModal({ accountName, agentName, buildId, onClose, onPublish, isPublishing }: PublishModalProps) {
  const [version, setVersion] = useState("");
  const semverValid = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([\w.-]+))?(?:\+([\w.-]+))?$/.test(version);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[400px] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
          <div>
            <h2 className="text-lg font-semibold"><span className="font-normal text-stone-500">{accountName}/</span>{agentName}</h2>
            <p className="text-sm text-stone-600 font-mono">{buildId}</p>
          </div>
          <button className="bg-transparent border-none cursor-pointer p-1" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="p-4">
          <p className="text-sm text-stone-600 mb-3">
            Assign a semver version to make this build publicly visible and deployable by anyone.
          </p>
          <label className="block text-sm font-medium text-stone-700 mb-1">Version</label>
          <input
            type="text"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            placeholder="1.0.0"
            className="w-full py-2 px-3 border border-stone-300 text-sm font-mono focus:outline-2 focus:outline-stone-800 focus:-outline-offset-2"
          />
          {version && !semverValid && (
            <p className="text-xs text-red-600 mt-1">Must be valid semver (e.g. 1.0.0)</p>
          )}
        </div>

        <div className="flex gap-2 p-4 border-t border-stone-300">
          <button
            type="button"
            onClick={onClose}
            className="flex-1 px-4 py-2 border border-stone-300 text-sm bg-white hover:bg-stone-50 cursor-pointer"
            disabled={isPublishing}
          >
            Cancel
          </button>
          <button
            onClick={() => onPublish(version)}
            disabled={!semverValid || isPublishing}
            className="flex-1 px-4 py-2 border border-green-700 text-sm bg-green-700 text-white hover:bg-green-600 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
          >
            {isPublishing ? (
              <>
                <Loader2 size={16} className="animate-spin" />
                Publishing...
              </>
            ) : (
              <>
                <Globe size={16} />
                Publish
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
