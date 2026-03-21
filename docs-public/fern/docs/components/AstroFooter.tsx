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
    if (typeof window !== 'undefined' && (window as any).transcend) {
      (window as any).transcend.showConsentManager();
    }
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
