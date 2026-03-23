const ANSI_ESCAPE_RE = new RegExp(
  `${String.fromCharCode(27)}(?:[@-Z\\\\-_]|\\[[0-?]*[ -/]*[@-~])`,
  'g',
)

export type OperationOutputRoute = 'console' | 'update' | 'software' | 'discard'

export function normalizeOperationOutputChunk(output: string): string[] {
  return output
    .replace(/\r/g, '')
    .split('\n')
    .map((line) => line.replace(ANSI_ESCAPE_RE, '').trimEnd())
    .filter((line) => line.trim().length > 0)
}

export function appendOperationOutputLines(
  existingLines: string[],
  output: string,
  maxLines = 80,
): string[] {
  const nextLines = [...existingLines, ...normalizeOperationOutputChunk(output)]
  if (nextLines.length <= maxLines) {
    return nextLines
  }
  return nextLines.slice(-maxLines)
}

export function resolveOperationOutputRoute(params: {
  isServerOffline: boolean
  updateInProgress: boolean
  softwareOperationInProgress: boolean
}): OperationOutputRoute {
  if (!params.isServerOffline) {
    return 'console'
  }

  if (params.softwareOperationInProgress) {
    return 'software'
  }

  if (params.updateInProgress) {
    return 'update'
  }

  return 'discard'
}
