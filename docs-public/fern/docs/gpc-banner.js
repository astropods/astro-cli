/**
 * GPC (Global Privacy Control) opt-out notice banner for Fern Docs.
 *
 * Displays a "Global Privacy Control Honored" notice when the user's browser
 * has GPC enabled and their region has purposes affected by GPC opt-out signals.
 * Uses Transcend's airgap API to detect both the signal and applicable regime.
 *
 * Dismissal is persisted via localStorage so it stays dismissed across sessions.
 *
 * References:
 * - MKTG-9235
 * - www-next GPCBanner.jsx (React version)
 * - https://globalprivacycontrol.org/
 */
(function () {
  'use strict';

  var STORAGE_KEY = 'gpc-banner-dismissed';
  var BANNER_ID = 'gpc-banner';

  // Already dismissed
  try {
    if (localStorage.getItem(STORAGE_KEY)) return;
  } catch (e) {
    // localStorage unavailable
  }

  function waitFor(checkFn, timeout, interval) {
    timeout = timeout || 10000;
    interval = interval || 200;
    return new Promise(function (resolve, reject) {
      var start = Date.now();
      function check() {
        var result = checkFn();
        if (result) return resolve(result);
        if (Date.now() - start > timeout) return reject(new Error('waitFor timed out'));
        setTimeout(check, interval);
      }
      check();
    });
  }

  function createBanner() {
    var banner = document.createElement('div');
    banner.id = BANNER_ID;
    banner.setAttribute('role', 'status');
    var headerHeight = getComputedStyle(document.documentElement).getPropertyValue('--header-height').trim() || '54px';
    banner.style.cssText =
      'position:fixed;top:' + headerHeight + ';left:0;z-index:9998;display:flex;width:100%;' +
      'align-items:center;justify-content:center;background:#1c1c1c;' +
      'padding:12px 48px;font-size:16px;color:#fff;font-family:Inter,sans-serif;';

    banner.textContent = 'Global Privacy Control Honored';

    var closeBtn = document.createElement('button');
    closeBtn.setAttribute('aria-label', 'Dismiss GPC notice');
    closeBtn.textContent = '\u00D7';
    closeBtn.style.cssText =
      'position:absolute;top:50%;right:12px;transform:translateY(-50%);' +
      'cursor:pointer;border:none;background:transparent;font-size:24px;' +
      'color:#fff;padding:0;line-height:1;';
    closeBtn.onmouseover = function () { closeBtn.style.opacity = '0.6'; };
    closeBtn.onmouseout = function () { closeBtn.style.opacity = '1'; };

    closeBtn.onclick = function () {
      banner.remove();
      try {
        localStorage.setItem(STORAGE_KEY, '1');
      } catch (e) {
        // localStorage unavailable
      }
    };

    banner.appendChild(closeBtn);

    document.body.appendChild(banner);
  }

  function checkGPC() {
    waitFor(function () {
      return window.airgap && typeof window.airgap.getPrivacySignals === 'function';
    })
      .then(function () {
        var hasGPC = window.airgap.getPrivacySignals().has('GPC');
        if (!hasGPC) return;

        // Check if any purpose in the user's regime is affected by GPC
        var purposes = Array.from(window.airgap.getRegimePurposes());
        var types = window.airgap.getPurposeTypes();
        var affected = purposes.some(function (p) {
          var signals = types[p] && types[p].optOutSignals;
          if (!signals) return false;
          // optOutSignals may be a Set or Array
          return typeof signals.has === 'function' ? signals.has('GPC') : signals.includes('GPC');
        });

        if (affected) {
          createBanner();
        }
      })
      .catch(function () {
        // airgap didn't load in time -- don't show banner
      });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', checkGPC);
  } else {
    checkGPC();
  }
})();
