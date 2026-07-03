import { Link } from "react-router";
import { AlertCircle, ArrowRight, MessagesSquare, PlugZap, RotateCw, Telescope } from "lucide-react";
import { RocketLaunchIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { AstroLogoLoader } from "@/components/ui/astro-logo-loader";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { blueprintsAccountPath, dashboardPath, explorePath } from "@/lib/routes";
import { cn } from "@/lib/utils";

const CHAT_HEADER_DESCRIPTION = "Chat with your deployed agents.";

export function ChatEmptyState({
  variant,
  account,
  className,
  onRetry,
}: {
  variant: "loading" | "no-chat-agents" | "agents-not-chattable" | "error";
  account: string;
  className?: string;
  onRetry?: () => void;
}) {
  if (variant === "loading") {
    // Full-center, no header — loader reveals before the page shell.
    return (
      <div
        className={cn(
          "flex min-h-0 flex-col items-center justify-center gap-5 bg-background px-6 py-16",
          className,
        )}
      >
        <AstroLogoLoader size={52} />
        <div className="text-center">
          <p className="text-sm font-medium text-foreground">
            Getting your chat ready
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Checking which agents are available to chat
          </p>
        </div>
      </div>
    );
  }

  if (variant === "agents-not-chattable") {
    return (
      <PageContainer outerClassName={cn("bg-background", className)}>
        <PageHeader title="Chat" description={CHAT_HEADER_DESCRIPTION} />
        <NotChattableCard />
      </PageContainer>
    );
  }

  if (variant === "error") {
    return (
      <PageContainer outerClassName={cn("bg-background", className)}>
        <PageHeader title="Chat" description={CHAT_HEADER_DESCRIPTION} />
        <ChatErrorCard onRetry={onRetry} />
      </PageContainer>
    );
  }

  return <NoAgentsState account={account} className={className} />;
}

function NoAgentsState({
  account,
  className,
}: {
  account: string;
  className?: string;
}) {
  const { data, isLoading } = useAccountBlueprints(account, {
    enabled: !!account,
  });

  const hasBlueprints =
    (data?.agents ?? []).filter((b) => !b.archived_at).length > 0;

  return (
    <PageContainer outerClassName={cn("bg-background", className)}>
      <PageHeader title="Chat" description={CHAT_HEADER_DESCRIPTION} />
      {isLoading ? (
        <div className="flex min-h-[240px] items-center justify-center">
          <Spinner delay={300} />
        </div>
      ) : (
        <ChatEmptyCard account={account} hasBlueprints={hasBlueprints} />
      )}
    </PageContainer>
  );
}

/** CTA routes to the account's own blueprints if any exist, otherwise to Explore. */
function ChatEmptyCard({
  account,
  hasBlueprints,
}: {
  account: string;
  hasBlueprints: boolean;
}) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="mb-3 flex justify-center text-muted-foreground">
        <MessagesSquare className="size-6" strokeWidth={1.5} />
      </div>
      <p className="text-sm font-medium text-foreground">
        No agents to chat with yet
      </p>
      <p className="mx-auto mb-4 mt-1 max-w-sm text-xs text-muted-foreground">
        {hasBlueprints
          ? "Pick one of your blueprints and deploy it. Agents with web messaging enabled will appear here."
          : "Find a community blueprint on Explore and deploy it. Agents with web messaging enabled will appear here."}
      </p>
      <div className="flex flex-wrap items-center justify-center gap-3">
        <Button asChild className="group">
          <Link to={hasBlueprints ? blueprintsAccountPath(account) : explorePath}>
            {hasBlueprints ? (
              <RocketLaunchIcon className="size-4 transition-transform duration-300 group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
            ) : (
              <Telescope className="size-4 transition-transform duration-300 group-hover:rotate-12" strokeWidth={1.5} />
            )}
            {hasBlueprints ? "Deploy a blueprint" : "Explore blueprints"}
          </Link>
        </Button>
      </div>
    </div>
  );
}

/** Eligibility reads failed — agent state is unknown, so offer retry not a deploy nudge. */
function ChatErrorCard({ onRetry }: { onRetry?: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="mb-3 flex justify-center text-muted-foreground">
        <AlertCircle className="size-6" strokeWidth={1.5} />
      </div>
      <p className="text-sm font-medium text-foreground">
        Couldn't load your agents
      </p>
      <p className="mx-auto mb-4 mt-1 max-w-sm text-xs text-muted-foreground">
        Something went wrong while checking which agents you can chat with. Try
        again in a moment.
      </p>
      {onRetry ? (
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button onClick={onRetry}>
            <RotateCw className="size-4" strokeWidth={1.5} />
            Try again
          </Button>
        </div>
      ) : null}
    </div>
  );
}

/** Agents exist but none expose web chat — routes to agents to enable messaging. */
function NotChattableCard() {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="mb-3 flex justify-center text-muted-foreground">
        <PlugZap className="size-6" strokeWidth={1.5} />
      </div>
      <p className="text-sm font-medium text-foreground">
        No agents are connected to chat yet
      </p>
      <p className="mx-auto mb-4 mt-1 max-w-sm text-xs text-muted-foreground">
        You have agents deployed, but none have web messaging turned on.
        Enable it on an agent to start a conversation here.
      </p>
      <div className="flex flex-wrap items-center justify-center gap-3">
        <Button asChild className="group">
          <Link to={dashboardPath}>
            Go to agents
            <ArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-0.5" strokeWidth={1.5} />
          </Link>
        </Button>
      </div>
    </div>
  );
}
