/**
 * Strips a leading path separator (/ or \) from a full path string.
 * If fullPath is empty, returns fileName as fallback.
 */
export function stripLeadingPathSeparator(fullPath: string, fileName: string): string {
  if (fullPath === '') {
    return fileName
  }
  if (fullPath.startsWith('/') || fullPath.startsWith('\\')) {
    return fullPath.slice(1)
  }
  return fullPath
}

/**
 * Builds the upload destination path from a base path, path separator,
 * webkitRelativePath, and file name.
 */
export function buildUploadPath(
  basePath: string,
  pathSeparator: string,
  webkitRelativePath: string,
  fileName: string,
): string {
  let joinedRelativePath = basePath
  if (joinedRelativePath.length > 0) {
    joinedRelativePath += pathSeparator
  }
  let relativePath = webkitRelativePath
  if (webkitRelativePath === '') {
    const lastIndexOfPath = fileName.lastIndexOf('/')
    if (lastIndexOfPath !== -1) {
      relativePath = fileName.slice(0, lastIndexOfPath) + pathSeparator
    }
  }
  joinedRelativePath += relativePath
  return joinedRelativePath.slice(0, joinedRelativePath.lastIndexOf(pathSeparator))
}
