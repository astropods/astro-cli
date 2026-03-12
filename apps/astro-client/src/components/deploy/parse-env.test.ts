import { describe, it, expect } from 'vitest';
import { parseEnvText, parseVariables } from './parse-env';

describe('parseEnvText', () => {
  it('parses basic KEY=VALUE pairs', () => {
    expect(parseEnvText('FOO=bar\nBAZ=qux')).toEqual({ FOO: 'bar', BAZ: 'qux' });
  });

  it('splits on the first = only', () => {
    expect(parseEnvText('DB_URL=postgres://user:pass@host/db')).toEqual({
      DB_URL: 'postgres://user:pass@host/db',
    });
  });

  it('skips blank lines and comments', () => {
    const input = `
# Database config
DB_URL=postgres://localhost

# API keys
API_KEY=sk-123
    `;
    expect(parseEnvText(input)).toEqual({
      DB_URL: 'postgres://localhost',
      API_KEY: 'sk-123',
    });
  });

  it('strips double-quoted values', () => {
    expect(parseEnvText('MSG="hello world"')).toEqual({ MSG: 'hello world' });
  });

  it('strips single-quoted values', () => {
    expect(parseEnvText("MSG='hello world'")).toEqual({ MSG: 'hello world' });
  });

  it('does not strip mismatched quotes', () => {
    expect(parseEnvText('MSG="hello\'')).toEqual({ MSG: '"hello\'' });
  });

  it('handles values with spaces', () => {
    expect(parseEnvText('PROMPT=You are a helpful assistant')).toEqual({
      PROMPT: 'You are a helpful assistant',
    });
  });

  it('trims whitespace around keys and values', () => {
    expect(parseEnvText('  FOO  =  bar  ')).toEqual({ FOO: 'bar' });
  });

  it('handles empty values', () => {
    expect(parseEnvText('EMPTY=')).toEqual({ EMPTY: '' });
  });

  it('skips lines without =', () => {
    expect(parseEnvText('NO_EQUALS_HERE\nFOO=bar')).toEqual({ FOO: 'bar' });
  });

  it('skips lines with empty key', () => {
    expect(parseEnvText('=value')).toEqual({});
  });

  it('handles Windows line endings', () => {
    expect(parseEnvText('FOO=bar\r\nBAZ=qux')).toEqual({ FOO: 'bar', BAZ: 'qux' });
  });

  it('strips inline comments for unquoted values', () => {
    expect(parseEnvText('FOO=bar # this is a comment')).toEqual({ FOO: 'bar' });
  });

  it('returns empty object for empty input', () => {
    expect(parseEnvText('')).toEqual({});
  });

  it('returns empty object for only comments', () => {
    expect(parseEnvText('# just a comment\n# another')).toEqual({});
  });
});

describe('parseVariables', () => {
  it('auto-detects JSON format', () => {
    const json = JSON.stringify({ API_KEY: 'sk-123', MAX_TOKENS: '4096' });
    expect(parseVariables(json)).toEqual({ API_KEY: 'sk-123', MAX_TOKENS: '4096' });
  });

  it('stringifies non-string JSON values', () => {
    const json = JSON.stringify({ COUNT: 42, ENABLED: true, TAGS: ['a', 'b'] });
    expect(parseVariables(json)).toEqual({
      COUNT: '42',
      ENABLED: 'true',
      TAGS: '["a","b"]',
    });
  });

  it('rejects JSON arrays and falls back to env parsing', () => {
    expect(parseVariables('[1, 2, 3]')).toEqual({});
  });

  it('falls back to env format when JSON parse fails', () => {
    const input = 'API_KEY=sk-123\nDB_URL=postgres://localhost';
    expect(parseVariables(input)).toEqual({
      API_KEY: 'sk-123',
      DB_URL: 'postgres://localhost',
    });
  });

  it('falls back to env for JSON-like but invalid input', () => {
    expect(parseVariables('{ broken json')).toEqual({});
  });

  it('returns empty object for empty string', () => {
    expect(parseVariables('')).toEqual({});
  });

  it('returns empty object for whitespace-only input', () => {
    expect(parseVariables('   \n  \n  ')).toEqual({});
  });

  it('handles a real-world .env file', () => {
    const input = `
# Production config
OPENAI_API_KEY=sk-abc123
QDRANT_API_KEY="qdrant-key-456"
AGENT_PERSONALITY=You are a helpful assistant
MAX_TOKENS=4096
# Optional
WEBHOOK_URL=
DATABASE_CONNECTION_STRING='postgres://user:pass@host/db'
    `;
    expect(parseVariables(input)).toEqual({
      OPENAI_API_KEY: 'sk-abc123',
      QDRANT_API_KEY: 'qdrant-key-456',
      AGENT_PERSONALITY: 'You are a helpful assistant',
      MAX_TOKENS: '4096',
      WEBHOOK_URL: '',
      DATABASE_CONNECTION_STRING: 'postgres://user:pass@host/db',
    });
  });
});
