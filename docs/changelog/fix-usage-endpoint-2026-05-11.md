## Summary

The account usage endpoint returned a fixed set of four hardcoded meters (`compute`, `agent_builds`, `agent_deployments`, `agents`). Any meter present in a customer's subscription but not in that list was silently dropped. Knowledge store meters added recently were never surfaced.

## Design

The response shape changes from named fields to a `meters` map keyed by OpenMeter feature key:

```json
{
  "account_id": "...",
  "period_start": "...",
  "period_end": "...",
  "meters": {
    "compute":          { "usage": 1.5, "quota": 10 },
    "agents":           { "usage": 3,   "quota": 5  },
    "knowledge_stores": { "usage": 1,   "quota": 3  }
  }
}
```

The server now iterates all entitlements returned by `GetCustomerAccess` and includes every key in the map. No server-side filter list exists anymore.

The frontend `UsageSettings` page renders a `StatCard` per entry in `meters`, using a local `meterMeta` lookup for display labels and units. Unknown future meter keys fall back to the raw key as the label. The compute info box renders conditionally when `"compute"` is present in the response.

Components that only needed the compute meter (`DashboardStats`, `DeployFormFields`, `DeployBlueprint`) use `meters?.compute` directly.

## Migration

The `account_id/usage` response shape is a breaking change for any client reading the old named fields (`compute_unit_hours`, `agent_builds`, `active_deployments`, `active_agents`). The astro-client is updated in this PR. No database or infrastructure changes required.
