export interface MetadataListProps {
  entries: Array<{ label: string; value: React.ReactNode }>;
}

export function MetadataList({ entries }: MetadataListProps) {
  const visible = entries.filter((e) => e.value != null && e.value !== "");
  if (visible.length === 0) return null;

  return (
    <dl className="flex flex-col gap-2 rounded-md border border-border/40 px-4 py-3">
      {visible.map((e) => (
        <div
          key={e.label}
          className="grid grid-cols-[7rem_1fr] items-baseline gap-3"
        >
          <dt className="text-mono-sm text-muted-foreground">{e.label}</dt>
          <dd className="break-all font-mono text-body-sm text-foreground">
            {e.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
