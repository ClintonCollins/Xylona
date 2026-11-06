import { describe, expect, it, vi } from 'vitest'

import { readDroppedFiles } from './dropped-files'

function fileEntry(fullPath: string): FileSystemFileEntry {
  return {
    isFile: true,
    isDirectory: false,
    fullPath,
    file: (resolve: FileCallback) =>
      resolve(
        new File(['world data'], fullPath.split('/').pop() ?? fullPath, { lastModified: 1234 }),
      ),
  } as FileSystemFileEntry
}

function directoryEntry(batches: FileSystemEntry[][]): FileSystemDirectoryEntry {
  let index = 0
  const reader = {
    readEntries: vi.fn((resolve: FileSystemEntriesCallback) => resolve(batches[index++] ?? [])),
  }
  return {
    isFile: false,
    isDirectory: true,
    createReader: vi.fn(() => reader),
  } as unknown as FileSystemDirectoryEntry
}

describe('readDroppedFiles', () => {
  it('drains every batch from the same directory reader, including a short final batch', async () => {
    const entries = Array.from({ length: 205 }, (_, index) => fileEntry(`/world/${index}.dat`))
    const directory = directoryEntry([
      entries.slice(0, 100),
      entries.slice(100, 200),
      entries.slice(200),
    ])

    const files = await readDroppedFiles([directory])

    expect(files.map((file) => file.name)).toEqual(entries.map((entry) => entry.fullPath.slice(1)))
    expect(directory.createReader).toHaveBeenCalledTimes(1)
    const reader = vi.mocked(directory.createReader).mock.results[0]?.value
    expect(reader?.readEntries).toHaveBeenCalledTimes(4)
  })

  it('drains nested folders and preserves full relative paths, contents, and metadata', async () => {
    const nested = directoryEntry([
      [fileEntry('/world/region/0.mca')],
      [fileEntry('/world/region/1.mca')],
    ])
    const directory = directoryEntry([
      [fileEntry('/world/level.dat'), nested],
      [fileEntry('/world/icon.png')],
    ])

    const files = await readDroppedFiles([fileEntry('/server.properties'), directory])

    expect(files.map((file) => file.name)).toEqual([
      'server.properties',
      'world/level.dat',
      'world/region/0.mca',
      'world/region/1.mca',
      'world/icon.png',
    ])
    expect(await files[2]?.text()).toBe('world data')
    expect(files[2]?.lastModified).toBe(1234)
  })

  it('finishes empty folders without inventing uploadable files', async () => {
    expect(await readDroppedFiles([directoryEntry([])])).toEqual([])
  })

  it('rejects the entire drop when a later directory batch cannot be read', async () => {
    const error = new DOMException('Permission denied', 'NotReadableError')
    const readEntries = vi
      .fn()
      .mockImplementationOnce((resolve: FileSystemEntriesCallback) =>
        resolve([fileEntry('/world/level.dat')]),
      )
      .mockImplementationOnce((_resolve: FileSystemEntriesCallback, reject: ErrorCallback) =>
        reject(error),
      )
    const directory = {
      isDirectory: true,
      createReader: () => ({ readEntries }),
    } as unknown as FileSystemDirectoryEntry

    await expect(readDroppedFiles([directory])).rejects.toBe(error)
  })

  it('rejects an unreadable nested file instead of silently omitting it', async () => {
    const error = new DOMException('File was removed', 'NotFoundError')
    const unreadable = fileEntry('/world/region/missing.mca')
    unreadable.file = (_resolve, reject) => reject?.(error)

    await expect(
      readDroppedFiles([directoryEntry([[fileEntry('/world/level.dat'), unreadable]])]),
    ).rejects.toBe(error)
  })
})
