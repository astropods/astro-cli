export type VaultEntryType = 'secret' | 'variable'

export interface VaultEntry {
  name: string
  type: VaultEntryType
  description?: string
  updatedAt: string
  value?: string // variables only — secrets never store a readable value
}
