<template>
  <div>
    <q-dialog
      v-model="showChangeDialog"
      persistent
      backdrop-filter="brightness(15%)"
      aria-labelledby="variant-change-title">
      <q-card class="change-dialog-card">
        <q-card-section class="change-dialog-header">
          <div id="variant-change-title" class="change-dialog-title">Change Variant</div>
          <div class="change-dialog-subtitle">
            Select the server distribution this server should use.
          </div>
        </q-card-section>

        <q-card-section class="change-dialog-body">
          <div class="dialog-current">
            <div class="dialog-current-icon">
              <q-icon name="layers" size="16px" color="accent" />
            </div>
            <div>
              <div class="dialog-current-label">Currently active</div>
              <div>
                <span class="dialog-current-value">{{ currentSoftwareDisplayName }}</span>
                <span v-if="currentVersion" class="dialog-current-version">{{
                  currentVersion
                }}</span>
              </div>
            </div>
          </div>

          <div class="dialog-field">
            <q-select
              v-model="selectedVariantId"
              outlined
              dense
              label="Variant"
              emit-value
              map-options
              :display-value="selectedVariantName || undefined"
              :options="variantSelectOptions"
              :disable="saving || installStatus === 'installing'"
              options-selected-class="selected-option" />
          </div>

          <div v-if="targetOptions.length > 0" class="dialog-field">
            <q-select
              v-model="selectedTarget"
              outlined
              dense
              label="Version"
              emit-value
              map-options
              :options="targetOptions"
              :disable="saving || loadingTargets || installStatus === 'installing'"
              options-selected-class="selected-option" />
          </div>

          <div v-if="showPinTargetToggle" class="dialog-field">
            <q-toggle
              v-model="selectedPinTarget"
              color="primary"
              label="Stick to selected version"
              :disable="saving || loadingTargets || installStatus === 'installing'" />
            <div class="dialog-toggle-hint">
              When off, Xylona installs this version now and keeps tracking the latest release.
            </div>
          </div>

          <div class="dialog-warning">
            <q-icon name="warning" size="16px" color="warning" class="dialog-warning-icon" />
            <span>
              Switching variants may change update behavior and mod compatibility. The server must
              stay offline while Xylona applies the new variant.
            </span>
          </div>
        </q-card-section>

        <q-card-actions class="change-dialog-footer" align="right">
          <q-btn
            flat
            no-caps
            label="Cancel"
            class="dialog-cancel-btn"
            @click="cancelChangeDialog" />
          <q-btn
            no-caps
            label="Apply"
            color="primary"
            :loading="saving"
            :disable="!canApply || installStatus === 'installing'"
            @click="applyVariant" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

import { UpdateProviderKind, type Variant } from '@/proto/shared_pb'
import {
  GetUpdateTargetsRequestSchema,
  GetVariantOperationStatusRequestSchema,
  SetServerVariantRequestSchema,
} from '@/proto/xylona_pb'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import type { ServerSoftwareOperationEvent } from './ServerSoftwareSelector.types'

interface Props {
  gameServerId: string
  gameName: string
  currentSoftware?: string
  currentVersion?: string
  currentTarget?: string
  currentTargetPinned?: boolean
  currentInstalledVersion?: string
  variants?: Variant[]
}

const props = withDefaults(defineProps<Props>(), {
  currentSoftware: '',
  currentVersion: '',
  currentTarget: '',
  currentTargetPinned: false,
  currentInstalledVersion: '',
  variants: () => [],
})
const emit = defineEmits<{
  'software-changed': []
  'software-operation-state': [event: ServerSoftwareOperationEvent]
}>()

const $q = useQuasar()

const selectedVariantId = ref('')
const saving = ref(false)
const showChangeDialog = ref(false)
const installStatus = ref<'idle' | 'installing' | 'complete' | 'failed'>('idle')
const selectedTarget = ref('')
const targetOptions = ref<Array<{ label: string; value: string }>>([])
const loadingTargets = ref(false)
const initialVariantId = ref('')
const initialTarget = ref('')
const selectedPinTarget = ref(false)
const initialPinTarget = ref(false)

const variantSelectOptions = computed(() =>
  props.variants.map((variant) => ({
    label: variant.name,
    value: variant.id,
  })),
)

const selectedVariantName = computed(() => {
  return resolveVariantName(selectedVariantId.value)
})

const selectedVariantProviderKind = computed(() => {
  return resolveVariantProviderKind(selectedVariantId.value)
})

const showPinTargetToggle = computed(() => {
  return (
    targetOptions.value.length > 0 &&
    (selectedVariantProviderKind.value === UpdateProviderKind.MOJANG ||
      selectedVariantProviderKind.value === UpdateProviderKind.PAPERMC)
  )
})

const currentSoftwareDisplayName = computed(() => {
  if (props.currentSoftware) {
    const name = resolveVariantName(props.currentSoftware)
    if (name !== '') {
      return name
    }
  }
  return props.gameName || 'Default'
})

const canApply = computed(() => {
  if (selectedVariantId.value === '') {
    return false
  }

  const variantChanged = selectedVariantId.value !== initialVariantId.value
  const targetChanged =
    targetOptions.value.length > 0 && selectedTarget.value.trim() !== initialTarget.value.trim()
  const pinChanged = showPinTargetToggle.value && selectedPinTarget.value !== initialPinTarget.value

  return variantChanged || targetChanged || pinChanged
})

onMounted(async () => {
  XylonaEventBus.on('serverSoftwareInstall', handleInstallEvent)
  selectedVariantId.value = props.currentSoftware
  await checkInstallStatus()
})

onUnmounted(() => {
  XylonaEventBus.off('serverSoftwareInstall', handleInstallEvent)
})

watch(
  () => props.currentSoftware,
  (currentSoftware) => {
    selectedVariantId.value = currentSoftware
  },
)

watch(selectedVariantId, async (variantId) => {
  if (!showChangeDialog.value) {
    return
  }
  await loadVariantTargets(variantId)
})

function openChangeDialog(): void {
  selectedVariantId.value = props.currentSoftware
  initialVariantId.value = props.currentSoftware
  initialTarget.value = ''
  selectedPinTarget.value =
    props.currentTargetPinned && variantSupportsExplicitPinning(props.currentSoftware)
  initialPinTarget.value = selectedPinTarget.value
  showChangeDialog.value = true
  void loadVariantTargets(selectedVariantId.value, true)
}

function cancelChangeDialog(): void {
  showChangeDialog.value = false
}

async function checkInstallStatus(): Promise<void> {
  try {
    const response = await GetXylonaClient().getVariantOperationStatus(
      create(GetVariantOperationStatusRequestSchema, {
        gameServerId: props.gameServerId,
      }),
    )
    if (response.status === 'installing') {
      installStatus.value = 'installing'
      emitOperationState('installing', response.variantId)
      return
    }
    if (response.status === 'failed') {
      installStatus.value = 'failed'
      emitOperationState('failed', response.variantId, response.error)
    }
  } catch {
    // Non-critical.
  }
}

async function applyVariant(): Promise<void> {
  saving.value = true
  try {
    const response = await GetXylonaClient().setServerVariant(
      create(SetServerVariantRequestSchema, {
        gameServerId: props.gameServerId,
        variantId: selectedVariantId.value,
        target: selectedTarget.value,
        pinTarget: selectedPinTarget.value,
      }),
    )
    showChangeDialog.value = false

    if (response.status === 'installing') {
      installStatus.value = 'installing'
      emitOperationState('installing', selectedVariantId.value)
      return
    }

    installStatus.value = 'complete'
    emitOperationState('complete', selectedVariantId.value)
    emit('software-changed')
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    showChangeDialog.value = false
    installStatus.value = 'failed'
    emitOperationState('failed', selectedVariantId.value, err.message)
    $q.notify({
      type: 'xylona-error',
      caption: err.message,
      position: 'top',
      timeout: 5000,
    })
  } finally {
    saving.value = false
  }
}

function emitOperationState(
  status: ServerSoftwareOperationEvent['status'],
  softwareId: string,
  error = '',
): void {
  const resolvedVariantId = softwareId || props.currentSoftware || selectedVariantId.value
  emit('software-operation-state', {
    status,
    softwareId: resolvedVariantId,
    softwareName: resolveVariantName(resolvedVariantId) || currentSoftwareDisplayName.value,
    versionLabel: props.currentVersion || undefined,
    error: error || undefined,
  })
}

function resolveVariantName(variantId: string): string {
  if (variantId === '') {
    return ''
  }
  const found = props.variants.find((variant) => variant.id === variantId)
  return found?.name ?? ''
}

function resolveVariantProviderKind(variantId: string): UpdateProviderKind | undefined {
  const trimmedVariantID = variantId.trim()
  if (trimmedVariantID === '') {
    return undefined
  }
  const found = props.variants.find((variant) => variant.id === trimmedVariantID)
  return found?.updateProvider?.kind
}

function variantSupportsExplicitPinning(variantId: string): boolean {
  const providerKind = resolveVariantProviderKind(variantId)
  return providerKind === UpdateProviderKind.MOJANG || providerKind === UpdateProviderKind.PAPERMC
}

function installedTargetCandidate(variantId: string): string {
  const providerKind = resolveVariantProviderKind(variantId)
  const normalizedInstalledVersion = props.currentInstalledVersion.trim()
  if (normalizedInstalledVersion === '') {
    return ''
  }

  if (providerKind === UpdateProviderKind.PAPERMC) {
    const match = normalizedInstalledVersion.match(/^(.+)-\d+$/)
    if (match?.[1]) {
      return match[1].trim()
    }
  }

  return normalizedInstalledVersion
}

function resolveInitialTargetSelection(
  variantId: string,
  availableTargets: string[],
  currentTarget: string,
): string {
  if (availableTargets.length === 0) {
    return ''
  }

  const sameVariant = variantId.trim() === initialVariantId.value.trim()
  if (!sameVariant) {
    return availableTargets[0]
  }

  if (props.currentTargetPinned) {
    if (currentTarget !== '' && availableTargets.includes(currentTarget)) {
      return currentTarget
    }
    const normalizedCurrentTarget = props.currentTarget.trim()
    if (normalizedCurrentTarget !== '' && availableTargets.includes(normalizedCurrentTarget)) {
      return normalizedCurrentTarget
    }
  }

  const installedCandidate = installedTargetCandidate(variantId)
  if (
    !props.currentTargetPinned &&
    installedCandidate !== '' &&
    availableTargets.includes(installedCandidate)
  ) {
    return installedCandidate
  }

  if (
    props.currentTargetPinned &&
    currentTarget !== '' &&
    availableTargets.includes(currentTarget)
  ) {
    return currentTarget
  }

  return availableTargets[0]
}

function handleInstallEvent(
  gameServerId: string,
  status: string,
  error: string,
  softwareId: string,
): void {
  if (gameServerId !== props.gameServerId) {
    return
  }

  if (status === 'installing') {
    installStatus.value = 'installing'
    emitOperationState('installing', softwareId)
    return
  }

  if (status === 'complete') {
    installStatus.value = 'idle'
    emitOperationState('complete', softwareId)
    emit('software-changed')
    return
  }

  if (status === 'failed') {
    installStatus.value = 'failed'
    emitOperationState('failed', softwareId, error)
  }
}

async function loadVariantTargets(variantId: string, resetBaseline = false): Promise<void> {
  const trimmedVariantID = variantId.trim()
  if (trimmedVariantID === '') {
    targetOptions.value = []
    selectedTarget.value = ''
    selectedPinTarget.value = false
    if (resetBaseline) {
      initialTarget.value = ''
      initialPinTarget.value = false
    }
    return
  }

  if (!variantSupportsExplicitPinning(trimmedVariantID)) {
    selectedPinTarget.value = false
  }

  loadingTargets.value = true
  try {
    const response = await GetXylonaClient().getUpdateTargets(
      create(GetUpdateTargetsRequestSchema, {
        gameServerId: props.gameServerId,
        variantId: trimmedVariantID,
      }),
    )

    targetOptions.value = response.targets.map((target) => ({
      label: target.label || target.id,
      value: target.id,
    }))

    if (targetOptions.value.length == 0) {
      selectedTarget.value = ''
      if (resetBaseline) {
        initialTarget.value = ''
        initialPinTarget.value = selectedPinTarget.value
      }
      return
    }

    const currentTarget = response.currentTarget.trim()
    const availableTargets = targetOptions.value.map((target) => target.value)
    selectedTarget.value = resolveInitialTargetSelection(
      trimmedVariantID,
      availableTargets,
      currentTarget,
    )
    if (resetBaseline) {
      initialTarget.value = selectedTarget.value
      initialPinTarget.value = selectedPinTarget.value
    }
  } catch {
    targetOptions.value = []
    selectedTarget.value = ''
    selectedPinTarget.value = false
    if (resetBaseline) {
      initialTarget.value = ''
      initialPinTarget.value = false
    }
  } finally {
    loadingTargets.value = false
  }
}

defineExpose({
  currentSoftwareDisplayName,
  openChangeDialog,
  selectedVariantId,
  selectedTarget,
  selectedPinTarget,
  applyVariant,
})
</script>

<style scoped>
.change-dialog-card {
  width: 420px;
  max-width: 90vw;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.change-dialog-header {
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--xy-border);
}

.change-dialog-title {
  font-family: var(--xy-font-display);
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--xy-text-primary);
}

.change-dialog-subtitle {
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  margin-top: 0.2rem;
}

.change-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 0.875rem;
}

.dialog-current {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.5rem 0.75rem;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
}

.dialog-current-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 999px;
  background: color-mix(in srgb, var(--xy-accent) 16%, transparent);
}

.dialog-current-label {
  font-size: 0.7rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.dialog-current-value {
  font-weight: 600;
  color: var(--xy-text-primary);
}

.dialog-current-version {
  margin-left: 0.4rem;
  color: var(--xy-text-secondary);
}

.dialog-field {
  display: flex;
  flex-direction: column;
}

.dialog-toggle-hint {
  margin-top: 0.35rem;
  font-size: 0.75rem;
  color: var(--xy-text-muted);
}

.dialog-warning {
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
  padding: 0.75rem;
  background: color-mix(in srgb, var(--xy-warning) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--xy-warning) 22%, transparent);
  color: var(--xy-text-secondary);
  font-size: 0.84rem;
}

.dialog-warning-icon {
  margin-top: 0.05rem;
}

.change-dialog-footer {
  border-top: 1px solid var(--xy-border);
  padding-top: 0.75rem;
}
</style>
