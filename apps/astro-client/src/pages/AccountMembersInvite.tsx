import { useParams } from "react-router";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { useAccount } from "../api/queries/accounts";

function AccountMembersInviteContent() {
  const { account } = useParams<{ account: string }>();
  const { data, isLoading } = useAccount(account ?? "");

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col p-6 md:p-8">
      <h1 className="text-2xl font-bold">Invite Member</h1>
      <p className="text-muted-foreground mt-2">
        Invite form for {data.name} will appear here.
      </p>
    </div>
  );
}

export default function AccountMembersInvite() {
  return (
    <ProtectedRoute>
      <AccountMembersInviteContent />
    </ProtectedRoute>
  );
}
