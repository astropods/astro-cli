import { useParams, type MetaFunction } from 'react-router'
import { IngestKeysPanel } from './ApiKeysSettings'

export const meta: MetaFunction = () => [{ title: "API Keys - Organization Settings | Astro" }];

export default function OrgApiKeysSettings() {
  const { orgSlug = '' } = useParams()
  return <IngestKeysPanel account={orgSlug} />
}
