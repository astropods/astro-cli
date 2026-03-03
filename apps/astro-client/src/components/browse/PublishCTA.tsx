import { Plus } from "lucide-react";

export function PublishCTA() {
  return (
    <a
      href="https://docs.astropods.ai"
      target="_blank"
      rel="noopener noreferrer"
      className="flex flex-col items-center justify-center gap-3 rounded-sm border border-dashed border-muted-foreground/30 p-6 transition-colors hover:bg-card-hover"
    >
      <div className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-primary/10 text-primary">
        <Plus className="size-5" />
      </div>
      <div className="flex flex-col gap-1 text-center">
        <h3 className="text-base font-semibold text-foreground">
          Your agent
        </h3>
        <p className="text-sm text-muted-foreground">
          Build your own agent using the Astro CLI.
        </p>
      </div>
    </a>
  );
}
