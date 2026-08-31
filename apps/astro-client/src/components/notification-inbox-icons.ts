import {
  AlertCircle,
  Bell,
  CircleCheck,
  Info,
  PartyPopper,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'

export type NotificationIconTone = 'neutral' | 'primary' | 'info' | 'warning' | 'critical' | 'success'

export interface NotificationIconConfig {
  icon: LucideIcon
  tone: NotificationIconTone
}

const DEFAULT_NOTIFICATION_ICON: NotificationIconConfig = { icon: Bell, tone: 'neutral' }

const NOTIFICATION_ICONS: Record<string, NotificationIconConfig> = {
  'system.test': { icon: Bell, tone: 'neutral' },
  'account.welcome': { icon: PartyPopper, tone: 'primary' },
  'build.failed': { icon: AlertCircle, tone: 'critical' },
  'billing.payment_failed': { icon: AlertCircle, tone: 'critical' },
  'billing.action_required': { icon: TriangleAlert, tone: 'warning' },
  'billing.spend_threshold': { icon: AlertCircle, tone: 'critical' },
  'billing.spend_warning': { icon: TriangleAlert, tone: 'warning' },
  'billing.usage_warning': { icon: TriangleAlert, tone: 'warning' },
  'billing.usage_limit': { icon: AlertCircle, tone: 'critical' },
  'billing.credits_exhausted': { icon: AlertCircle, tone: 'critical' },
  'billing.dunning_suspended': { icon: AlertCircle, tone: 'critical' },
  'billing.recovered': { icon: CircleCheck, tone: 'success' },
  'team.member_changed': { icon: Bell, tone: 'neutral' },
  'account.ownership_transferred': { icon: Bell, tone: 'neutral' },
  'security.key_changed': { icon: TriangleAlert, tone: 'warning' },
  'observation.info': { icon: Info, tone: 'info' },
  'observation.warning': { icon: TriangleAlert, tone: 'warning' },
  'observation.critical': { icon: AlertCircle, tone: 'critical' },
}

export function resolveNotificationIcon(workflowIdentifier?: string): NotificationIconConfig {
  return NOTIFICATION_ICONS[workflowIdentifier ?? ''] ?? DEFAULT_NOTIFICATION_ICON
}
