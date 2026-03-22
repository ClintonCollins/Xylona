<template>
  <div class="node-api-key-settings">
    <div class="xy-section-header q-mb-md">
      <div class="text-h6">External Service API Keys</div>
      <div class="text-caption text-xy-secondary">
        Configure API keys for external services like Steam and CurseForge. These keys enable
        features such as mod browsing and Steam Workshop integration.
      </div>
    </div>

    <q-table
      flat
      class="xy-standalone-table"
      :rows="apiKeys"
      :columns="columns"
      row-key="serviceName"
      :loading="loading"
      hide-pagination
      :rows-per-page-options="[0]">
      <template #body-cell-serviceName="slotProps">
        <q-td :props="slotProps">
          {{ getServiceLabel(slotProps.row.serviceName) }}
        </q-td>
      </template>
      <template #body-cell-maskedKey="slotProps">
        <q-td :props="slotProps">
          <code class="text-xy-secondary">{{ slotProps.row.maskedKey }}</code>
        </q-td>
      </template>
      <template #body-cell-actions="slotProps">
        <q-td :props="slotProps">
          <div class="q-gutter-xs">
            <q-btn
              flat
              dense
              class="text-main-brighter"
              icon="edit"
              aria-label="Edit API key"
              @click="startEdit(slotProps.row)">
              <q-tooltip>Edit key</q-tooltip>
            </q-btn>
            <q-btn
              flat
              dense
              class="text-error-brighter"
              icon="delete"
              aria-label="Delete API key"
              @click="confirmDelete(slotProps.row)">
              <q-tooltip>Delete key</q-tooltip>
            </q-btn>
          </div>
        </q-td>
      </template>
      <template #no-data>
        <div class="full-width column items-center q-pa-lg text-xy-secondary">
          <q-icon name="vpn_key" size="3rem" class="q-mb-sm text-xy-muted" />
          <div class="text-subtitle1">No API keys configured</div>
          <div class="text-caption text-xy-muted">
            Add an API key to enable external service integrations.
          </div>
        </div>
      </template>
    </q-table>

    <div class="q-mt-md">
      <q-btn v-if="!showForm" color="primary" label="Add Key" icon="add" @click="startAdd" />
    </div>

    <q-card v-if="showForm" flat class="q-mt-md q-pa-md xy-surface-1">
      <div class="text-subtitle2 q-mb-sm">
        {{ editingServiceName ? 'Edit API Key' : 'Add API Key' }}
      </div>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-sm-4">
          <q-select
            v-model="formServiceName"
            :options="availableServices"
            label="Service"
            outlined
            dense
            emit-value
            map-options
            :disable="!!editingServiceName"
            :rules="[(val: string) => !!val || 'Service is required']" />
        </div>
        <div class="col-12 col-sm-8">
          <q-input
            v-model="formApiKey"
            label="API Key"
            outlined
            dense
            type="password"
            :rules="[(val: string) => !!val || 'API key is required']" />
        </div>
      </div>
      <div class="q-mt-sm q-gutter-sm">
        <q-btn
          color="primary"
          label="Save"
          :loading="saving"
          :disable="!formServiceName || !formApiKey"
          @click="saveKey" />
        <q-btn flat label="Cancel" color="neutral" @click="cancelForm" />
      </div>
    </q-card>

    <q-dialog v-model="showDeleteDialog" persistent backdrop-filter="brightness(15%)">
      <q-card>
        <q-card-section>
          <div class="text-h6 text-error">Delete API Key</div>
        </q-card-section>
        <q-card-section>
          <p>
            Are you sure you want to delete the
            <strong>{{ getServiceLabel(deleteTargetServiceName) }}</strong> API key?
            <span class="text-bold">This action cannot be undone.</span>
          </p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn label="Cancel" color="neutral" flat @click="showDeleteDialog = false" />
          <q-btn label="Delete" class="bg-error" :loading="deleting" @click="deleteKey" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { NodeApiKey } from '@/proto/shared_pb'
import {
  DeleteNodeApiKeyRequestSchema,
  ListNodeApiKeysRequestSchema,
  SetNodeApiKeyRequestSchema,
} from '@/proto/xylona_pb'

const services = [
  { label: 'Steam Web API', value: 'steam_web_api' },
  { label: 'CurseForge', value: 'curseforge' },
]

const apiKeys = ref<NodeApiKey[]>([])
const loading = ref(false)
const saving = ref(false)
const deleting = ref(false)

const showForm = ref(false)
const editingServiceName = ref('')
const formServiceName = ref('')
const formApiKey = ref('')

const showDeleteDialog = ref(false)
const deleteTargetServiceName = ref('')

const columns = [
  {
    name: 'serviceName',
    label: 'Service',
    align: 'left' as const,
    field: (row: NodeApiKey) => row.serviceName,
    sortable: true,
  },
  {
    name: 'maskedKey',
    label: 'API Key',
    align: 'left' as const,
    field: (row: NodeApiKey) => row.maskedKey,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
  },
]

const availableServices = computed(() => {
  if (editingServiceName.value) {
    return services
  }
  const configuredNames = new Set(apiKeys.value.map((k) => k.serviceName))
  return services.filter((s) => !configuredNames.has(s.value))
})

function getServiceLabel(serviceName: string): string {
  const service = services.find((s) => s.value === serviceName)
  return service ? service.label : serviceName
}

onMounted(async () => {
  await loadKeys()
})

async function loadKeys(): Promise<void> {
  loading.value = true
  try {
    const response = await GetXylonaClient().listNodeApiKeys(
      create(ListNodeApiKeysRequestSchema, {}),
    )
    apiKeys.value = response.apiKeys
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  } finally {
    loading.value = false
  }
}

function startAdd(): void {
  editingServiceName.value = ''
  formServiceName.value = ''
  formApiKey.value = ''
  showForm.value = true
}

function startEdit(key: NodeApiKey): void {
  editingServiceName.value = key.serviceName
  formServiceName.value = key.serviceName
  formApiKey.value = ''
  showForm.value = true
}

function cancelForm(): void {
  showForm.value = false
  editingServiceName.value = ''
  formServiceName.value = ''
  formApiKey.value = ''
}

async function saveKey(): Promise<void> {
  if (!formServiceName.value || !formApiKey.value) {
    return
  }
  saving.value = true
  try {
    await GetXylonaClient().setNodeApiKey(
      create(SetNodeApiKeyRequestSchema, {
        serviceName: formServiceName.value,
        apiKey: formApiKey.value,
      }),
    )
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: `${getServiceLabel(formServiceName.value)} API key saved`,
      timeout: 5000,
    })
    cancelForm()
    await loadKeys()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  } finally {
    saving.value = false
  }
}

function confirmDelete(key: NodeApiKey): void {
  deleteTargetServiceName.value = key.serviceName
  showDeleteDialog.value = true
}

async function deleteKey(): Promise<void> {
  deleting.value = true
  try {
    await GetXylonaClient().deleteNodeApiKey(
      create(DeleteNodeApiKeyRequestSchema, {
        serviceName: deleteTargetServiceName.value,
      }),
    )
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: `${getServiceLabel(deleteTargetServiceName.value)} API key deleted`,
      timeout: 5000,
    })
    showDeleteDialog.value = false
    deleteTargetServiceName.value = ''
    await loadKeys()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  } finally {
    deleting.value = false
  }
}
</script>

<style scoped></style>
