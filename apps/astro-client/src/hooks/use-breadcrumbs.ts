import { useLocation, useParams } from "react-router-dom";
import type { BreadcrumbItem } from "@/components/ContentHeader";

export function useBreadcrumbs(): BreadcrumbItem[] {
  const { pathname } = useLocation();
  const params = useParams();

  const segments = pathname.split("/").filter(Boolean);

  if (segments.length === 0) {
    return [];
  }

  const breadcrumbs: BreadcrumbItem[] = [];

  const first = segments[0];

  switch (first) {
    case "hire":
      if (segments.length === 1) {
        breadcrumbs.push({ label: "Hire Agents" });
      } else {
        breadcrumbs.push({ label: "Hire Agents", to: "/hire" });
        const slug = params.agentSlug ?? segments[1];
        breadcrumbs.push({ label: decodeURIComponent(slug) });
      }
      break;
    case "agents":
      breadcrumbs.push({ label: "My Agents" });
      break;
    case "operator":
      breadcrumbs.push({ label: "Operator" });
      break;
    case "request-agent":
      breadcrumbs.push({ label: "Request Agent" });
      break;
    default:
      breadcrumbs.push({ label: decodeURIComponent(first) });
      break;
  }

  return breadcrumbs;
}
