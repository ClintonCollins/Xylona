export type RuleResult = true | string

function normalizeString(value: string | null | undefined): string {
  return value?.trim() ?? ''
}

function toFiniteNumber(value: number | string | bigint | null | undefined): number | undefined {
  if (typeof value === 'bigint') {
    return Number(value)
  }

  if (value === null || value === undefined || value === '') {
    return undefined
  }

  const numericValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numericValue)) {
    return undefined
  }

  return numericValue
}

export function validateRequiredText(
  value: string | null | undefined,
  label: string,
  maxLength = 80,
): RuleResult {
  const normalized = normalizeString(value)

  if (normalized.length === 0) {
    return `${label} is required`
  }

  if (normalized.length > maxLength) {
    return `${label} must be ${maxLength} characters or fewer`
  }

  return true
}

export function validateRequiredSelection(
  value: string | null | undefined,
  label: string,
): RuleResult {
  return normalizeString(value).length > 0 ? true : `${label} is required`
}

export function validateRequiredValue<T>(value: T | null | undefined, label: string): RuleResult {
  if (value === null || value === undefined) {
    return `${label} is required`
  }

  if (typeof value === 'string' && normalizeString(value).length === 0) {
    return `${label} is required`
  }

  return true
}

export function validatePort(value: number | string | bigint | null | undefined): RuleResult {
  const numericValue = toFiniteNumber(value)

  if (numericValue === undefined) {
    return 'Port is required'
  }

  if (!Number.isInteger(numericValue)) {
    return 'Port must be a whole number'
  }

  if (numericValue < 1 || numericValue > 65535) {
    return 'Port must be between 1 and 65535'
  }

  return true
}

export function validatePlayerCount(
  value: number | string | bigint | null | undefined,
  label: string,
  options?: { minimum?: number; maximum?: number },
): RuleResult {
  const numericValue = toFiniteNumber(value)

  if (numericValue === undefined) {
    return `${label} is required`
  }

  if (!Number.isInteger(numericValue)) {
    return `${label} must be a whole number`
  }

  const minimum = options?.minimum ?? 0
  if (numericValue < minimum) {
    return `${label} must be ${minimum} or greater`
  }

  if (options?.maximum !== undefined && numericValue > options.maximum) {
    return `${label} cannot exceed ${options.maximum}`
  }

  return true
}

export function validatePlayerCountAtMost(
  value: number | string | bigint | null | undefined,
  label: string,
  maximum: number | string | bigint | null | undefined,
  maximumLabel: string,
): RuleResult {
  const numericValue = toFiniteNumber(value)
  const numericMaximum = toFiniteNumber(maximum)

  if (numericValue === undefined || numericMaximum === undefined) {
    return true
  }

  if (numericValue > numericMaximum) {
    return `${label} cannot exceed ${maximumLabel}`
  }

  return true
}

export function validateMaxMemory(value: number | string | bigint | null | undefined): RuleResult {
  const numericValue = toFiniteNumber(value)

  if (numericValue === undefined) {
    return 'Max Memory MB is required'
  }

  if (!Number.isInteger(numericValue)) {
    return 'Max Memory MB must be a whole number'
  }

  if (numericValue < 128) {
    return 'Max Memory MB must be at least 128'
  }

  return true
}

export function describeMinecraftMemoryState(
  value: number | string | bigint | null | undefined,
): string | undefined {
  const validationResult = validateMaxMemory(value)
  if (validationResult === true) {
    return undefined
  }

  switch (validationResult) {
    case 'Max Memory MB is required':
      return 'Set a RAM limit for this Minecraft server before saving changes.'
    case 'Max Memory MB must be a whole number':
      return 'Use a whole number for the Minecraft RAM limit.'
    default:
      return 'Minecraft servers need at least 128 MB before you can save changes.'
  }
}
