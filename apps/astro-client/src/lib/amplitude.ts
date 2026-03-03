import * as amplitude from '@amplitude/analytics-browser';
import { sessionReplayPlugin } from '@amplitude/plugin-session-replay-browser';

let initialized = false;

export function initAmplitude(apiKey: string) {
  if (typeof window === 'undefined' || initialized || !apiKey) return;

  amplitude.init(apiKey, {
    autocapture: {
      attribution: true,
      pageViews: true,
      sessions: true,
      formInteractions: true,
      fileDownloads: true,
      elementInteractions: true,
    },
  });

  // Session replay — must be added after init
  const sessionReplay = sessionReplayPlugin({
    sampleRate: 1,
    privacyConfig: {
      blockSelector: ['.amp-block', '[data-sensitive]'],
    },
  });
  amplitude.add(sessionReplay);

  initialized = true;
}

export function identifyUser(userId: string, email?: string, orgId?: string) {
  if (typeof window === 'undefined' || !initialized) return;
  amplitude.setUserId(userId);
  const identify = new amplitude.Identify();
  if (email) identify.set('email', email);
  if (orgId) identify.set('org_id', orgId);
  amplitude.identify(identify);
}

export function resetUser() {
  if (typeof window === 'undefined' || !initialized) return;
  amplitude.reset();
}

export function trackEvent(name: string, properties?: Record<string, unknown>) {
  if (typeof window === 'undefined' || !initialized) return;
  amplitude.track(name, properties);
}
