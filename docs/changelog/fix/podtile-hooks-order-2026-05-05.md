# Fix PodTile rules-of-hooks violation

## Summary

The `PodTile` component bailed out with `if (!workload) return null` *before*
calling `useLastErrorLog`, which violates `react-hooks/rules-of-hooks` —
the hook is called conditionally depending on whether the workload prop is
populated. CI lint failed on this rule. The defensive guard was added in
the previous PR (#930) without realising it preceded the hook.

## Design

Hooks must run on every render in the same order. Reorder so all hooks
run unconditionally with safe defaults (empty pod name, disabled query),
then bail out after. The user-visible behaviour is unchanged: when
`workload` is undefined the component still renders nothing.

## Migration

None.
