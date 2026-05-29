## Summary

Agent cards on the dashboard now behave as direct entry points to their deployment detail page. This reduces the precision required to open an agent without removing the existing manage, launch, blueprint, update, or menu actions.

## Design

The deployed-agent card shell resolves the same deployment detail route used by the existing manage action. The shell handles mouse and keyboard activation only when the event originates from non-interactive card space; nested links, buttons, menu triggers, and other focusable controls are detected before navigation so their own behavior wins.

Focused component tests cover shell click navigation, keyboard activation, and precedence for an inner blueprint link.

## Migration

No user or operator action is required.
