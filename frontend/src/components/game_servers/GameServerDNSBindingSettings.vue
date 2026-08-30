<template>
  <div class="dns-binding-settings">
    <div v-if="loading" class="dns-binding-state" role="status">
      <q-spinner color="primary" size="32px" />
      <span>Loading DNS binding</span>
    </div>

    <div v-else-if="loadError" class="dns-binding-state" role="alert">
      <q-icon name="sync_problem" size="32px" />
      <strong>DNS binding could not be loaded.</strong>
      <span>{{ loadError }}</span>
      <q-btn color="primary" label="Try again" no-caps @click="loadBinding" />
    </div>

    <template v-else>
      <q-banner class="manual-only-note" rounded>
        Saving stores only the relative name. DNS changes only when you choose
        <strong>Sync now</strong>.
      </q-banner>

      <q-input
        v-model="relativeName"
        :error="relativeNameError !== ''"
        :error-message="relativeNameError"
        class="q-mt-md"
        data-testid="dns-relative-name"
        hint="One name relative to the configured authoritative zone, such as play"
        label="Relative Record Name"
        maxlength="253"
        outlined
        @update:model-value="relativeNameError = ''" />

      <div v-if="binding" class="dns-binding-summary q-mt-lg">
        <div>
          <span>Fully qualified name</span>
          <code>{{ binding.fullyQualifiedName }}</code>
        </div>
        <div>
          <span>Inferred record</span>
          <strong>{{ recordTypeLabel }}</strong>
        </div>
        <div>
          <span>Bind address</span>
          <code>{{ binding.bindAddress }}</code>
        </div>
        <div>
          <span>TTL</span>
          <strong>{{ binding.ttlSeconds || 300 }} seconds</strong>
        </div>
        <div>
          <span>Local ownership</span>
          <strong>{{ ownershipLabel }}</strong>
          <code v-if="previousOwnedRecord">{{ previousOwnedRecord }}</code>
        </div>
      </div>

      <q-banner v-if="binding?.privateAddress" class="private-address-warning q-mt-md" rounded>
        <template #avatar><q-icon color="warning" name="warning_amber" /></template>
        <strong>Private address:</strong> {{ binding.bindAddress }} may not be reachable from the
        public internet.
      </q-banner>

      <q-banner v-if="actionError" class="dns-binding-error q-mt-md" role="alert" rounded>
        {{ actionError }}
      </q-banner>

      <div class="dns-binding-actions q-mt-lg">
        <q-btn
          :disable="!dirty || !formValid"
          :loading="saving"
          color="primary"
          data-testid="save-dns-binding"
          icon="save"
          label="Save binding"
          no-caps
          @click="save" />
        <q-btn
          :disable="!configured || dirty"
          :loading="syncing"
          data-testid="sync-dns-binding"
          icon="sync"
          label="Sync now"
          no-caps
          outline
          @click="sync" />
        <q-btn
          v-if="configured"
          :disable="syncing"
          :loading="removing"
          color="negative"
          data-testid="remove-dns-binding"
          flat
          label="Remove binding"
          no-caps
          @click="confirmRemove" />
      </div>
      <div class="save-state q-mt-sm" aria-live="polite">
        {{
          dirty ? 'Unsaved binding changes' : configured ? 'Binding saved' : 'No binding configured'
        }}
      </div>
    </template>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'

import {
  AdoptDNSBindingRecordRequestSchema,
  type DNSBinding,
  DNSRecordType,
  DNSSyncResult,
  GetDNSBindingRequestSchema,
  RemoveDNSBindingRequestSchema,
  SetDNSBindingRequestSchema,
  SyncDNSBindingRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ gameServerId: string }>()
const $q = useQuasar()
const binding = ref<DNSBinding>()
const configured = ref(false)
const relativeName = ref('')
const savedRelativeName = ref('')
const loading = ref(true)
const saving = ref(false)
const syncing = ref(false)
const removing = ref(false)
const loadError = ref('')
const actionError = ref('')
const relativeNameError = ref('')

const dirty = computed(() => relativeName.value !== savedRelativeName.value)
const formValid = computed(() => relativeName.value.trim() !== '')
const recordTypeLabel = computed(() => recordTypeName(binding.value?.recordType))
const ownsDesiredRecord = computed(
  () =>
    binding.value?.owned === true &&
    binding.value.ownedFullyQualifiedName === binding.value.fullyQualifiedName &&
    binding.value.ownedRecordType === binding.value.recordType,
)
const ownershipLabel = computed(() => {
  if (!binding.value?.owned) return 'Not owned'
  return ownsDesiredRecord.value ? 'Owned by this binding' : 'Previous record retained'
})
const previousOwnedRecord = computed(() => {
  const current = binding.value
  if (!current?.owned || ownsDesiredRecord.value) return ''
  return `${recordTypeName(current.ownedRecordType)} ${current.ownedFullyQualifiedName} → ${current.ownedValue} · TTL ${current.ownedTtlSeconds}`
})

function recordTypeName(recordType?: DNSRecordType): string {
  return recordType === DNSRecordType.DNS_RECORD_TYPE_AAAA ? 'AAAA' : 'A'
}

function applyBinding(next: DNSBinding): void {
  binding.value = next
  configured.value = true
  relativeName.value = next.relativeName
  savedRelativeName.value = next.relativeName
  relativeNameError.value = ''
  actionError.value = ''
}

async function loadBinding(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().getDNSBinding(
      create(GetDNSBindingRequestSchema, { gameServerId: props.gameServerId }),
    )
    configured.value = response.configured
    if (response.binding) {
      applyBinding(response.binding)
    } else {
      binding.value = undefined
      relativeName.value = ''
      savedRelativeName.value = ''
    }
  } catch (unknownError: unknown) {
    loadError.value = sanitizedError(unknownError)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (!dirty.value || !formValid.value) return
  saving.value = true
  actionError.value = ''
  try {
    const response = await GetXylonaClient().setDNSBinding(
      create(SetDNSBindingRequestSchema, {
        gameServerId: props.gameServerId,
        relativeName: relativeName.value.trim(),
      }),
    )
    if (!response.binding) throw new Error('The saved DNS binding response was empty.')
    applyBinding(response.binding)
    $q.notify({ type: 'positive', message: 'DNS binding saved. No DNS record was changed.' })
  } catch (unknownError: unknown) {
    const error = ConnectError.from(unknownError)
    if (error.code === Code.InvalidArgument) {
      relativeNameError.value = ConnectErrorToString(error)
    } else {
      actionError.value = ConnectErrorToString(error)
    }
  } finally {
    saving.value = false
  }
}

async function sync(): Promise<void> {
  if (!configured.value || dirty.value) return
  syncing.value = true
  actionError.value = ''
  try {
    const response = await GetXylonaClient().syncDNSBinding(
      create(SyncDNSBindingRequestSchema, { gameServerId: props.gameServerId }),
    )
    if (!response.binding) throw new Error('The synchronized DNS binding response was empty.')
    applyBinding(response.binding)
    const messages: Record<DNSSyncResult, string> = {
      [DNSSyncResult.DNS_SYNC_RESULT_UNSPECIFIED]: 'DNS binding synchronized.',
      [DNSSyncResult.DNS_SYNC_RESULT_CREATED]: 'DNS record created and owned.',
      [DNSSyncResult.DNS_SYNC_RESULT_UPDATED]: 'DNS record updated.',
      [DNSSyncResult.DNS_SYNC_RESULT_UNCHANGED]: 'DNS record is already current.',
    }
    $q.notify({ type: 'positive', message: messages[response.result] })
  } catch (unknownError: unknown) {
    const error = ConnectError.from(unknownError)
    actionError.value = ConnectErrorToString(error)
    if (error.code === Code.Aborted && binding.value) binding.value.owned = false
    if (error.code === Code.AlreadyExists) confirmAdoption()
  } finally {
    syncing.value = false
  }
}

function confirmAdoption(): void {
  const name = binding.value?.fullyQualifiedName || 'this record'
  $q.dialog({
    title: 'Adopt existing DNS record?',
    message: `Adopting ${name} gives this binding local ownership without changing DNS. A later Sync now may update it.`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'primary', label: 'Adopt' },
    persistent: true,
  }).onOk(adopt)
}

async function adopt(): Promise<void> {
  syncing.value = true
  actionError.value = ''
  try {
    const response = await GetXylonaClient().adoptDNSBindingRecord(
      create(AdoptDNSBindingRecordRequestSchema, { gameServerId: props.gameServerId }),
    )
    if (!response.binding) throw new Error('The adopted DNS binding response was empty.')
    applyBinding(response.binding)
    $q.notify({
      type: 'positive',
      message: 'Existing DNS record adopted without changing it.',
    })
  } catch (unknownError: unknown) {
    actionError.value = sanitizedError(unknownError)
  } finally {
    syncing.value = false
  }
}

function confirmRemove(): void {
  $q.dialog({
    title: 'Remove DNS binding?',
    message: 'This removes only the local binding. DNS records remain at the provider unchanged.',
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Remove binding' },
    persistent: true,
  }).onOk(remove)
}

async function remove(): Promise<void> {
  removing.value = true
  actionError.value = ''
  try {
    await GetXylonaClient().removeDNSBinding(
      create(RemoveDNSBindingRequestSchema, { gameServerId: props.gameServerId }),
    )
    configured.value = false
    binding.value = undefined
    relativeName.value = ''
    savedRelativeName.value = ''
    $q.notify({ type: 'positive', message: 'Local DNS binding removed. DNS records remain.' })
  } catch (unknownError: unknown) {
    actionError.value = sanitizedError(unknownError)
  } finally {
    removing.value = false
  }
}

function sanitizedError(error: unknown): string {
  return ConnectErrorToString(ConnectError.from(error))
}

onMounted(loadBinding)
</script>

<style scoped>
.dns-binding-settings {
  display: grid;
  gap: var(--xy-space-sm);
}

.manual-only-note {
  color: var(--xy-text-secondary);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
}

.dns-binding-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.dns-binding-summary > div {
  display: grid;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.dns-binding-summary span,
.save-state {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.dns-binding-summary code {
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.private-address-warning {
  color: var(--xy-text-secondary);
  background: var(--xy-warning-bg-faint);
  border: 1px solid var(--xy-warning-border);
}

.dns-binding-error {
  color: var(--xy-text-primary);
  background: var(--xy-error-bg-faint);
  border: 1px solid var(--xy-error-border);
}

.dns-binding-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.dns-binding-state {
  display: grid;
  min-height: 180px;
  place-content: center;
  justify-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  text-align: center;
}
</style>
