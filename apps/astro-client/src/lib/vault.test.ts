import { describe, it, expect } from 'vitest'
import { VARIABLE_NAME_PATTERN } from './vault'

describe('VARIABLE_NAME_PATTERN', () => {
  const valid = [
    'FOO',
    'API_KEY',
    'a',
    'myVar',
    'database_url',
    'NodeEnv',
    'myApp_secretKey',
    '_INTERNAL',
    '_private_token',
    '_',
    'X',
    'S3_BUCKET',
    'EC2_INSTANCE_ID',
    'a1',
    'A1_B2_C3',
  ]

  const invalid = [
    '',
    '1STARTS_WITH_NUMBER',
    '123',
    'HAS SPACE',
    'HAS-DASH',
    'has.dot',
    'has@symbol',
    'has/slash',
    'has=equals',
  ]

  for (const name of valid) {
    it(`accepts "${name}"`, () => {
      expect(VARIABLE_NAME_PATTERN.test(name)).toBe(true)
    })
  }

  for (const name of invalid) {
    it(`rejects "${name || '(empty)'}"`, () => {
      expect(VARIABLE_NAME_PATTERN.test(name)).toBe(false)
    })
  }
})
