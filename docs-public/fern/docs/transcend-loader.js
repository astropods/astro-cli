/**
 * Transcend Consent Manager Loader for Fern Docs (Astro)
 * Loads airgap.js with required data attributes
 */
(function() {
  'use strict';

  var LOG_PREFIX = '[Transcend]';
  var BUNDLE_ID = '1f0cf102-c592-4eb4-834f-e07f2fe68ef1';

  function log(message, data) {
    if (data !== undefined) {
      console.log(LOG_PREFIX, message, data);
    } else {
      console.log(LOG_PREFIX, message);
    }
  }

  // Prevent double loading
  if (document.querySelector('script[src*="transcend-cdn.com"]')) {
    log('Script already loaded, skipping');
    return;
  }

  function loadTranscend() {
    var script = document.createElement('script');

    // Use production environment (cm)
    script.src = 'https://transcend-cdn.com/cm/' + BUNDLE_ID + '/airgap.js';

    // Required attributes per Postman team
    script.setAttribute('data-cfasync', 'false');
    script.setAttribute('data-local-sync', 'allow-network-observable');
    script.setAttribute('data-site', 'astropods.com');
    script.setAttribute('data-more-choices-view', 'CompleteOptionsToggles');

    script.onload = function() {
      log('Airgap loaded successfully');
    };

    script.onerror = function() {
      log('Failed to load airgap.js');
    };

    // Insert as first script for consent management priority
    var firstScript = document.getElementsByTagName('script')[0];
    if (firstScript && firstScript.parentNode) {
      firstScript.parentNode.insertBefore(script, firstScript);
    } else {
      document.head.appendChild(script);
    }

    log('Loading airgap.js...');
  }

  // Load immediately (consent manager should load ASAP)
  loadTranscend();
})();
