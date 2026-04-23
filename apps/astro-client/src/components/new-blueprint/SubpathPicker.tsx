import { useState, useRef, useEffect } from "react";
import { FolderOpen, Search, X, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { inputBase, inputFocusWithin } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useGitHubAccountDirs } from "@/api/queries/github";

type TakenSubpath = { subpath: string; agentName: string };

type SubpathPickerProps = {
  account: string;
  repo: string;
  branch: string;
  value: string;
  onChange: (value: string) => void;
  enabled?: boolean;
  takenSubpaths?: TakenSubpath[];
};

export function SubpathPicker({ account, repo, branch, value, onChange, enabled = true, takenSubpaths }: SubpathPickerProps) {
  const [dirOpen, setDirOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const { data: dirsData, isLoading } = useGitHubAccountDirs(account, repo, branch, {
    enabled: enabled && !!repo,
  });

  const filteredDirs = (dirsData?.dirs ?? []).filter(d =>
    value === "" || d.toLowerCase().includes(value.toLowerCase())
  );

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setDirOpen(false);
      }
    }
    document.addEventListener("mousedown", onMouseDown);
    return () => document.removeEventListener("mousedown", onMouseDown);
  }, []);

  return (
    <div ref={containerRef} className="space-y-1.5">
      <Label size="md" className="flex items-center gap-1.5">
        <FolderOpen className="size-3.5" />
        Subdirectory
        <span className="font-normal text-muted-foreground">(optional)</span>
      </Label>

      <div className={cn(inputBase, inputFocusWithin, "flex h-9 items-center gap-2 px-3")}>
        <Search className="size-3.5 text-muted-foreground shrink-0" />
        <input
          type="text"
          className="flex-1 min-w-0 bg-transparent border-none outline-none text-sm placeholder:text-muted-foreground"
          placeholder="e.g. services/my-agent"
          value={value}
          onChange={e => { onChange(e.target.value); setDirOpen(true); }}
          onFocus={() => setDirOpen(true)}
        />
        {value && (
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            onClick={() => onChange("")}
            className="shrink-0 text-muted-foreground hover:text-foreground"
          >
            <X className="size-3.5" />
          </Button>
        )}
      </div>

      <div className={cn(
        "grid transition-[grid-template-rows] duration-150 ease-out",
        dirOpen && filteredDirs.length > 0 ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}>
        <div className="overflow-hidden">
          <div className="mt-0.5 max-h-40 overflow-y-auto rounded-sm border border-border bg-background">
            {isLoading ? (
              <div className="flex items-center justify-center gap-2 py-4 text-sm text-muted-foreground">
                <Loader2 className="size-3.5 animate-spin" />
                Loading...
              </div>
            ) : (
              filteredDirs.map(dir => {
                const taken = takenSubpaths?.find(t => t.subpath === dir);
                return (
                  <Button
                    key={dir}
                    type="button"
                    variant="ghost"
                    disabled={!!taken}
                    onClick={() => { onChange(dir); setDirOpen(false); }}
                    className={cn(
                      "w-full justify-start h-auto px-3 py-2 font-medium rounded-none",
                      value === dir && "bg-primary/5",
                      taken && "opacity-50 cursor-not-allowed",
                    )}
                  >
                    <FolderOpen className="size-3.5 text-muted-foreground shrink-0" />
                    <span className="flex-1 text-left truncate">{dir}</span>
                    {taken && <span className="text-[10px] text-muted-foreground font-normal shrink-0">linked to {taken.agentName}</span>}
                  </Button>
                );
              })
            )}
          </div>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Only trigger builds when files inside this path change.
      </p>
    </div>
  );
}
