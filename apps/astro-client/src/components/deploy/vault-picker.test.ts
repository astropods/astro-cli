import { describe, it, expect } from 'vitest'
import { parseVaultToken } from './VaultPicker'

describe('parseVaultToken', () => {
  it('parses uppercase secret token', () => {
    expect(parseVaultToken('{{secrets.API_KEY}}')).toEqual({ type: 'secret', name: 'API_KEY' })
  })

  it('parses uppercase variable token', () => {
    expect(parseVaultToken('{{vars.APP_ENV}}')).toEqual({ type: 'variable', name: 'APP_ENV' })
  })

  it('parses lowercase key', () => {
    expect(parseVaultToken('{{secrets.database_url}}')).toEqual({ type: 'secret', name: 'database_url' })
  })

  it('parses mixed-case key', () => {
    expect(parseVaultToken('{{vars.myApp_key}}')).toEqual({ type: 'variable', name: 'myApp_key' })
  })

  it('parses underscore-prefixed key', () => {
    expect(parseVaultToken('{{secrets._INTERNAL}}')).toEqual({ type: 'secret', name: '_INTERNAL' })
  })

  it('parses single character key', () => {
    expect(parseVaultToken('{{vars.X}}')).toEqual({ type: 'variable', name: 'X' })
  })

  it('rejects token with no braces', () => {
    expect(parseVaultToken('secrets.FOO')).toBeNull()
  })

  it('rejects token with unknown prefix', () => {
    expect(parseVaultToken('{{env.FOO}}')).toBeNull()
  })

  it('rejects empty key', () => {
    expect(parseVaultToken('{{secrets.}}')).toBeNull()
  })

  it('rejects key starting with number', () => {
    expect(parseVaultToken('{{secrets.1BAD}}')).toBeNull()
  })

  it('rejects key with dash', () => {
    expect(parseVaultToken('{{vars.my-key}}')).toBeNull()
  })

  it('rejects empty string', () => {
    expect(parseVaultToken('')).toBeNull()
  })
})
