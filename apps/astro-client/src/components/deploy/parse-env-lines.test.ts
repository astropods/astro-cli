import { describe, it, expect } from 'vitest'
import { parseEnvLines } from './parse-env'

describe('parseEnvLines', () => {
  it('parses mixed-case keys', () => {
    const result = parseEnvLines('myApp_key=value\nNodeEnv=production')
    expect(result).toEqual([
      { name: 'myApp_key', value: 'value', valid: true },
      { name: 'NodeEnv', value: 'production', valid: true },
    ])
  })

  it('parses underscore-prefixed keys', () => {
    const result = parseEnvLines('_INTERNAL=true\n_private=tok')
    expect(result).toEqual([
      { name: '_INTERNAL', value: 'true', valid: true },
      { name: '_private', value: 'tok', valid: true },
    ])
  })

  it('handles empty values', () => {
    const result = parseEnvLines('EMPTY_VAL=')
    expect(result).toEqual([{ name: 'EMPTY_VAL', value: '', valid: true }])
  })

  it('strips inline comments from unquoted values', () => {
    const result = parseEnvLines('TTL=3600 # seconds')
    expect(result).toEqual([{ name: 'TTL', value: '3600', valid: true }])
  })

  it('preserves special characters in values', () => {
    const result = parseEnvLines('URL=https://example.com/hook?token=abc&ref=123')
    expect(result).toEqual([
      { name: 'URL', value: 'https://example.com/hook?token=abc&ref=123', valid: true },
    ])
  })

  it('handles values with equals signs', () => {
    const result = parseEnvLines('DB=postgres://user:pass@host/db?sslmode=require')
    expect(result).toEqual([
      { name: 'DB', value: 'postgres://user:pass@host/db?sslmode=require', valid: true },
    ])
  })

  it('handles double-quoted values with spaces', () => {
    const result = parseEnvLines('DESC="This is a long description"')
    expect(result).toEqual([
      { name: 'DESC', value: 'This is a long description', valid: true },
    ])
  })

  it('handles single-quoted values', () => {
    const result = parseEnvLines("MOTD='Welcome back'")
    expect(result).toEqual([{ name: 'MOTD', value: 'Welcome back', valid: true }])
  })

  it('marks lines without = as invalid', () => {
    const result = parseEnvLines('NOT_A_PAIR')
    expect(result).toEqual([{ name: 'NOT_A_PAIR', value: '', valid: false }])
  })

  it('marks lines with empty key as invalid', () => {
    const result = parseEnvLines('=value')
    expect(result).toEqual([{ name: '', value: 'value', valid: false }])
  })

  it('skips comments and blank lines', () => {
    const result = parseEnvLines('# comment\n\nFOO=bar\n  \n# another')
    expect(result).toEqual([{ name: 'FOO', value: 'bar', valid: true }])
  })

  it('handles a full .env file with mixed content', () => {
    const input = [
      '# Config',
      'OPENAI_API_KEY=sk-abc123',
      'database_url=postgresql://localhost/mydb',
      'NodeEnv=production',
      '_FLAG=true',
      'EMPTY=',
      'GREETING="Hello, World!"',
      'CACHE_TTL=3600 # seconds',
      '',
      '# End',
    ].join('\n')

    const result = parseEnvLines(input)
    expect(result).toHaveLength(7)
    expect(result.every((l) => l.valid)).toBe(true)
    expect(result[0]).toEqual({ name: 'OPENAI_API_KEY', value: 'sk-abc123', valid: true })
    expect(result[1]).toEqual({ name: 'database_url', value: 'postgresql://localhost/mydb', valid: true })
    expect(result[4]).toEqual({ name: 'EMPTY', value: '', valid: true })
    expect(result[5]).toEqual({ name: 'GREETING', value: 'Hello, World!', valid: true })
    expect(result[6]).toEqual({ name: 'CACHE_TTL', value: '3600', valid: true })
  })
})
