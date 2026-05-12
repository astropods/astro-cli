# Fix Quota Increase Permissions for Org Admins

## Summary

Org admins could not view or submit quota increase requests. Both endpoints were restricted to owners only due to an incorrect permission level.

## Design

The quota increase list (GET) and submit (POST) endpoints now require `org:manage` instead of `org:admin`. In WorkOS, `org:manage` is granted to both admins and owners; `org:admin` is owner-only. Moving both routes to an `accountManage` middleware group unblocks org admins while keeping plain members restricted.

## Migration

No action required. Org admins will now see the quota request table and the "Request increase" button in usage settings.
