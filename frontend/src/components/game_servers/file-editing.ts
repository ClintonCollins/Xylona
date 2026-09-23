/** Files above this size download instead of loading into the editor. */
export const MAX_EDITABLE_FILE_BYTES = 5 * 1024 * 1024

const editableExtensions = new Set([
  '.bat',
  '.cfg',
  '.conf',
  '.config',
  '.csv',
  '.env',
  '.ini',
  '.js',
  '.json',
  '.json5',
  '.log',
  '.lua',
  '.md',
  '.properties',
  '.ps1',
  '.py',
  '.sh',
  '.toml',
  '.ts',
  '.txt',
  '.xml',
  '.yaml',
  '.yml',
])

/**
 * Reports whether the editor should open a file by name. Names without an
 * extension (README, LICENSE) are treated as text; the caller still checks
 * size and falls back to a download for binary content.
 */
export function isEditableFileName(name: string): boolean {
  const extensionStart = name.lastIndexOf('.')
  if (extensionStart === -1) {
    return true
  }
  return editableExtensions.has(name.slice(extensionStart).toLocaleLowerCase())
}
