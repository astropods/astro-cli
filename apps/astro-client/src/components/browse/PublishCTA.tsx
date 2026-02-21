import { Sparkles } from "lucide-react";

export function PublishCTA() {
  return (
    <a
      href="https://docs.astropod.ai"
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-start gap-3 rounded-lg border border-dashed border-border p-4 transition-colors hover:bg-muted/50"
    >
      <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
        <Sparkles className="size-5" />
      </div>
      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-semibold text-foreground">
          Publish your own agent
        </h3>
        <p className="text-xs text-muted-foreground">
          Build and share agents on the Astro marketplace.
        </p>
      </div>
    </a>
  );
}
