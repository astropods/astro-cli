import { Link as LinkIcon, Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { ServiceEndpointInfo } from "@/lib/api";

function CopyUrlButton({ url }: { url: string }) {
  const { copy, copied } = useCopyToClipboard(2000);
  return (
    <Button
      variant="outline"
      size="sm"
      className="shrink-0 ml-auto"
      onClick={() => { void copy(url); }}
    >
      {copied ? (
        <><Check className="size-3" /> Copied</>
      ) : (
        <><Copy className="size-3" /> Copy</>
      )}
    </Button>
  );
}

export function ExternalUrls({ urls }: { urls: ServiceEndpointInfo[] }) {
  return (
    <div className="flex flex-col gap-2 px-6 py-4">
      {urls.map((ep) => (
        <div
          key={ep.url}
          className="flex items-center gap-3 border border-border rounded-sm bg-background px-3 py-2 min-w-0"
        >
          <LinkIcon className="size-3.5 text-muted-foreground shrink-0" />
          <span className="text-sm font-medium shrink-0">{ep.name}</span>
          <a
            href={ep.url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground hover:underline font-mono truncate"
          >
            {ep.url}
          </a>
          <CopyUrlButton url={ep.url} />
        </div>
      ))}
    </div>
  );
}
