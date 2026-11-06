// Read the whole drop before adding anything to the upload queue. A failed read
// must not leave an apparently complete folder with some files silently missing.
export async function readDroppedFiles(entries: FileSystemEntry[]): Promise<File[]> {
  const files = await Promise.all(entries.map(readEntry))
  return files.flat()
}

async function readEntry(entry: FileSystemEntry): Promise<File[]> {
  if (entry.isFile) {
    const fileEntry = entry as FileSystemFileEntry
    const file = await new Promise<File>((resolve, reject) => {
      fileEntry.file(resolve, reject)
    })
    const relativePath = entry.fullPath.replace(/^[\\/]/, '') || file.name
    return [new File([file], relativePath, { lastModified: file.lastModified, type: file.type })]
  }

  if (!entry.isDirectory) {
    throw new Error('Unsupported dropped item')
  }

  // Chromium returns at most 100 entries per call. Keep the same reader and
  // drain it until it returns an empty batch, including for nested folders.
  const reader = (entry as FileSystemDirectoryEntry).createReader()
  const files: File[] = []
  while (true) {
    const entries = await new Promise<FileSystemEntry[]>((resolve, reject) => {
      reader.readEntries(resolve, reject)
    })
    if (entries.length === 0) {
      return files
    }
    for (const file of await readDroppedFiles(entries)) {
      files.push(file)
    }
  }
}
