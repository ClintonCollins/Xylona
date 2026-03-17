<template>
  <q-page :padding="windowWidth > 1024">
    <div class="row justify-center">
      <q-card class="col">
        <q-card-section>
          <div class="q-pa-md">
            <q-table
              flat
              title="Users"
              :rows="rows"
              :columns="columns"
              row-key="id"
              selection="multiple"
              :filter="search"
              :loading="loading"
              v-model:pagination="initialPagination"
              v-model:selected="selected">
              <template v-slot:top>
                <div class="row col flex justify-between flex-center">
                  <div class="col-12 col-md-6">
                    <span class="text-h6">Users</span>
                  </div>
                  <div class="col-12 col-md-6">
                    <div class="row flex q-gutter-xl justify-end">
                      <q-btn color="primary" to="/admin/users/create" label="Add User" />
                      <q-input dense debounce="300" color="primary" v-model="search">
                        <template v-slot:append>
                          <q-icon name="search" />
                        </template>
                      </q-input>
                    </div>
                  </div>
                </div>
              </template>
              <template v-slot:body-cell-userName="props">
                <q-td :props="props">
                  <router-link class="table-link" :to="'/admin/users/' + props.row.id + '/edit'">
                    {{ props.row.userName }}
                  </router-link>
                </q-td>
              </template>
              <template v-slot:body-cell-superUser="props">
                <q-td :props="props">
                  <q-icon name="check" size="md" v-if="props.row.superUser" color="green" />
                  <q-icon name="close" size="md" v-else color="red" />
                </q-td>
              </template>
              <template v-slot:body-cell-actions="props">
                <q-td :props="props">
                  <div class="q-gutter-xs">
                    <router-link :to="'/admin/users/' + props.row.id + '/edit'">
                      <q-btn flat class="text-main-brighter" :icon="tabSettings">
                        <q-tooltip>Edit user</q-tooltip>
                      </q-btn>
                    </router-link>
                    <span>
                      <q-btn
                        flat
                        class="text-error-brighter"
                        :icon="tabTrash"
                        @click="deleteUserAction(props.row)">
                        <q-tooltip>Delete user</q-tooltip>
                      </q-btn>
                    </span>
                  </div>
                </q-td>
              </template>
            </q-table>
          </div>
        </q-card-section>
      </q-card>
      <UserDeleteDialog
        :user="selectedActionUser"
        v-model:showDialog="showUserDeleteDialog"
        @submit="deleteUserSubmitted"></UserDeleteDialog>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useStorage } from '@vueuse/core'
import dayjs from 'dayjs'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import UserDeleteDialog from '@/components/admin/UserDeleteDialog.vue'
import { ConnectErrorToString, GetXylonaClient, WindowWidth } from '@/utils/shared'
import { User } from '@/proto/xylona_pb'
import { ListUsersRequest, ListUsersRequestSchema, ListUsersResponse } from '@/proto/xylona_pb'

const windowWidth = WindowWidth()
const rows = ref([] as User[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showUserDeleteDialog = ref(false)
const selectedActionUser = ref<User | null>(null)

const initialPagination = useStorage('user-pagination', {
  rowsPerPage: 25,
  page: 1,
})

onMounted(async () => {
  await getUsers()
})

async function getUsers() {
  loading.value = true
  const request: ListUsersRequest = create(ListUsersRequestSchema, {})
  try {
    const response: ListUsersResponse = await GetXylonaClient().listUsers(request)
    rows.value = []
    response.users.forEach((user) => {
      rows.value.push(user)
    })
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
    console.error(err.message)
  } finally {
    loading.value = false
  }
}

async function deleteUserAction(user: User) {
  selectedActionUser.value = user
  showUserDeleteDialog.value = true
}

async function deleteUserSubmitted(error: unknown | boolean) {
  if (!error) {
    void getUsers()
  }
}

const selected = ref([])
const columns = ref([
  {
    name: 'userName',
    label: 'Username',
    required: true,
    align: 'left',
    field: (row: { userName: string }) => row.userName,
    sortable: true,
  },
  {
    name: 'email',
    label: 'Email',
    align: 'left',
    field: (row: { email: string }) => row.email,
    sortable: true,
  },
  {
    name: 'firstName',
    label: 'First Name',
    align: 'left',
    field: (row: { firstName: string }) => row.firstName,
    sortable: true,
  },
  {
    name: 'lastName',
    label: 'Last Name',
    align: 'left',
    field: (row: { lastName: string }) => row.lastName,
    sortable: true,
  },
  {
    name: 'superUser',
    label: 'Super User',
    align: 'left',
    field: (row: { superUser: boolean }) => row.superUser,
    sortable: true,
  },
  {
    name: 'createdAt',
    label: 'Created At',
    align: 'left',
    field: (row: { createdAt?: Timestamp }) =>
      row.createdAt ? dayjs(timestampDate(row.createdAt)).format('MM/DD/YYYY HH:mm:ss A') : '',
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
