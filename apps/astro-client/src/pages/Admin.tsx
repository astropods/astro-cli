import type { MetaFunction } from "react-router";
import { useAuth } from "@/lib/auth";

export const meta: MetaFunction = () => [{ title: "Admin | Astro" }];

export default function Admin() {
  const { hasPermission } = useAuth();

  if (!hasPermission("admin:view")) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">
          You don't have permission to view this page.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <p className="text-muted-foreground">Admin dashboard coming soon.</p>
    </div>
  );
}
