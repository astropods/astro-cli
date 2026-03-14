import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { openmeterKeys } from "./keys";
import type {
  Meter,
  Feature,
  Customer,
  Entitlement,
  EntitlementValue,
  Grant,
  Plan,
  Subscription,
  CloudEvent,
} from "@/types/openmeter";

// OpenAPI Schema
export function useOpenAPISchema() {
  return useQuery({
    queryKey: openmeterKeys.openapi(),
    queryFn: () => api.get<Record<string, unknown>>("/api/openapi.json"),
    staleTime: Infinity,
  });
}

// Meters
export function useMeters() {
  return useQuery({
    queryKey: openmeterKeys.meters(),
    queryFn: () => api.get<Meter[]>("/api/openmeter/api/v1/meters"),
  });
}

export function useMeter(id: string) {
  return useQuery({
    queryKey: openmeterKeys.meter(id),
    queryFn: () => api.get<Meter>(`/api/openmeter/api/v1/meters/${encodeURIComponent(id)}`),
    enabled: !!id,
  });
}

export function useCreateMeter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Meter>) =>
      api.post<Meter>("/api/openmeter/api/v1/meters", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.meters() });
    },
  });
}

export function useUpdateMeter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<Meter> }) =>
      api.put<Meter>(`/api/openmeter/api/v1/meters/${encodeURIComponent(id)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.meters() });
    },
  });
}

export function useDeleteMeter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/openmeter/api/v1/meters/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.meters() });
    },
  });
}

export function useQueryMeter() {
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: unknown }) =>
      api.post(`/api/openmeter/api/v1/meters/${encodeURIComponent(id)}/query`, body),
  });
}

export function useMeterGroupByValues(id: string, groupByKey: string) {
  return useQuery({
    queryKey: openmeterKeys.meterGroupBy(id, groupByKey),
    queryFn: () =>
      api.get(
        `/api/openmeter/api/v1/meters/${encodeURIComponent(id)}/group-by/${encodeURIComponent(groupByKey)}/values`
      ),
    enabled: !!id && !!groupByKey,
  });
}

// Features
export function useFeatures(includeArchived = false) {
  return useQuery({
    queryKey: openmeterKeys.features(),
    queryFn: () =>
      api.get<Feature[]>(
        `/api/openmeter/api/v1/features${includeArchived ? "?includeArchived=true" : ""}`
      ),
  });
}

export function useCreateFeature() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Feature>) =>
      api.post<Feature>("/api/openmeter/api/v1/features", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.features() });
    },
  });
}

export function useDeleteFeature() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/openmeter/api/v1/features/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.features() });
    },
  });
}

// Customers
export function useCustomers() {
  return useQuery({
    queryKey: openmeterKeys.customers(),
    queryFn: async () => {
      const res = await api.get<Customer[] | { items: Customer[] }>("/api/openmeter/api/v1/customers");
      return Array.isArray(res) ? res : res.items;
    },
  });
}

export function useCustomer(id: string) {
  return useQuery({
    queryKey: openmeterKeys.customer(id),
    queryFn: () =>
      api.get<Customer>(`/api/openmeter/api/v1/customers/${encodeURIComponent(id)}`),
    enabled: !!id,
  });
}

export function useUpdateCustomer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<Customer> }) =>
      api.put<Customer>(
        `/api/openmeter/api/v1/customers/${encodeURIComponent(id)}`,
        body
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: openmeterKeys.customers() });
      qc.invalidateQueries({ queryKey: openmeterKeys.customer(vars.id) });
    },
  });
}

export function useDeleteCustomer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/openmeter/api/v1/customers/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.customers() });
    },
  });
}

export function useCustomerAccess(id: string) {
  return useQuery({
    queryKey: openmeterKeys.customerAccess(id),
    queryFn: () =>
      api.get<{ entitlements: Record<string, Record<string, unknown>> }>(
        `/api/openmeter/api/v1/customers/${encodeURIComponent(id)}/access`
      ),
    enabled: !!id,
  });
}

export function useCustomerApps(id: string) {
  return useQuery({
    queryKey: openmeterKeys.customerApps(id),
    queryFn: () =>
      api.get(`/api/openmeter/api/v1/customers/${encodeURIComponent(id)}/apps`),
    enabled: !!id,
  });
}

export function useCustomerEntitlements(id: string) {
  return useQuery({
    queryKey: openmeterKeys.customerEntitlements(id),
    queryFn: async () => {
      const res = await api.get<{ items: Entitlement[] } | Entitlement[]>(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(id)}/entitlements`
      );
      return Array.isArray(res) ? res : res.items;
    },
    enabled: !!id,
  });
}

export function useCreateEntitlement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      customerId,
      body,
    }: {
      customerId: string;
      body: unknown;
    }) =>
      api.post(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(customerId)}/entitlements`,
        body
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({
        queryKey: openmeterKeys.customerEntitlements(vars.customerId),
      });
    },
  });
}

export function useDeleteEntitlement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      customerId,
      entitlementId,
    }: {
      customerId: string;
      entitlementId: string;
    }) =>
      api.del(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(customerId)}/entitlements/${encodeURIComponent(entitlementId)}`
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({
        queryKey: openmeterKeys.customerEntitlements(vars.customerId),
      });
    },
  });
}

export function useEntitlementValue(custId: string, entId: string) {
  return useQuery({
    queryKey: openmeterKeys.entitlementValue(custId, entId),
    queryFn: () =>
      api.get<EntitlementValue>(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(custId)}/entitlements/${encodeURIComponent(entId)}/value`
      ),
    enabled: !!custId && !!entId,
  });
}

export function useEntitlementGrants(custId: string, entId: string) {
  return useQuery({
    queryKey: openmeterKeys.entitlementGrants(custId, entId),
    queryFn: async () => {
      const res = await api.get<{ items: Grant[] } | Grant[]>(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(custId)}/entitlements/${encodeURIComponent(entId)}/grants`
      );
      return Array.isArray(res) ? res : res.items;
    },
    enabled: !!custId && !!entId,
  });
}

export function useCreateGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      customerId,
      entitlementId,
      body,
    }: {
      customerId: string;
      entitlementId: string;
      body: unknown;
    }) =>
      api.post(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(customerId)}/entitlements/${encodeURIComponent(entitlementId)}/grants`,
        body
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({
        queryKey: openmeterKeys.entitlementGrants(
          vars.customerId,
          vars.entitlementId
        ),
      });
    },
  });
}

export function useResetEntitlement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      customerId,
      entitlementId,
      body,
    }: {
      customerId: string;
      entitlementId: string;
      body: unknown;
    }) =>
      api.post(
        `/api/openmeter/api/v2/customers/${encodeURIComponent(customerId)}/entitlements/${encodeURIComponent(entitlementId)}/reset`,
        body
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({
        queryKey: openmeterKeys.entitlementValue(
          vars.customerId,
          vars.entitlementId
        ),
      });
    },
  });
}

// Plans
export function usePlans() {
  return useQuery({
    queryKey: openmeterKeys.plans(),
    queryFn: async () => {
      const res = await api.get<Plan[] | { items: Plan[] }>(
        "/api/openmeter/api/v1/plans"
      );
      return Array.isArray(res) ? res : res.items;
    },
  });
}

export function usePlan(id: string) {
  return useQuery({
    queryKey: openmeterKeys.plan(id),
    queryFn: () => api.get<Plan>(`/api/openmeter/api/v1/plans/${encodeURIComponent(id)}`),
    enabled: !!id,
  });
}

export function useCreatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) =>
      api.post<Plan>("/api/openmeter/api/v1/plans", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.plans() });
    },
  });
}

export function useDeletePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/openmeter/api/v1/plans/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.plans() });
    },
  });
}

export function usePublishPlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/api/openmeter/api/v1/plans/${encodeURIComponent(id)}/publish`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.plans() });
    },
  });
}

export function useArchivePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/api/openmeter/api/v1/plans/${encodeURIComponent(id)}/archive`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.plans() });
    },
  });
}

export function useUpdatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: unknown }) =>
      api.put<Plan>(`/api/openmeter/api/v1/plans/${encodeURIComponent(id)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.plans() });
    },
  });
}

// Subscriptions
export function useCreateSubscription() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) =>
      api.post<Subscription>("/api/openmeter/api/v1/subscriptions", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.customers() });
    },
  });
}

export function useSubscription(id: string) {
  return useQuery({
    queryKey: openmeterKeys.subscription(id),
    queryFn: () =>
      api.get<Subscription>(
        `/api/openmeter/api/v1/subscriptions/${encodeURIComponent(id)}`
      ),
    enabled: !!id,
  });
}

export function useCancelSubscription() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: unknown }) =>
      api.post(
        `/api/openmeter/api/v1/subscriptions/${encodeURIComponent(id)}/cancel`,
        body
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.customers() });
    },
  });
}

// Entitlements (global)
export function useEntitlements() {
  return useQuery({
    queryKey: openmeterKeys.entitlements(),
    queryFn: async () => {
      const res = await api.get<Entitlement[] | { items: Entitlement[] }>(
        "/api/openmeter/api/v2/entitlements"
      );
      return Array.isArray(res) ? res : res.items;
    },
  });
}

// Grants (global)
export function useGrants() {
  return useQuery({
    queryKey: openmeterKeys.grants(),
    queryFn: async () => {
      const res = await api.get<Grant[] | { items: Grant[] }>(
        "/api/openmeter/api/v2/grants"
      );
      return Array.isArray(res) ? res : res.items;
    },
  });
}

// Events
interface EventWrapper {
  event: CloudEvent;
  ingestedAt: string;
  storedAt: string;
  validationError?: string;
}

export function useEvents(params?: string) {
  return useQuery({
    queryKey: openmeterKeys.events(),
    queryFn: async () => {
      const raw = await api.get<EventWrapper[]>(
        `/api/openmeter/api/v1/events${params ? `?${params}` : ""}`
      );
      return raw.map((w) => ({ ...w.event, ingestedAt: w.ingestedAt }));
    },
  });
}

export function useIngestEvent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) => api.post("/api/openmeter/api/v1/events", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: openmeterKeys.events() });
    },
  });
}
