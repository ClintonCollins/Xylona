export function parseEnvFileContents(contents: string): Record<string, string>

export function loadEnvFile(envFilePath?: string): Record<string, string>

export function normalizeProxyHost(host?: string): string

export function resolveBackendProxyTarget(options?: {
  processEnv?: Record<string, string | undefined>
  envFilePath?: string
}): string
