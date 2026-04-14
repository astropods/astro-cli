import { useParams, type MetaFunction } from 'react-router'
import { VaultSettings } from './SecretsSettings'

export const meta: MetaFunction = () => [{ title: "Secrets - Organization Settings | Astro" }];

export default function OrgSecretsSettings() {
  const { orgSlug = '' } = useParams()
  return <VaultSettings account={orgSlug} />
}
