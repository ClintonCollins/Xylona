import { existsSync, readFileSync } from 'node:fs'

export function parseEnvFileContents(contents) {
  const values = {}

  for (const rawLine of contents.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (line === '' || line.startsWith('#')) {
      continue
    }

    const separatorIndex = line.indexOf('=')
    if (separatorIndex === -1) {
      continue
    }

    const key = line.slice(0, separatorIndex).trim()
    let value = line.slice(separatorIndex + 1).trim()
    if (
      value.length >= 2 &&
      ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'")))
    ) {
      value = value.slice(1, -1)
    }
    values[key] = value
  }

  return values
}

export function loadEnvFile(envFilePath) {
  if (!envFilePath || !existsSync(envFilePath)) {
    return {}
  }

  return parseEnvFileContents(readFileSync(envFilePath, 'utf8'))
}

export function normalizeProxyHost(host) {
  const normalizedHost = host?.trim()
  if (
    normalizedHost === undefined ||
    normalizedHost === '' ||
    normalizedHost === '0.0.0.0' ||
    normalizedHost === '::' ||
    normalizedHost === '[::]'
  ) {
    return 'localhost'
  }

  return normalizedHost
}

export function resolveBackendProxyTarget({
  processEnv = globalThis.process?.env ?? {},
  envFilePath,
} = {}) {
  const fileEnv = loadEnvFile(envFilePath)
  const backendURL = processEnv.BACKEND_URL || fileEnv.BACKEND_URL
  if (backendURL) {
    return backendURL
  }

  const host = normalizeProxyHost(
    processEnv.HOST ||
      fileEnv.HOST ||
      processEnv.HTTP_HOST ||
      fileEnv.HTTP_HOST ||
      'localhost',
  )
  const port = processEnv.HTTP_PORT || fileEnv.HTTP_PORT || '8080'

  return `http://${host}:${port}`
}
