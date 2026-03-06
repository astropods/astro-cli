import { describe, it, expect } from 'vitest';
import { slugToTitle } from './useDeployForm';

describe('slugToTitle', () => {
  it('converts hyphenated slug to title case', () => {
    expect(slugToTitle('code-reviewer')).toBe('Code Reviewer');
  });

  it('converts underscored slug to title case', () => {
    expect(slugToTitle('code_reviewer')).toBe('Code Reviewer');
  });

  it('handles mixed delimiters', () => {
    expect(slugToTitle('my-agent_v2')).toBe('My Agent V2');
  });

  it('handles single word', () => {
    expect(slugToTitle('agent')).toBe('Agent');
  });

  it('handles consecutive delimiters', () => {
    expect(slugToTitle('code--reviewer')).toBe('Code Reviewer');
  });

  it('handles leading and trailing delimiters', () => {
    expect(slugToTitle('-code-reviewer-')).toBe('Code Reviewer');
  });

  it('returns empty string for empty input', () => {
    expect(slugToTitle('')).toBe('');
  });

  it('returns empty string for delimiter-only input', () => {
    expect(slugToTitle('---')).toBe('');
  });
});
