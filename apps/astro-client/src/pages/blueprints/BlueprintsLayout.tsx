import { Outlet } from "react-router";
import { BlueprintsSidebar } from "@/components/browse/BlueprintsSidebar";
import {
  SidebarLayout,
  SidebarBody,
} from "@/components/ui/sidebar-layout";

export default function BlueprintsLayout() {
  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[1500px] mx-auto">
      <SidebarLayout>
        <BlueprintsSidebar />
        <SidebarBody>
          <Outlet />
        </SidebarBody>
      </SidebarLayout>
    </div>
  );
}
