import { useConnectedDevices } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime } from "@/lib/utils";

export function ConnectedDevicesPage() {
  const { data, isLoading, error } = useConnectedDevices();

  const connected = data?.devices?.filter((d) => d.status === "connected") ?? [];
  const disconnected = data?.devices?.filter((d) => d.status !== "connected") ?? [];

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">Connected Devices</h2>
        {data && (
          <span className="text-xs text-muted-foreground">
            {connected.length} online / {data.count} total
          </span>
        )}
      </div>

      {isLoading && <TableSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}

      {data && (
        <>
          {connected.length > 0 && (
            <div className="mb-6">
              <h3 className="mb-2 text-sm font-medium text-green-600">Online</h3>
              <DeviceTable devices={connected} />
            </div>
          )}

          {disconnected.length > 0 && (
            <div>
              <h3 className="mb-2 text-sm font-medium text-muted-foreground">Offline</h3>
              <DeviceTable devices={disconnected} />
            </div>
          )}

          {data.count === 0 && (
            <div className="rounded-lg glass p-8 text-center text-sm text-muted-foreground">
              No devices have connected yet. Run <code className="rounded bg-glass-light px-1.5 py-0.5 font-mono text-xs">ast connect</code> to register a device.
            </div>
          )}
        </>
      )}
    </div>
  );
}

function DeviceTable({ devices }: { devices: NonNullable<ReturnType<typeof useConnectedDevices>["data"]>["devices"] }) {
  return (
    <div className="overflow-x-auto rounded-lg glass">
      <table className="w-full text-[11px] whitespace-nowrap">
        <thead>
          <tr className="border-b border-glass-border-honey glass-subtle">
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Status</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Hostname</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Account</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">OS / Arch</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">CLI Version</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Last Heartbeat</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Connected</th>
            <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Device ID</th>
          </tr>
        </thead>
        <tbody>
          {devices.map((d) => (
            <tr key={d.id} className="border-b border-comb-light hover:bg-glass-light">
              <td className="px-2 py-0.5">
                <StatusDot status={d.status} />
              </td>
              <td className="px-2 py-0.5 font-medium">{d.hostname || "-"}</td>
              <td className="px-2 py-0.5 text-muted-foreground">{d.account_name || "-"}</td>
              <td className="px-2 py-0.5 text-muted-foreground">
                {d.os}/{d.arch}
              </td>
              <td className="px-2 py-0.5 font-mono text-muted-foreground">{d.cli_version || "-"}</td>
              <td className="px-2 py-0.5 text-muted-foreground">
                {d.last_heartbeat_at ? formatDateTime(d.last_heartbeat_at) : "-"}
              </td>
              <td className="px-2 py-0.5 text-muted-foreground">
                {d.connected_at ? formatDateTime(d.connected_at) : "-"}
              </td>
              <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">
                {d.device_id.slice(0, 12)}...
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  const isConnected = status === "connected";
  return (
    <span className="flex items-center gap-1">
      <span
        className={`inline-block size-1.5 rounded-full ${
          isConnected ? "bg-green-500 shadow-[0_0_4px_rgba(34,197,94,0.5)]" : "bg-gray-400"
        }`}
      />
      <span className={`text-xs ${isConnected ? "text-green-600" : "text-muted-foreground"}`}>
        {isConnected ? "online" : "offline"}
      </span>
    </span>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
