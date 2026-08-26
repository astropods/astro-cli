import { useParams, type MetaFunction } from "react-router";
import { AppsPanel } from "./AppsSettings";

export const meta: MetaFunction = () => [
  { title: "OAuth apps - Organization Settings | Astro" },
];

export default function OrgAppsSettings() {
  const { orgSlug = "" } = useParams();
  return <AppsPanel account={orgSlug} />;
}
