export type VaultEntryType = 'secret' | 'variable'

export interface VaultEntry {
  name: string
  type: VaultEntryType
  description?: string
  updatedAt: string
  value?: string // variables only — secrets never store a readable value
}

/** Uppercase letters, digits, underscores; must start with a letter. */
export const VARIABLE_NAME_PATTERN = /^[A-Z][A-Z0-9_]*$/
