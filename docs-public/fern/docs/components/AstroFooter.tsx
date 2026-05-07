"use client";

import React from 'react';

export default function AstroFooter() {
  const currentYear = new Date().getFullYear();

  const handleDoNotSell = () => {
    if (typeof window !== 'undefined' && (window as any).transcend) {
      (window as any).transcend.showConsentManager({ viewState: 'DoNotSellExplainer' });
    }
  };

  const handleCookiePreferences = () => {
    if (typeof window === 'undefined' || !(window as any).transcend) return;
    // Transcend admin "UI View State" is set to Hidden, which no-ops a no-args
    // showConsentManager() call. Pass an explicit viewState to bypass Hidden.
    // US-safe default (CCPA/CPRA-compliant) if airgap hasn't resolved at click time.
    let viewState = 'AcceptOrRejectAllOrMoreChoices';
    try {
      const airgap = (window as any).airgap;
      const regimes = airgap && typeof airgap.getRegimes === 'function'
        ? airgap.getRegimes()
        : undefined;
      if (regimes !== undefined) {
        const regimeList = Array.isArray(regimes) ? regimes : [...regimes];
        const isUS = regimeList.some((r: unknown) => {
          const u = String(r).toUpperCase();
          return u === 'US' || u.indexOf('US-') === 0;
        });
        viewState = isUS ? 'AcceptOrRejectAllOrMoreChoices' : 'CompleteOptionsToggles';
      }
    } catch (_err) { /* fall back to US-safe default */ }
    (window as any).transcend.showConsentManager({ viewState });
  };

  return (
    <footer>
      <div className="astro-footer">
        <a href="https://www.postman.com/legal/astro-ai-terms-of-service/" target="_blank" rel="noopener noreferrer">
          Terms of Service
        </a>
        <a href="https://privacy.postman.com/policies/" target="_blank" rel="noopener noreferrer">
          Privacy Policy
        </a>
        <button
          className="astro-footer-dnsmi"
          onClick={handleDoNotSell}
          aria-label="Manage your privacy settings"
        >
          Do Not Sell or Share My Personal Information
        </button>
        <button
          onClick={handleCookiePreferences}
          aria-label="Manage your cookie and privacy preferences"
        >
          Cookie Preferences
        </button>
        <p className="astro-footer-copyright">
          &copy; {currentYear} Postman. All rights reserved.
        </p>
      </div>
    </footer>
  );
}
