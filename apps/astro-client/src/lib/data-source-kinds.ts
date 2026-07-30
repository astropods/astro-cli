/**
 * Registry of external data-source kinds shown in Settings → Data Sources.
 *
 * Claude Code is the only kind today, but more (Codex, etc.) are coming. Add a
 * new tool here and the table picks up its icon + name automatically — `icon`
 * is an `@astropods/brand-icons` key (resolved to a themed logo via
 * `getIntegrationIconUrl`) and `label` is the full display name shown in the
 * icon's tooltip.
 */
export interface DataSourceKind {
  /** Full display name, shown in the table's tooltip. */
  label: string
  /** Brand-icon key resolved to a themed logo. */
  icon: string
}

export const DATA_SOURCE_KINDS: Record<string, DataSourceKind> = {
  'claude-code': { label: 'Claude Code', icon: 'claude-code' },
  codex: { label: 'Codex', icon: 'openai' },
}

/** Assumed when a source carries no explicit type — every source today. */
export const DEFAULT_DATA_SOURCE_KIND = 'claude-code'

/** Resolves a source's type to its kind, falling back to the default. */
export function resolveDataSourceKind(kind?: string | null): DataSourceKind {
  return (kind && DATA_SOURCE_KINDS[kind]) || DATA_SOURCE_KINDS[DEFAULT_DATA_SOURCE_KIND]
}
