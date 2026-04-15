export type VaultEntryType = 'secret' | 'variable'

export interface VaultEntry {
  name: string
  type: VaultEntryType
  description?: string
  updatedAt: string
  value?: string // variables only — secrets never store a readable value
}

/** Letters, digits, underscores; must start with a letter or underscore. */
export const VARIABLE_NAME_PATTERN = /^[a-zA-Z_][a-zA-Z0-9_]*$/
