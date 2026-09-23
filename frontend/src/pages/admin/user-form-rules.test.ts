import { describe, expect, it } from 'vitest'

import { matchesPasswordRule, requiredRule } from './user-form-rules'

describe('user form rules', () => {
  it('requires a value that is more than whitespace', () => {
    const rule = requiredRule('Username')
    expect(rule('')).toBe('Username is required')
    expect(rule('   ')).toBe('Username is required')
    expect(rule('alice')).toBe(true)
  })

  it('requires the confirmation to repeat the password exactly', () => {
    let password = 'secret'
    const rule = matchesPasswordRule(() => password)
    expect(rule('secret')).toBe(true)
    expect(rule('Secret')).toBe('Passwords do not match')
    password = ''
    expect(rule('')).toBe(true)
  })
})
