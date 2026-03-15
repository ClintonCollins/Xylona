export type NodePairingPayload = {
  base_url: string
  secret_key: string
}

export function normalizeNodePairingBaseURL(baseURL: string): string {
  const normalizedBaseURL = baseURL.trim().replace(/\/+$/, '')
  if (normalizedBaseURL === '') {
    throw new Error('Base URL is required')
  }

  let parsedURL: URL
  try {
    parsedURL = new URL(normalizedBaseURL)
  } catch (_errParseURL) {
    throw new Error('Base URL must be a valid URL')
  }

  if (parsedURL.protocol !== 'http:' && parsedURL.protocol !== 'https:') {
    throw new Error('Base URL must use http or https')
  }

  return normalizedBaseURL
}

export function createNodePairingPayload(baseURL: string, secretKey: string): string {
  const normalizedBaseURL = normalizeNodePairingBaseURL(baseURL)
  const normalizedSecretKey = secretKey.trim()
  if (normalizedSecretKey === '') {
    throw new Error('Secret key is required')
  }

  const payload: NodePairingPayload = {
    base_url: normalizedBaseURL,
    secret_key: normalizedSecretKey,
  }
  return JSON.stringify(payload, null, 2)
}

export function parseNodePairingPayload(payloadText: string): NodePairingPayload {
  let parsedPayload: unknown
  try {
    parsedPayload = JSON.parse(payloadText)
  } catch (_errParseJSON) {
    throw new Error('Pairing JSON is invalid')
  }

  if (parsedPayload === null || typeof parsedPayload !== 'object' || Array.isArray(parsedPayload)) {
    throw new Error('Pairing JSON must be an object')
  }

  const payloadObject = parsedPayload as Record<string, unknown>
  const baseURL = typeof payloadObject.base_url === 'string' ? payloadObject.base_url : ''
  const secretKey = typeof payloadObject.secret_key === 'string' ? payloadObject.secret_key : ''
  const normalizedSecretKey = secretKey.trim()
  if (normalizedSecretKey === '') {
    throw new Error('secret_key is required')
  }

  return {
    base_url: normalizeNodePairingBaseURL(baseURL),
    secret_key: normalizedSecretKey,
  }
}
