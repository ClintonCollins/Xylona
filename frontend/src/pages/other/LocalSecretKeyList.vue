<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Nodes"
                                :rows="rows"
                                :columns="columns"
                                row-key="name"
                                selection="multiple"
                                :filter="search"
                                v-model:pagination="initialPagination"
                                v-model:selected="selected">
                            <template v-slot:top>
                                <div class="row col flex justify-between flex-center">
                                    <div class="col-12 col-md-6">
                                        <span class="text-h6">Local Secret Keys</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" label="Create Secret Key" @click="showSecretKeyFormDialog = true"/>
                                            <q-input dense debounce="300" color="primary" v-model="search">
                                                <template v-slot:append>
                                                    <q-icon name="search"/>
                                                </template>
                                            </q-input>
                                        </div>
                                    </div>
                                </div>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs">
                                        <span>
                                            <q-btn flat class="text-error-brighter"
                                                   :icon="tabTrash" @click="deleteSecretKeyAction(props.row)">
                                                <q-tooltip>Delete secret key</q-tooltip>
                                            </q-btn>
                                        </span>
                                    </div>
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
            <SecretKeyDeleteDialog :secret-key="selectedActionSecretKey" v-model:showDialog="showSecretKeyDeleteDialog"
                                   @submit="deleteSecretKeySubmitted"></SecretKeyDeleteDialog>
            <SecretKeyFormDialog v-model:showDialog="showSecretKeyFormDialog" @submit="getSecretKeys"></SecretKeyFormDialog>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useStorage } from '@vueuse/core'
import dayjs from 'dayjs'
import { tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth } from 'src/utils/shared'
import { SecretKey } from 'src/proto/shared_pb'
import { ListLocalSecretKeysRequest, ListLocalSecretKeysRequestSchema
} from 'src/proto/xylona_pb'
import SecretKeyDeleteDialog from 'src/components/keys/SecretKeyDeleteDialog.vue'
import SecretKeyFormDialog from '../../components/keys/SecretKeyFormDialog.vue'

const windowWidth = WindowWidth()
const rows = ref([] as SecretKey[])
const search: Ref<string> = ref('')
const showSecretKeyDeleteDialog = ref(false)
const showSecretKeyFormDialog = ref(false)
const selectedActionSecretKey = ref<SecretKey | null>(null)

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1
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
    field: (row: { name: any; }) => row.name,
    sortable: true
  },
  {
    name: 'last_accessed_from',
    label: 'Last Accessed From',
    align: 'left',
    field: (row: { last_accessed_from: any; }) => row.last_accessed_from,
    sortable: true
  },
  {
    name: 'last_used',
    label: 'Last Used',
    align: 'left',
    field: (row: { last_used: any; }) => row.last_used ? dayjs(timestampDate(row.last_used)).format('MM/DD/YYYY HH:mm:ss A') : '',
    sortable: true
  },
  {
    name: 'createdAt',
    label: 'Created At',
    align: 'left',
    field: (row: { createdAt: Timestamp; }) => dayjs(timestampDate(row.createdAt)).format('MM/DD/YYYY HH:mm:ss A'),
    sortable: true
  },
  {
    name: 'actions',
    label: '',
    align: 'center',
    field: () => ''
  }
])

</script>

<style scoped>

</style>
