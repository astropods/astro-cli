import { Bell, Info } from 'lucide-react'
import { describe, expect, it } from 'vitest'

import { resolveNotificationIcon } from './notification-inbox-icons'

describe('resolveNotificationIcon', () => {
  it('resolves a known workflow identifier', () => {
    expect(resolveNotificationIcon('observation.info')).toEqual({
      icon: Info,
      tone: 'info',
    })
  })

  it('falls back for an unknown workflow identifier', () => {
    expect(resolveNotificationIcon('unknown.workflow')).toEqual({
      icon: Bell,
      tone: 'neutral',
    })
  })

  it('falls back when the notification has no workflow', () => {
    expect(resolveNotificationIcon(undefined)).toEqual({
      icon: Bell,
      tone: 'neutral',
    })
  })
})
