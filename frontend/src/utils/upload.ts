export interface UploadProgress {
  loaded: number
  total: number
}

export class UploadError extends Error {
  readonly status: number
  readonly body: string

  constructor(status: number, body: string) {
    super(body.trim() !== '' ? body : `Upload failed with status ${status}`)
    this.name = 'UploadError'
    this.status = status
    this.body = body
  }
}

export function uploadFormData(
  url: string,
  formData: FormData,
  options: {
    withCredentials?: boolean
    signal?: AbortSignal
    onProgress?: (progress: UploadProgress) => void
  } = {},
): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest()
    request.open('POST', url)
    request.withCredentials = options.withCredentials ?? true

    request.upload.onprogress = (event) => {
      options.onProgress?.({
        loaded: event.loaded,
        total: event.total,
      })
    }

    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        resolve()
        return
      }
      reject(new UploadError(request.status, request.responseText))
    }

    request.onerror = () => {
      reject(new Error('Failed to upload file.'))
    }

    request.onabort = () => {
      reject(new DOMException('Upload aborted', 'AbortError'))
    }

    if (options.signal) {
      if (options.signal.aborted) {
        request.abort()
        return
      }
      options.signal.addEventListener('abort', () => request.abort(), { once: true })
    }

    request.send(formData)
  })
}
