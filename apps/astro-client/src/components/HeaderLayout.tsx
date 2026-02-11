import { Outlet, useNavigate, useOutletContext } from "react-router-dom";
import { BreadcrumbHeader } from "./BreadcrumbHeader";
import { useBreadcrumbs } from "@/hooks/use-breadcrumbs";
import type { LayoutContext } from "./Layout";

export function HeaderLayout() {
  const context = useOutletContext<LayoutContext>();
  const navigate = useNavigate();
  const breadcrumbs = useBreadcrumbs();

  return (
    <>
      <BreadcrumbHeader
        breadcrumbs={breadcrumbs}
        onBack={() => navigate(-1)}
        onForward={() => navigate(1)}
      />
      <div className="flex flex-1 flex-col min-h-0 p-6 md:p-8">
        <Outlet context={context} />
      </div>
    </>
  );
}
