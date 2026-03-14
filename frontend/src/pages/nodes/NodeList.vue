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
                                        <span class="text-h6">Nodes</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" to="/nodes/add" label="Add Node"/>
                                            <q-input dense debounce="300" color="primary" v-model="search">
                                                <template v-slot:append>
                                                    <q-icon name="search"/>
                                                </template>
                                            </q-input>
                                        </div>
                                    </div>
                                </div>
                            </template>
                            <template v-slot:body-cell-name="props">
                                <q-td :props="props">
                                    <router-link class="table-link" :to="'/nodes/'+props.row.id+'/edit'">{{ props.row.name }}
                                    </router-link>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-windows_support="props">
                                <q-td :props="props">
                                    <q-icon name="check" size="md" v-if="props.row.windowsSupport" color="green"/>
                                    <q-icon name="close" size="md" v-else color="red"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-linux_support="props">
                                <q-td :props="props">
                                    <q-icon name="check" size="md" v-if="props.row.linuxSupport" color="green"/>
                                    <q-icon name="close" size="md" v-else color="red"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs">
                                        <router-link :to="'/nodes/' + props.row.id + '/edit'">
                                            <q-btn flat class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit node</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <span>
                                            <q-btn flat class="text-error-brighter"
                                                   :icon="tabTrash" @click="deleteNodeAction(props.row)">
                                                <q-tooltip>Delete node</q-tooltip>
                                            </q-btn>
                                        </span>
                                    </div>
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
            <NodeDeleteDialog :node="selectedActionNode" v-model:showDialog="showNodeDeleteDialog" @submit="deleteNodeSubmitted"></NodeDeleteDialog>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useStorage } from '@vueuse/core'
import NodeDeleteDialog from '@/components/nodes/NodeDeleteDialog.vue'
import {
  tabSettings,
  tabTrash, tabKey
} from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth } from '@/utils/shared'
import { Node } from '@/proto/shared_pb'
import { ListNodesRequest, ListNodesRequestSchema, ListNodesResponse } from '../../proto/xylona_pb'

const windowWidth = WindowWidth()
const rows = ref([] as Node[])
const search: Ref<string> = ref('')
const showNodeDeleteDialog = ref(false)
const selectedActionNode = ref<Node | null>(null)

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1
})

onMounted(async () => {
  await getNodes()
})

async function getNodes() {
  const request: ListNodesRequest = create(ListNodesRequestSchema, {})
  try {
    const response: ListNodesResponse = await GetXylonaClient().listNodes(request)
    rows.value = []
    response.nodes.forEach((node) => {
      rows.value.push(node)
    })
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    console.error(err.message)
  }
}

async function deleteNodeAction(node: Node) {
  selectedActionNode.value = node
  showNodeDeleteDialog.value = true
}

async function deleteNodeSubmitted(error: unknown | boolean) {
  if (!error) {
    void getNodes()
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
    name: 'host',
    label: 'Host',
    align: 'left',
    field: (row: { host: any; }) => row.host,
    sortable: true
  },
  {
    name: 'port',
    label: 'Port',
    align: 'left',
    field: (row: { port: any; }) => row.port,
    sortable: true
  },
  {
    name: 'local',
    label: 'Local',
    align: 'left',
    field: (row: { local: boolean; }) => row.local,
    sortable: true
  },
  {
    name: 'node_id',
    label: 'ID',
    required: true,
    align: 'left',
    field: (row: { id: any; }) => row.id,
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
