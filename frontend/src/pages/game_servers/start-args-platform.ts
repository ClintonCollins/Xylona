export type StartArgsPlatform = 'linux' | 'windows'

export function resolveStartArgsPlatform(
  nodeOs: string | null | undefined,
  hasLinuxTemplate: boolean,
  hasWindowsTemplate: boolean,
): StartArgsPlatform | null {
  const normalizedNodeOs = nodeOs?.trim().toLowerCase() ?? ''
  if (normalizedNodeOs.includes('windows')) {
    return 'windows'
  }
  if (normalizedNodeOs !== '') {
    return 'linux'
  }

  if (hasLinuxTemplate && !hasWindowsTemplate) {
    return 'linux'
  }
  if (hasWindowsTemplate && !hasLinuxTemplate) {
    return 'windows'
  }

  return null
}
