export interface TagsRowProps {
  tags?: string[];
}

// Tags Langfuse uses for routing/filtering rather than user-meaningful labels.
const HIDDEN_TAG_PREFIXES = ["deployment:"];

function isHiddenTag(tag: string): boolean {
  return HIDDEN_TAG_PREFIXES.some((p) => tag.startsWith(p));
}

export function TagsRow({ tags }: TagsRowProps) {
  const visible = tags?.filter((t) => !isHiddenTag(t)) ?? [];
  if (visible.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-1.5">
      {visible.map((tag) => (
        <span
          key={tag}
          className="inline-flex items-center rounded border border-border/60 bg-card px-2 py-0.5 font-mono text-label text-muted-foreground"
        >
          {tag}
        </span>
      ))}
    </div>
  );
}
