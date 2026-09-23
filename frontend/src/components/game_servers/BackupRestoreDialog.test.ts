import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { BackupRestoreMode } from '@/proto/shared_pb'
import BackupRestoreDialog from './BackupRestoreDialog.vue'

const stubs = {
  'q-dialog': { props: ['modelValue'], template: '<div><slot v-if="modelValue" /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-card-actions': { template: '<div><slot /></div>' },
  'q-banner': { template: '<div class="q-banner-stub"><slot /></div>' },
  'q-icon': { template: '<i />' },
  'q-checkbox': {
    props: ['modelValue', 'label', 'disable'],
    emits: ['update:modelValue'],
    template:
      '<label>{{ label }}<input type="checkbox" :checked="modelValue" :disabled="disable" @change="$emit(\'update:modelValue\', !modelValue)" /></label>',
  },
  'q-btn': {
    props: ['label', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
  },
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(BackupRestoreDialog, {
    props: { modelValue: true, backup: null, ...props },
    global: { stubs },
  })
}

function button(wrapper: ReturnType<typeof mountDialog>, label: string) {
  const found = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  if (!found) throw new Error(`button ${label} not found`)
  return found
}

describe('BackupRestoreDialog', () => {
  it('backs up current files first unless the operator opts out', async () => {
    const wrapper = mountDialog({ currentSizeBytes: 3n * 1024n * 1024n })

    expect(wrapper.text()).toContain('Back up current files first (about 3 MB)')
    await button(wrapper, 'Restore As Overlay').trigger('click')

    await wrapper.get('input[type="checkbox"]').trigger('change')
    expect(wrapper.text()).toContain('There is no way back.')
    await button(wrapper, 'Restore Exactly').trigger('click')

    expect(wrapper.emitted('restore')).toEqual([
      [BackupRestoreMode.OVERLAY, true],
      [BackupRestoreMode.EXACT, false],
    ])
  })

  it('restores without a safety backup when the server cannot take one', async () => {
    const wrapper = mountDialog({ backupUnavailableReason: 'Backups are disabled for this server' })

    const checkbox = wrapper.get('input[type="checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)
    expect(checkbox.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Backups are disabled for this server')
    expect(wrapper.text()).toContain('There is no way back.')

    await button(wrapper, 'Restore As Overlay').trigger('click')
    expect(wrapper.emitted('restore')).toEqual([[BackupRestoreMode.OVERLAY, false]])
  })

  it('explains and blocks a restore while the server is running', () => {
    const wrapper = mountDialog({ blockedReason: 'Stop the server to restore' })

    expect(wrapper.text()).toContain('Stop the server to restore')
    expect(button(wrapper, 'Restore As Overlay').attributes('disabled')).toBeDefined()
    expect(button(wrapper, 'Restore Exactly').attributes('disabled')).toBeDefined()
  })
})
