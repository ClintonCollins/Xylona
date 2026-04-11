import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import {
  describeMinecraftMemoryState,
  validateMaxMemory,
  validatePlayerCount,
  validatePlayerCountAtMost,
  validatePort,
  validateRequiredSelection,
  validateRequiredText,
  validateRequiredValue,
} from './game-server-form-validation'

type ValidationParityFixture = {
  port: Array<ValidationParityPortCase>
  playerCount: Array<ValidationParityCountCase>
  playerCountAtMost: Array<ValidationParityAtMostCase>
  maxMemory: Array<ValidationParityMemoryCase>
}

type ValidationParityPortCase = {
  name: string
  value: number
  expected: string
}

type ValidationParityCountCase = {
  name: string
  label: string
  minimum: number
  value: number
  expected: string
}

type ValidationParityAtMostCase = {
  name: string
  label: string
  maximumLabel: string
  value: number
  maximum: number | null
  expected: string
}

type ValidationParityMemoryCase = {
  name: string
  value: number
  expected: string
}

const validationParityFixture = JSON.parse(
  readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../testdata/game-server-validation-parity.json',
    ),
    'utf8',
  ),
) as ValidationParityFixture

function assertValidationParityResult(result: true | string, expected: string) {
  const actual = result === true ? 'ok' : result
  expect(actual).toBe(expected)
}

describe('validateRequiredText', () => {
  it('requires a non-empty value', () => {
    expect(validateRequiredText('', 'Server Name')).toBe('Server Name is required')
  })

  it('rejects values longer than the max length', () => {
    expect(validateRequiredText('x'.repeat(81), 'Server Name')).toBe(
      'Server Name must be 80 characters or fewer',
    )
  })

  it('accepts trimmed values', () => {
    expect(validateRequiredText('  Survival SMP  ', 'Server Name')).toBe(true)
  })
})

describe('validateRequiredSelection', () => {
  it('requires a selected string value', () => {
    expect(validateRequiredSelection('', 'Game')).toBe('Game is required')
  })

  it('accepts selected values', () => {
    expect(validateRequiredSelection('minecraft', 'Game')).toBe(true)
  })
})

describe('validateRequiredValue', () => {
  it('requires object-like values', () => {
    expect(validateRequiredValue(undefined, 'IP Address')).toBe('IP Address is required')
  })

  it('accepts present object-like values', () => {
    expect(validateRequiredValue({ address: '127.0.0.1' }, 'IP Address')).toBe(true)
  })
})

describe('validatePort', () => {
  it('requires a port', () => {
    expect(validatePort(undefined)).toBe('Port is required')
  })

  it('rejects non-integer values', () => {
    expect(validatePort(25565.5)).toBe('Port must be a whole number')
  })

  it('rejects ports outside the valid range', () => {
    expect(validatePort(70000)).toBe('Port must be between 1 and 65535')
  })

  it('accepts valid ports', () => {
    expect(validatePort(25565)).toBe(true)
  })
})

describe('validatePlayerCount', () => {
  it('requires a value', () => {
    expect(validatePlayerCount(undefined, 'Max Players', { minimum: 1 })).toBe(
      'Max Players is required',
    )
  })

  it('rejects values lower than the minimum', () => {
    expect(validatePlayerCount(0, 'Max Players', { minimum: 1 })).toBe(
      'Max Players must be 1 or greater',
    )
  })

  it('rejects values above the maximum', () => {
    expect(validatePlayerCount(101, 'Max Players', { maximum: 100 })).toBe(
      'Max Players cannot exceed 100',
    )
  })

  it('accepts integers in range', () => {
    expect(validatePlayerCount(32, 'Max Players', { minimum: 1, maximum: 100 })).toBe(true)
  })
})

describe('validatePlayerCountAtMost', () => {
  it('allows validation to defer until both values are present', () => {
    expect(validatePlayerCountAtMost(undefined, 'Set Players', 10, 'Max Players')).toBe(true)
    expect(validatePlayerCountAtMost(4, 'Set Players', undefined, 'Max Players')).toBe(true)
  })

  it('rejects values above the related maximum', () => {
    expect(validatePlayerCountAtMost(40, 'Set Players', 20, 'Max Players')).toBe(
      'Set Players cannot exceed Max Players',
    )
  })

  it('accepts values within the related maximum', () => {
    expect(validatePlayerCountAtMost(20, 'Set Players', 20, 'Max Players')).toBe(true)
  })
})

describe('validateMaxMemory', () => {
  it('requires a value', () => {
    expect(validateMaxMemory(undefined)).toBe('Max Memory MB is required')
  })

  it('rejects values below the minimum', () => {
    expect(validateMaxMemory(64)).toBe('Max Memory MB must be at least 128')
  })

  it('accepts valid memory limits', () => {
    expect(validateMaxMemory(2048)).toBe(true)
  })
})

describe('describeMinecraftMemoryState', () => {
  it('explains when the memory limit is missing', () => {
    expect(describeMinecraftMemoryState(undefined)).toBe(
      'Set a RAM limit for this Minecraft server before saving changes.',
    )
  })

  it('explains when the memory limit is not a whole number', () => {
    expect(describeMinecraftMemoryState(512.5)).toBe(
      'Use a whole number for the Minecraft RAM limit.',
    )
  })

  it('explains when the memory limit is too low', () => {
    expect(describeMinecraftMemoryState(64)).toBe(
      'Minecraft servers need at least 128 MB before you can save changes.',
    )
  })

  it('returns nothing when the memory limit is valid', () => {
    expect(describeMinecraftMemoryState(2048)).toBeUndefined()
  })
})

describe('validation parity fixtures', () => {
  it.each(validationParityFixture.port)('matches the port contract: %s', (tt) => {
    assertValidationParityResult(validatePort(tt.value), tt.expected)
  })

  it.each(validationParityFixture.playerCount)('matches the player count contract: %s', (tt) => {
    assertValidationParityResult(
      validatePlayerCount(tt.value, tt.label, { minimum: tt.minimum }),
      tt.expected,
    )
  })

  it.each(validationParityFixture.playerCountAtMost)(
    'matches the player-count relationship contract: %s',
    (tt) => {
      assertValidationParityResult(
        validatePlayerCountAtMost(tt.value, tt.label, tt.maximum ?? undefined, tt.maximumLabel),
        tt.expected,
      )
    },
  )

  it.each(validationParityFixture.maxMemory)('matches the max memory contract: %s', (tt) => {
    assertValidationParityResult(validateMaxMemory(tt.value), tt.expected)
  })
})
