import { describe, expect, it } from 'vitest'

import { getMonacoWorkerKinds } from './monaco-runtime'

describe('getMonacoWorkerKinds', () => {
  it('loads the JSON worker only for JSON editors', () => {
    expect(getMonacoWorkerKinds('json')).toEqual(['editor', 'json'])
  })

  it('loads language-specific workers for script and markup editors', () => {
    expect(getMonacoWorkerKinds('javascript')).toEqual(['editor', 'ts'])
    expect(getMonacoWorkerKinds('css')).toEqual(['editor', 'css'])
    expect(getMonacoWorkerKinds('html')).toEqual(['editor', 'html'])
  })

  it('falls back to the generic editor worker for unsupported languages', () => {
    expect(getMonacoWorkerKinds('plaintext')).toEqual(['editor'])
  })
})
