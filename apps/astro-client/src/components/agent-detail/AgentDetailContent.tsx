import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { ShieldCheck, FileText } from "lucide-react";
import { PrivacyBadge } from "@/components/PrivacyBadge";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { InlineBadge } from "@/components/InlineBadge";
import { AgentIdentity } from "@/components/AgentIdentity";

export interface AgentDetailContentProps {
  account: string;
  name: string;
  visibility?: string;
  summary?: string;
  categories: string[];
  readme?: string;
  safetyPermissions: string[];
  mobileSidebar?: ReactNode;
}

export function AgentDetailContent({
  account,
  name,
  visibility,
  summary,
  categories,
  readme,
  safetyPermissions,
  mobileSidebar,
}: AgentDetailContentProps) {
  const readmeContent = (() => {
    if (!readme) return readme;

    const lines = readme.split("\n");
    const headingIndices: number[] = [];
    for (let i = 0; i < lines.length; i += 1) {
      if (/^\s{0,3}#{1,6}\s+\S+/.test(lines[i])) {
        headingIndices.push(i);
      }
    }

    // TODO: Remove this heuristic once API returns resume-only markdown content.
    // Keep content unchanged unless we can safely remove one intro section.
    if (headingIndices.length < 2) return readme;

    const secondHeadingLine = headingIndices[1];
    const sliced = lines.slice(secondHeadingLine).join("\n").trim();
    return sliced || readme;
  })();

  const resumeScrollRef = useRef<HTMLDivElement | null>(null);
  const [showResumeHint, setShowResumeHint] = useState(false);

  useEffect(() => {
    const el = resumeScrollRef.current;
    if (!el || !readmeContent) return;

    const updateResumeHint = () => {
      const canScroll = el.scrollHeight - el.clientHeight > 4;
      const atTop = el.scrollTop <= 2;
      setShowResumeHint(canScroll && atTop);
    };

    updateResumeHint();
    el.addEventListener("scroll", updateResumeHint, { passive: true });

    const resizeObserver =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(updateResumeHint)
        : null;
    resizeObserver?.observe(el);
    window.addEventListener("resize", updateResumeHint);

    return () => {
      el.removeEventListener("scroll", updateResumeHint);
      resizeObserver?.disconnect();
      window.removeEventListener("resize", updateResumeHint);
    };
  }, [readmeContent]);

  return (
    <div className="flex-1 min-w-0 p-6 md:p-8">
      {/* Header */}
      <header className="mb-6 border-b border-border-strong pb-6">
        <div className="flex items-start gap-4">
          <AgentIdentity
            account={account}
            name={name}
            size={56}
            className="size-14 shrink-0 rounded-md overflow-hidden border border-stone-200 dark:border-border"
          />
          <div className="min-w-0">
            <h1 className="flex flex-wrap items-center gap-2 font-mono text-xl font-bold text-foreground">
              {name}
              {visibility === "private" && <PrivacyBadge />}
            </h1>
            <p className="font-mono text-body-sm text-muted-foreground mt-0.5">
              @{account}
            </p>
          </div>
        </div>
        {summary && (
          <p className="mt-4 max-w-4xl text-[14px] leading-[1.65] text-foreground/85">
            {summary}
          </p>
        )}
        {/* Category tags */}
        {categories.length > 0 && (
          <div className="mt-3.5 flex flex-wrap gap-2">
            {categories.map((tag) => (
              <InlineBadge key={tag}>{tag}</InlineBadge>
            ))}
          </div>
        )}
      </header>

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="min-[900px]:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* README */}
      {readmeContent && (
        <section className="mb-8 overflow-hidden rounded-md border border-border-strong bg-surface">
          <div className="flex items-center gap-2 border-b border-border-strong bg-stone-200 px-4 py-2.5 dark:bg-muted/30">
            <FileText className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
              ReadMe
            </span>
          </div>
          <div className="relative">
            <div ref={resumeScrollRef} className="max-h-[640px] overflow-y-auto px-6 py-5">
              <StyledMarkdown className="prose-headings:font-mono prose-p:font-mono prose-li:font-mono prose-a:font-mono prose-strong:font-mono prose-th:font-mono prose-td:font-mono [&>h1:first-child]:mt-0 [&>h2:first-child]:mt-0 [&>h3:first-child]:mt-0">
                {readmeContent}
              </StyledMarkdown>
              {safetyPermissions.length > 0 && (
                <section className="mt-6 border-t border-border-strong pt-5 font-mono">
                  <h2 className="mb-3 text-[15px] font-bold text-foreground">
                    Safety & Permissions
                  </h2>
                  <ul className="space-y-2.5">
                    {safetyPermissions.map((permission, i) => (
                      <li key={i} className="flex items-start gap-2.5 text-[13px] leading-relaxed">
                        <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">{permission}</span>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
            </div>
            {showResumeHint && (
              <div className="pointer-events-none absolute inset-x-0 bottom-0">
                <div className="h-16 bg-gradient-to-t from-surface via-surface/90 to-transparent" />
                <span className="absolute bottom-2 left-1/2 -translate-x-1/2 text-[11px] leading-4 font-mono uppercase tracking-[0.12em] text-muted-foreground">
                  Scroll to read more
                </span>
              </div>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
