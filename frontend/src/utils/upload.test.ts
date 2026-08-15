import { describe, expect, it, vi } from 'vitest'

import { UploadError, uploadFormData } from './upload'

class FakeXHR {
  static last: FakeXHR | undefined
  status = 201
  responseText = ''
  withCredentials = false
  upload = {
    onprogress: undefined as ((event: { loaded: number; total: number }) => void) | undefined,
  }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  onabort: (() => void) | null = null

  constructor() {
    FakeXHR.last = this
  }

  open = vi.fn()
  send = vi.fn()
  abort = vi.fn(() => this.onabort?.())
}

describe('uploadFormData', () => {
  it('resolves on a successful status and reports progress', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const formData = new FormData()
    const onProgress = vi.fn()
    const upload = uploadFormData('/api/backups/upload', formData, { onProgress })
    const request = FakeXHR.last
    expect(request?.open).toHaveBeenCalledWith('POST', '/api/backups/upload')
    request?.upload.onprogress?.({ loaded: 50, total: 100 })
    request?.onload?.()
    await expect(upload).resolves.toBeUndefined()
    expect(onProgress).toHaveBeenCalledWith({ loaded: 50, total: 100 })
  })

  it('rejects with the response body on a failed status', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const upload = uploadFormData('/api/backups/upload', new FormData())
    const request = FakeXHR.last
    if (request) {
      request.status = 400
      request.responseText = 'bad archive'
    }
    request?.onload?.()
    await expect(upload).rejects.toEqual(new UploadError(400, 'bad archive'))
  })
})
