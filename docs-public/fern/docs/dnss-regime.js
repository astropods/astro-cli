/**
 * DNSS (Do Not Sell/Share) regime visibility for Fern Docs.
 *
 * Hides the DNSS footer link by default, then shows it only
 * if Transcend airgap detects a US regime. Uses vanilla JS
 * to avoid React hook compatibility issues with Fern's MDX components.
 */
(function () {
  'use strict';

  function checkRegime() {
    var maxAttempts = 40; // 40 * 250ms = 10s
    var attempts = 0;

    // Hide DNSS by default until regime is confirmed (supports both Postman and Astro footer classes)
    var style = document.createElement('style');
    style.textContent = '.pm-footer-bottom-item--dnsmi, .astro-footer-dnsmi { display: none !important; }';
    document.head.appendChild(style);

    function check() {
      attempts++;
      var airgap = window.airgap;
      if (airgap && typeof airgap.getRegimes === 'function') {
        var regimes = airgap.getRegimes();
        var isUS = Array.from(regimes).some(function (r) {
          return String(r).toLowerCase().includes('us');
        });
        if (isUS) {
          // Show DNSS for US regime
          style.textContent = '';
        }
        return;
      }
      if (attempts < maxAttempts) {
        setTimeout(check, 250);
      }
    }

    check();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', checkRegime);
  } else {
    checkRegime();
  }
})();
