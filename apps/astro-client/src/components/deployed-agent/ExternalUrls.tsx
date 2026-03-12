import { useState } from "react";
import { Link as LinkIcon, Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ServiceEndpointInfo } from "@/lib/api";

export function ExternalUrls({ urls }: { urls: ServiceEndpointInfo[] }) {
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const handleCopy = (url: string) => {
    navigator.clipboard.writeText(url);
    setCopiedUrl(url);
    setTimeout(() => setCopiedUrl(null), 2000);
  };

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
          <Button
            variant="outline"
            size="sm"
            className="shrink-0 ml-auto"
            onClick={() => handleCopy(ep.url)}
          >
            {copiedUrl === ep.url ? (
              <><Check className="size-3" /> Copied</>
            ) : (
              <><Copy className="size-3" /> Copy</>
            )}
          </Button>
        </div>
      ))}
    </div>
  );
}
