## Summary

Fixes #821. Corrects the `baseUrl` variable in the generated Postman collections from `localhost:8080` to `localhost:3100`, matching the actual port the web adapter listens on during local development.

## Design

The Postman collections bundled with the TypeScript scaffold template and the `release-note-helper` agent both declared `http://localhost:8080` as the default `baseUrl` collection variable. The web adapter dev server runs on port `3100` (as noted in the README templates), so opening the collection out of the box resulted in failed requests against the wrong port.

Updated `apps/astro-cli/internal/scaffold/templates/template-ts/postman/collections/messaging.postman_collection.json`.

## Migration

No action required. Users who already imported the old collection can update the `baseUrl` variable to `http://localhost:3100` in Postman, or re-import the collection after pulling the latest scaffold.
