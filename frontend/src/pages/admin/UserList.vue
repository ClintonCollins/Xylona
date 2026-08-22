<template>
  <q-page class="xy-page-content">
    <page-header title="Users">
      <template #actions>
        <q-input
          v-model="search"
          aria-label="Search users"
          class="xy-search-input"
          color="primary"
          debounce="300"
          dense
          outlined
          placeholder="Search users">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn color="primary" label="Add user" to="/admin/users/create" />
      </template>
    </page-header>
    <div v-if="loadError" class="list-error" role="alert" aria-live="assertive">
      <q-icon name="sync_problem" size="sm" />
      <div>
        <strong>Users could not be loaded.</strong>
        <span>{{ loadError }}</span>
      </div>
      <q-btn :loading="loading" dense flat icon="refresh" label="Retry" @click="getUsers" />
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
        <template #item="props">
          <div class="user-grid-item col-12 col-sm-6">
            <q-card class="user-mobile-card" flat>
              <q-card-section class="user-mobile-header">
                <q-checkbox
                  v-model="props.selected"
                  :aria-label="`Select ${props.row.userName}`"
                  dense />
                <div class="user-mobile-identity">
                  <router-link :to="`/admin/users/${props.row.id}/edit`" class="user-mobile-name">
                    {{ props.row.userName }}
                  </router-link>
                  <span>{{
                    [props.row.firstName, props.row.lastName].filter(Boolean).join(' ')
                  }}</span>
                </div>
                <q-badge
                  :color="props.row.superUser ? 'warning' : 'grey-8'"
                  :label="props.row.superUser ? 'Administrator' : 'User'" />
              </q-card-section>

              <q-card-section class="user-mobile-details">
                <div>
                  <span>Email</span>
                  <strong>{{ props.row.email || 'Not provided' }}</strong>
                </div>
                <div>
                  <span>Created</span>
                  <strong>{{ formatCreatedAt(props.row.createdAt) || 'Not available' }}</strong>
                </div>
              </q-card-section>

              <q-card-actions class="user-mobile-actions">
                <q-btn
                  :to="`/admin/users/${props.row.id}/edit`"
                  color="primary"
                  flat
                  icon="edit"
                  label="Edit"
                  no-caps />
                <q-space />
                <q-btn
                  :aria-label="`Delete ${props.row.userName}`"
                  class="text-error-brighter"
                  flat
                  icon="delete"
                  @click="deleteUserAction(props.row)">
                  <q-tooltip>Delete user</q-tooltip>
                </q-btn>
              </q-card-actions>
            </q-card>
          </div>
        </template>
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
            <div class="text-subtitle1">{{ search ? 'No matching users' : 'No users yet' }}</div>
            <div class="text-caption text-xy-muted">
              {{ search ? 'Try a different search.' : 'Create a user to get started.' }}
            </div>
            <q-btn
              v-if="!search"
              class="q-mt-md"
              color="primary"
              label="Add user"
              to="/admin/users/create" />
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
import { Timestamp } from '@bufbuild/protobuf/wkt'
import { usePersistedRef } from '@/utils/persisted-ref'
import { ConnectError } from '@connectrpc/connect'
import { Notify, useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import UserDeleteDialog from '@/components/admin/UserDeleteDialog.vue'
import { formatDate } from '@/utils/format-timestamp'
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
const loadError = ref('')
const search: Ref<string> = ref('')
const showUserDeleteDialog = ref(false)
const selectedActionUser = ref<User | null>(null)

const initialPagination = usePersistedRef('user-pagination', {
  rowsPerPage: 25,
  page: 1,
})

onMounted(async () => {
  await getUsers()
})

async function getUsers() {
  loading.value = true
  loadError.value = ''
  const request: ListUsersRequest = create(ListUsersRequestSchema, {})
  try {
    const response: ListUsersResponse = await GetXylonaClient().listUsers(request)
    rows.value = [...response.users]
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    loadError.value = ConnectErrorToString(err)
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

function formatCreatedAt(createdAt?: Timestamp): string {
  return formatDate(createdAt)
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
    field: (row: { createdAt?: Timestamp }) => formatCreatedAt(row.createdAt),
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

<style scoped>
.list-error {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
}

.list-error > div {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.list-error span {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.user-grid-item {
  padding: var(--xy-space-xs);
}

.user-mobile-card {
  height: 100%;
  overflow: hidden;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.user-mobile-header {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
}

.user-mobile-identity {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.user-mobile-identity > span {
  overflow: hidden;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-mobile-name {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-mobile-details {
  display: grid;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.user-mobile-details > div {
  display: grid;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.user-mobile-details span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.user-mobile-details strong {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-mobile-actions {
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

@media (max-width: 599px) {
  .user-grid-item {
    padding-inline: 0;
  }
}
</style>
