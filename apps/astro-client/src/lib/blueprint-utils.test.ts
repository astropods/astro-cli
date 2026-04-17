import { describe, it, expect } from 'vitest';
import { getEffectiveCard, getBlueprintReadme, getBlueprintDescription } from './blueprint-utils';
import type { Blueprint, BlueprintCardData } from '@/lib/api';

// ─── getEffectiveCard ─────────────────────────────────────────────────────────

function makeBlueprint(overrides: Partial<Blueprint> = {}): Blueprint {
  return {
    id: 'bp-1',
    name: 'my-agent',
    account: 'acme',
    versions: [],
    draft_card: undefined,
    ...overrides,
  } as unknown as Blueprint;
}

describe('getEffectiveCard', () => {
  it('returns undefined when blueprint has no versions and no draft_card', () => {
    const bp = makeBlueprint();
    expect(getEffectiveCard(bp)).toBeUndefined();
  });

  it('prefers the agent_card from the latest version over draft_card', () => {
    const versionCard: BlueprintCardData = { description: 'from version' };
    const draftCard: BlueprintCardData = { description: 'from draft' };
    const bp = makeBlueprint({
      versions: [{ agent_card: versionCard } as any],
      draft_card: draftCard,
    });
    expect(getEffectiveCard(bp)?.description).toBe('from version');
  });

  it('falls back to draft_card when latest version has no agent_card', () => {
    const draftCard: BlueprintCardData = { description: 'from draft' };
    const bp = makeBlueprint({
      versions: [{ agent_card: undefined } as any],
      draft_card: draftCard,
    });
    expect(getEffectiveCard(bp)?.description).toBe('from draft');
  });

  it('falls back to draft_card when there are no versions', () => {
    const draftCard: BlueprintCardData = { description: 'draft only' };
    const bp = makeBlueprint({ versions: [], draft_card: draftCard });
    expect(getEffectiveCard(bp)?.description).toBe('draft only');
  });
});

// ─── getBlueprintReadme ───────────────────────────────────────────────────────

describe('getBlueprintReadme', () => {
  it('returns undefined when there is no card and no version readme', () => {
    const bp = makeBlueprint();
    expect(getBlueprintReadme(bp)).toBeUndefined();
  });

  it('returns the body from the effective card', () => {
    const bp = makeBlueprint({ draft_card: { body: '# Hello' } });
    expect(getBlueprintReadme(bp)).toBe('# Hello');
  });

  it('falls back to the version readme when card has no body', () => {
    const bp = makeBlueprint({
      versions: [{ agent_card: { description: 'no body' }, readme: 'version readme' } as any],
    });
    // card.body is undefined so falls through to version readme
    expect(getBlueprintReadme(bp)).toBe('version readme');
  });
});

// ─── getBlueprintDescription ──────────────────────────────────────────────────

describe('getBlueprintDescription', () => {
  it('falls back to the blueprint name when there is no card description', () => {
    const bp = makeBlueprint({ name: 'my-agent' });
    expect(getBlueprintDescription(bp)).toBe('my-agent');
  });

  it('returns the card description when present', () => {
    const bp = makeBlueprint({ draft_card: { description: 'A helpful agent' } });
    expect(getBlueprintDescription(bp)).toBe('A helpful agent');
  });
});
