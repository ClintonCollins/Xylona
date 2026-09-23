export function requiredRule(label: string) {
  return (value: string) => value.trim() !== '' || `${label} is required`
}

/** The confirm field must repeat the password, including when both are empty. */
export function matchesPasswordRule(password: () => string) {
  return (value: string) => value === password() || 'Passwords do not match'
}
