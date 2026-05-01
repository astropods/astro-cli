## Summary

Forked repositories were missing from the repo picker dropdown when connecting a GitHub repo to an agent. The GitHub Search API excludes forks from results by default to avoid duplicate content, so any repo the user had forked was silently omitted.

## Design

Adding `fork:true` to the Search API query opts back into fork results. Since forks a user created are owned by them (`user-login/repo-name`), they are already scoped correctly by the existing `user:{login}` qualifier — no additional filtering is needed.

## Migration

No action required.
