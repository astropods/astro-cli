import { ArrowRight, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";

export function PublishCTA() {
  return (
    <div className="flex flex-col justify-between gap-3 rounded-sm border border-dashed border-border p-3">
      <div className="flex items-start gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-primary/10 text-primary">
          <Sparkles className="size-5" />
        </div>
        <div className="flex flex-col gap-1">
          <h3 className="text-base font-semibold text-foreground">
            Publish your own agent
          </h3>
          <p className="text-sm text-muted-foreground">
            Build your own agents using the Astro CLI.
          </p>
        </div>
      </div>
      <Button variant="outline" asChild className="w-full hover:bg-primary/5">
        <a href="https://docs.astromode.ai" target="_blank" rel="noopener noreferrer">
          Create agent
          <ArrowRight className="size-4" />
        </a>
      </Button>
    </div>
  );
}
