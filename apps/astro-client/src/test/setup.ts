import '@testing-library/jest-dom/vitest';
import { beforeAll, afterEach, afterAll } from 'vitest';
import { cleanup } from '@testing-library/react';
import { clearPageFilterStorage } from '@/lib/persistent-storage';
import { server } from './msw/server';
import { createMemoryStorage } from './memory-storage';

if (typeof globalThis.localStorage?.getItem !== 'function') {
  Object.defineProperty(globalThis, 'localStorage', {
    value: createMemoryStorage(),
    configurable: true,
  });
}

// jsdom does not implement pointer capture or scrollIntoView — required by Radix UI components (Select, DropdownMenu, etc.)
if (typeof window !== 'undefined') {
  window.HTMLElement.prototype.hasPointerCapture ??= () => false;
  window.HTMLElement.prototype.setPointerCapture ??= () => {};
  window.HTMLElement.prototype.releasePointerCapture ??= () => {};
  window.HTMLElement.prototype.scrollIntoView ??= () => {};
}

// jsdom does not implement matchMedia — required by useMediaBreakpoint
// (useIsMobile / useCompactLayout) and any component that reads viewport width.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

// jsdom does not implement ResizeObserver
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof globalThis.ResizeObserver;
}

// jsdom logs "Not implemented" for window.scrollTo — stub it to silence the noise.
if (typeof window !== 'undefined') {
  window.scrollTo = () => {};
}

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => {
  server.resetHandlers();
  cleanup();
  clearPageFilterStorage();
});
afterAll(() => server.close());
