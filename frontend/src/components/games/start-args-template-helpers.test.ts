import { describe, expect, it } from 'vitest'

import type { StartArgBlock } from '@/components/game_servers/start-args'

import {
  applyPatchToTemplateByID,
  applyPatchToTemplateByIndex,
  cloneStartArgTemplate,
  normalizeTemplate,
  templateIDSequence,
  templateSignature,
  templatesShareSameIDs,
} from './start-args-template-helpers'

const template: StartArgBlock[] = [
  {
    id: 'jar',
    order: 10,
    ownership: 'system',
    label: 'Jar',
    managedSource: 'server_executable',
    tokens: ['-jar', 'server.jar'],
  },
  {
    id: 'nogui',
    order: 20,
    ownership: 'editable',
    tokens: ['nogui'],
  },
]

describe('start-args-template-helpers', () => {
  it('normalizes order and optional fields without mutating token arrays', () => {
    const normalized = normalizeTemplate(template)

    expect(normalized).toEqual([
      {
        id: 'jar',
        order: 0,
        ownership: 'system',
        label: 'Jar',
        managedSource: 'server_executable',
        tokens: ['-jar', 'server.jar'],
      },
      {
        id: 'nogui',
        order: 1,
        ownership: 'editable',
        label: '',
        managedSource: '',
        tokens: ['nogui'],
      },
    ])

    normalized[0]?.tokens.push('changed')
    expect(template[0]?.tokens).toEqual(['-jar', 'server.jar'])
  })

  it('applies index and ID patches while preserving normalized ordering', () => {
    expect(applyPatchToTemplateByIndex(template, 1, { label: 'No GUI' })).toMatchObject([
      { id: 'jar', order: 0 },
      { id: 'nogui', order: 1, label: 'No GUI' },
    ])

    expect(
      applyPatchToTemplateByID(template, 'jar', { tokens: ['-jar', 'paper.jar'] }),
    ).toMatchObject([
      { id: 'jar', order: 0, tokens: ['-jar', 'paper.jar'] },
      { id: 'nogui', order: 1 },
    ])

    expect(applyPatchToTemplateByID(template, 'missing', { label: 'Missing' })).toBeNull()
  })

  it('clones and compares template identity separately from order', () => {
    const clone = cloneStartArgTemplate(template)
    clone[0]?.tokens.push('changed')
    const [jarBlock, noguiBlock] = template
    if (jarBlock === undefined || noguiBlock === undefined) {
      throw new Error('expected test template blocks')
    }

    expect(template[0]?.tokens).toEqual(['-jar', 'server.jar'])
    expect(templatesShareSameIDs(template, [noguiBlock, jarBlock])).toBe(true)
    expect(templateIDSequence(template)).toBe('jar|nogui')
    expect(templateSignature(template)).not.toEqual(templateSignature([noguiBlock, jarBlock]))
  })
})
