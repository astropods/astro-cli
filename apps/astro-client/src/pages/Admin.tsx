import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useAuth } from "@/lib/auth";

function AdminContent() {
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

export default function Admin() {
  return (
    <ProtectedRoute>
      <AdminContent />
    </ProtectedRoute>
  );
}
