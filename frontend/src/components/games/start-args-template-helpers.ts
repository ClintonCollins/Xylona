import type { StartArgBlock } from '@/components/game_servers/start-args'

export function applyPatchToTemplateByIndex(
  template: StartArgBlock[],
  targetIndex: number,
  patch: Partial<StartArgBlock>,
) {
  return template.map((block, currentIndex) =>
    currentIndex === targetIndex
      ? normalizeBlock({ ...block, ...patch }, currentIndex)
      : normalizeBlock(block, currentIndex),
  )
}

export function applyPatchToTemplateByID(
  template: StartArgBlock[],
  blockID: string,
  patch: Partial<StartArgBlock>,
) {
  let found = false
  const nextTemplate = template.map((block, currentIndex) => {
    if (block.id !== blockID) return normalizeBlock(block, currentIndex)
    found = true
    return normalizeBlock({ ...block, ...patch }, currentIndex)
  })
  return found ? nextTemplate : null
}

export function normalizeTemplate(template: StartArgBlock[]) {
  return template.map((block, index) => normalizeBlock(block, index))
}

export function normalizeBlock(block: StartArgBlock, order: number): StartArgBlock {
  return {
    ...block,
    order,
    label: block.label ?? '',
    managedSource: block.managedSource ?? '',
    tokens: [...(block.tokens ?? [])],
  }
}

export function cloneStartArgTemplate(template: StartArgBlock[]) {
  return template.map((block) => cloneStartArgBlock(block))
}

export function cloneStartArgBlock(block: StartArgBlock): StartArgBlock {
  return { ...block, tokens: [...block.tokens] }
}

export function templateSignature(template: StartArgBlock[]) {
  return JSON.stringify(
    template.map((block) => ({
      id: block.id,
      label: block.label ?? '',
      managedSource: block.managedSource ?? '',
      order: block.order,
      ownership: block.ownership,
      tokens: [...block.tokens],
    })),
  )
}

export function templatesShareSameIDs(current: StartArgBlock[], baseline: StartArgBlock[]) {
  if (current.length !== baseline.length) return false

  const currentIDs = [...current.map((block) => block.id)].sort()
  const baselineIDs = [...baseline.map((block) => block.id)].sort()
  return currentIDs.every((id, index) => id === baselineIDs[index])
}

export function templateIDSequence(template: StartArgBlock[]) {
  return template.map((block) => block.id).join('|')
}
