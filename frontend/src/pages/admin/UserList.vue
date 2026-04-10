<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Users</h1>
      <div class="xy-page-actions">
        <q-input
          v-model="search"
          aria-label="Search users"
          class="xy-search-input"
          color="primary"
          debounce="300"
          dense
          outlined
          placeholder="Search...">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn color="primary" label="Add User" to="/admin/users/create" />
      </div>
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selected"
        :columns="columns"
        :filter="search"
        :grid="$q.screen.lt.md"
        :loading="loading"
        :rows="rows"
        class="xy-standalone-table"
        flat
        hide-header-in-grid
        row-key="id"
        selection="multiple">
        <template #body-cell-userName="props">
          <q-td :props="props">
            <router-link :to="'/admin/users/' + props.row.id + '/edit'" class="table-link">
              {{ props.row.userName }}
            </router-link>
          </q-td>
        </template>
        <template #body-cell-superUser="props">
          <q-td :props="props">
            <q-icon v-if="props.row.superUser" color="positive" name="check" size="md" />
            <q-icon v-else color="negative" name="close" size="md" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="q-gutter-xs">
              <router-link :to="'/admin/users/' + props.row.id + '/edit'">
                <q-btn :icon="tabSettings" aria-label="Edit user" class="text-main-brighter" flat>
                  <q-tooltip>Edit user</q-tooltip>
                </q-btn>
              </router-link>
              <span>
                <q-btn
                  :icon="tabTrash"
                  aria-label="Delete user"
                  class="text-error-brighter"
                  flat
                  @click="deleteUserAction(props.row)">
                  <q-tooltip>Delete user</q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon class="q-mb-sm text-xy-muted" name="people" size="3rem" />
            <div class="text-subtitle1">No users found</div>
            <div class="text-caption text-xy-muted">Create a user to get started.</div>
          </div>
        </template>
      </q-table>
    </div>
    <user-delete-dialog
      v-model:show-dialog="showUserDeleteDialog"
      :user="selectedActionUser"
      @submit="deleteUserSubmitted"></user-delete-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useStorage } from '@vueuse/core'
import dayjs from 'dayjs'
import { ConnectError } from '@connectrpc/connect'
import { Notify, useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import UserDeleteDialog from '@/components/admin/UserDeleteDialog.vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  ListUsersRequest,
  ListUsersRequestSchema,
  ListUsersResponse,
  User,
} from '@/proto/xylona_pb'

const $q = useQuasar()
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
