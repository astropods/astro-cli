import { Loader2, Rocket, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { BuildUpdateBadge } from "@/components/BuildUpdateBadge";

export interface DeployFormActionBarProps {
  isDirty: boolean;
  changeCount: number;
  requiresRedeploy: boolean;
  showBuildUpgradeRedeploy?: boolean;
  currentBuildId?: string;
  latestBuildId?: string;
  isSaving: boolean;
  formId: string;
  onReset: () => void;
}

export function DeployFormActionBar({
  isDirty,
  changeCount,
  requiresRedeploy,
  showBuildUpgradeRedeploy = false,
  currentBuildId,
  latestBuildId,
  isSaving,
  formId,
  onReset,
}: DeployFormActionBarProps) {
  const isVisible = isDirty || showBuildUpgradeRedeploy;
  const isBuildOnlyRedeploy = showBuildUpgradeRedeploy && !isDirty;
  return (
    <div className="fixed bottom-0 left-0 right-0 md:left-[calc(9rem+1.5rem)] z-10 flex justify-center pb-4 pointer-events-none">
      <div
        className={`w-full max-w-[calc(36rem+2rem)] mx-6 md:mx-8 border border-border rounded-lg bg-background shadow-lg pointer-events-auto transition-all duration-200 ${
          isVisible
            ? "translate-y-0 opacity-100"
            : "translate-y-[calc(100%+1rem)] opacity-0"
        }`}
      >
        <div className="px-5 py-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-sm text-muted-foreground text-center sm:text-left">
            {isBuildOnlyRedeploy ? (
              <BuildUpdateBadge
                currentBuildId={currentBuildId}
                latestBuildId={latestBuildId}
                stacked
                availableLabel
                className="text-slate-700 bg-slate-50 border-slate-200 dark:text-slate-200 dark:bg-slate-800/40 dark:border-slate-600/30"
              />
            ) : (
              <span>{changeCount} pending {changeCount === 1 ? "update" : "updates"}</span>
            )}
          </div>
          <div className="flex gap-3 sm:shrink-0">
            {isDirty && (
              <Button
                type="button"
                variant="ghost"
                size="default"
                className="flex-1 sm:flex-none"
                onClick={onReset}
              >
                Cancel
              </Button>
            )}
            <Button
              type="submit"
              form={formId}
              size="default"
              disabled={isSaving}
              className="flex-1 sm:flex-none px-6 has-[>svg]:px-6"
            >
              {isSaving ? (
                <>
                  <Loader2 size={16} className="animate-spin" />
                  {requiresRedeploy || isBuildOnlyRedeploy ? "Redeploying..." : "Saving..."}
                </>
              ) : isBuildOnlyRedeploy ? (
                <>
                  <Rocket size={16} />
                  Redeploy
                </>
              ) : requiresRedeploy ? (
                <>
                  <Rocket size={16} />
                  Save &amp; Redeploy
                </>
              ) : (
                <>
                  <Save size={16} />
                  Save
                </>
              )}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
