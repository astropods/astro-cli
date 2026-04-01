import { useParams } from 'react-router'
import { VaultSettings } from './SecretsSettings'

export default function OrgSecretsSettings() {
  const { orgSlug = '' } = useParams()
  return <VaultSettings account={orgSlug} />
}
