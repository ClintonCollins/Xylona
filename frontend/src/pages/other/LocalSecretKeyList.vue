<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="xy-page-title">Secret Keys</div>
      <div class="xy-page-actions">
        <q-input
          v-model="search"
          dense
          outlined
          debounce="300"
          color="primary"
          placeholder="Search..."
          aria-label="Search secret keys"
          style="min-width: 200px">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn color="primary" label="Create Secret Key" @click="showSecretKeyFormDialog = true" />
      </div>
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selected"
        flat
        class="xy-standalone-table"
        :grid="$q.screen.lt.md"
        :rows="rows"
        :columns="columns"
        row-key="name"
        selection="multiple"
        :filter="search"
        hide-header-in-grid>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="q-gutter-xs">
              <span>
                <q-btn
                  flat
                  class="text-error-brighter"
                  :icon="tabTrash"
                  aria-label="Delete secret key"
                  @click="deleteSecretKeyAction(props.row)">
                  <q-tooltip>Delete secret key</q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon name="vpn_key" size="3rem" class="q-mb-sm text-xy-muted" />
            <div class="text-subtitle1">No secret keys</div>
            <div class="text-caption text-xy-muted">Generate a secret key for node pairing.</div>
          </div>
        </template>
      </q-table>
    </div>
    <secret-key-delete-dialog
      v-model:show-dialog="showSecretKeyDeleteDialog"
      :secret-key="selectedActionSecretKey"
      @submit="deleteSecretKeySubmitted"></secret-key-delete-dialog>
    <secret-key-form-dialog
      v-model:show-dialog="showSecretKeyFormDialog"
      @submit="getSecretKeys"></secret-key-form-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useStorage } from '@vueuse/core'
import dayjs from 'dayjs'
import { tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { onMounted, Ref, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { SecretKey } from '@/proto/shared_pb'
import { ListLocalSecretKeysRequest, ListLocalSecretKeysRequestSchema } from 'src/proto/xylona_pb'
import SecretKeyDeleteDialog from '@/components/keys/SecretKeyDeleteDialog.vue'
import SecretKeyFormDialog from '../../components/keys/SecretKeyFormDialog.vue'

const $q = useQuasar()
const rows = ref([] as SecretKey[])
const search: Ref<string> = ref('')
const showSecretKeyDeleteDialog = ref(false)
const showSecretKeyFormDialog = ref(false)
const selectedActionSecretKey = ref<SecretKey | null>(null)

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1,
})

onMounted(async () => {
  await getSecretKeys()
})

async function getSecretKeys() {
  const request: ListLocalSecretKeysRequest = create(ListLocalSecretKeysRequestSchema, {})
  try {
    const response = await GetXylonaClient().listLocalSecretKeys(request)
    rows.value = []
    response.secretKeys.forEach((secretKey) => {
      rows.value.push(secretKey)
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load secret keys: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function deleteSecretKeyAction(secretKey: SecretKey) {
  selectedActionSecretKey.value = secretKey
  showSecretKeyDeleteDialog.value = true
}

async function deleteSecretKeySubmitted(error: unknown | boolean) {
  if (!error) {
    void getSecretKeys()
  }
}

const selected = ref([])
const columns = ref([
  {
    name: 'name',
    label: 'Name',
    required: true,
    align: 'left',
    field: (row: { name: any }) => row.name,
    sortable: true,
  },
  {
    name: 'last_accessed_from',
    label: 'Last Accessed From',
    align: 'left',
    field: (row: { last_accessed_from: any }) => row.last_accessed_from,
    sortable: true,
  },
  {
    name: 'last_used',
    label: 'Last Used',
    align: 'left',
    field: (row: { last_used: any }) =>
      row.last_used ? dayjs(timestampDate(row.last_used)).format('MM/DD/YYYY HH:mm:ss A') : '',
    sortable: true,
  },
  {
    name: 'createdAt',
    label: 'Created At',
    align: 'left',
    field: (row: { createdAt: Timestamp }) =>
      dayjs(timestampDate(row.createdAt)).format('MM/DD/YYYY HH:mm:ss A'),
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center',
    field: () => '',
  },
])
</script>

<style scoped></style>
