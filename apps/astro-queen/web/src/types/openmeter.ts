export interface Meter {
  id: string;
  slug: string;
  name: string;
  description: string;
  eventType: string;
  aggregation: string;
  valueProperty: string;
  groupBy: Record<string, string>;
  windowSize: string;
  createdAt: string;
  updatedAt: string;
}

export interface MeterQueryResult {
  data: MeterDataPoint[];
  from: string;
  to: string;
  windowSize: string;
}

export interface MeterDataPoint {
  value: number;
  windowStart: string;
  windowEnd: string;
  subject: string;
  groupBy: Record<string, string>;
}

export interface Feature {
  id: string;
  key: string;
  name: string;
  meterSlug: string;
  meterGroupByFilters: Record<string, string>;
  metadata: Record<string, string>;
  createdAt: string;
  updatedAt: string;
  archivedAt: string;
}

export interface Customer {
  id: string;
  name: string;
  key: string;
  description: string;
  primaryEmail: string;
  currency: string;
  subjects: string[];
  currentSubscriptionId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Entitlement {
  id: string;
  featureId: string;
  featureKey: string;
  subjectKey: string;
  customerId?: string;
  type: string;
  isSoftLimit?: boolean;
  usagePeriod?: { interval: string; anchor: string };
  currentUsagePeriod?: { from: string; to: string };
  measureUsageFrom?: string;
  activeFrom?: string;
  activeTo?: string;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface EntitlementValue {
  hasAccess: boolean;
  balance: number;
  usage: number;
  overage: number;
}

export interface Grant {
  id: string;
  entitlementId: string;
  amount: number;
  priority?: number;
  effectiveAt: string;
  expiration?: { duration: string; count: number };
  maxRolloverAmount?: number;
  minRolloverAmount?: number;
  recurrence?: { interval: string; anchor: string };
  expiresAt?: string;
  voidedAt?: string;
  nextRecurrence?: string;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface Plan {
  id: string;
  name: string;
  description?: string;
  key: string;
  version: number;
  currency: string;
  billingCadence: string;
  status: string;
  phases: PlanPhase[];
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface PlanPhase {
  key: string;
  name: string;
  description?: string;
  duration?: string;
  rateCards: RateCard[];
}

export interface RateCard {
  type: string;
  key: string;
  name: string;
  featureKey?: string;
  billingCadence?: string;
  entitlementTemplate?: Record<string, unknown>;
  price?: Record<string, unknown>;
}

export interface Subscription {
  id: string;
  name: string;
  description?: string;
  status: string;
  customerId: string;
  plan?: { key: string; version: number };
  currency: string;
  billingCadence: string;
  activeFrom: string;
  activeTo?: string;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface SubscriptionItem {
  id: string;
  key: string;
  name: string;
  description?: string;
  featureKey?: string;
  billingCadence: string;
  activeFrom: string;
  activeTo?: string;
  price?: Record<string, unknown>;
  entitlementTemplate?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface SubscriptionPhase {
  id: string;
  key: string;
  name: string;
  description?: string;
  activeFrom: string;
  activeTo?: string;
  items: SubscriptionItem[];
  createdAt: string;
  updatedAt: string;
}

export interface SubscriptionExpanded extends Subscription {
  phases: SubscriptionPhase[];
}

export interface CloudEvent {
  id: string;
  type: string;
  source: string;
  subject: string;
  time: string;
  data: Record<string, unknown>;
  ingestedAt: string;
}
