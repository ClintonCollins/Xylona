<template>
  <q-inner-loading :showing="loading" label="Loading access data..." />

  <template v-if="!loading">
    <q-card-section>
      <div class="row items-center justify-between">
        <div class="text-h6">
          <q-icon class="text-xy-muted q-mr-xs" name="admin_panel_settings" size="sm" />
          Access Management
        </div>
        <q-btn :loading="loading" flat icon="refresh" label="Refresh" @click="loadData"></q-btn>
      </div>
    </q-card-section>

    <q-card-section class="q-pt-none">
      <div class="access-panel access-panel--local">
        <div class="access-section-header access-section-header--local">
          <q-icon class="q-mr-xs" color="primary" name="person" size="xs" />
          <span>Access Grants</span>
          <q-badge
            v-if="localGrants.length > 0"
            :label="localGrants.length"
            class="q-ml-sm"
            color="primary" />
        </div>
        <div class="row q-col-gutter-md q-mb-md">
          <div class="col-12 col-md-4">
            <q-select
              v-model="selectedLocalUserID"
              :options="localUserOptions"
              dense
              emit-value
              label="User"
              map-options
              outlined></q-select>
          </div>
          <div class="col-12 col-md-4">
            <q-select
              v-model="selectedLocalRoleID"
              :options="roleOptions"
              aria-label="Local role"
              dense
              emit-value
              label="Role"
              map-options
              outlined></q-select>
          </div>
          <div class="col-12 col-md-4">
            <q-btn
              :disable="!selectedLocalUserID || !selectedLocalRoleID"
              :loading="grantingLocal"
              class="full-width"
              color="primary"
              label="Grant Access"
              @click="grantLocalAccess"></q-btn>
          </div>
        </div>

        <q-list aria-live="polite" bordered class="grant-list" separator>
          <q-item v-if="localGrants.length === 0">
            <q-item-section class="empty-state q-pa-lg">
              <q-icon
                class="q-mb-sm"
                color="primary"
                name="shield"
                size="40px"
                style="opacity: 0.3" />
              <div class="text-xy-secondary text-body1">No access grants</div>
              <div class="text-caption text-xy-muted">
                Grant users access to this server using the form above.
              </div>
            </q-item-section>
          </q-item>
          <transition-group name="grant">
            <q-item v-for="grant in localGrants" :key="grant.id" class="grant-item">
              <q-item-section avatar>
                <q-icon color="primary" name="person" />
              </q-item-section>
              <q-item-section>
                <q-item-label class="grant-username">{{ grant.userName }}</q-item-label>
                <q-item-label caption>
                  <q-badge
                    :label="grant.roleName"
                    class="q-mr-xs role-badge"
                    color="primary"
                    outline />
                  Granted by {{ grant.grantedByUserName }} on
                  {{ formatTimestamp(grant.createdAt) }}
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-btn
                  :loading="revokingLocalGrantID === grant.id"
                  color="negative"
                  flat
                  icon="delete"
                  label="Revoke"
                  @click="confirmRevokeLocal(grant)"></q-btn>
              </q-item-section>
            </q-item>
          </transition-group>
        </q-list>
      </div>
    </q-card-section>
  </template>

  <q-dialog v-model="revokeDialogVisible" aria-labelledby="revoke-dialog-title">
    <q-card class="revoke-dialog" style="min-width: min(400px, 90vw)">
      <q-card-section class="revoke-dialog-header">
        <div class="row items-center no-wrap">
          <q-icon class="q-mr-sm" color="negative" name="warning" size="sm" />
          <div id="revoke-dialog-title" class="text-h6">Revoke Access</div>
        </div>
      </q-card-section>
      <q-card-section class="q-pt-none">
        Are you sure you want to revoke
        <strong>{{ revokeTargetName }}</strong
        >'s access? This action cannot be undone.
      </q-card-section>
      <q-card-actions align="right" class="q-pa-md">
        <q-btn v-close-popup flat label="Cancel"></q-btn>
        <q-btn
          :loading="revokingLocalGrantID !== ''"
          color="negative"
          icon="delete"
          label="Revoke Access"
          @click="executeRevoke"></q-btn>
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import {
  type GameServerAccessGrant,
  GrantGameServerAccessRequestSchema,
  ListGameServerAccessGrantsRequestSchema,
  ListRolesRequestSchema,
  ListUsersRequestSchema,
  RevokeGameServerAccessRequestSchema,
  type Role,
} from '@/proto/xylona_pb'
import { formatProtoTimestamp } from '@/components/game_servers/game-server-access-utils'
import { GetXylonaClient } from '@/utils/shared'
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

const $q = useQuasar()
const route = useRoute()
const gameServerID = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)

const loading = ref(false)
const grantingLocal = ref(false)
const revokingLocalGrantID = ref('')

const roles = ref<Role[]>([])
const localUsers = ref<{ id: string; userName: string; email: string }[]>([])
const localGrants = ref<GameServerAccessGrant[]>([])

const selectedLocalUserID = ref('')
const selectedLocalRoleID = ref('')

const revokeDialogVisible = ref(false)
const revokeTargetName = ref('')
const revokeTargetGrantID = ref('')

const roleOptions = computed(() => {
  return roles.value.map((role) => ({ label: role.name, value: role.id }))
})

const localUserOptions = computed(() => {
  return localUsers.value.map((user) => ({
    label: `${user.userName} (${user.email || 'no email'})`,
    value: user.id,
  }))
})

const xylonaClient = GetXylonaClient()

onMounted(async () => {
  await loadData()
})

async function loadData() {
  loading.value = true
  try {
    await Promise.all([loadRoles(), loadLocalUsers(), loadLocalGrants()])
  } finally {
    loading.value = false
  }
}

async function loadRoles() {
  try {
    const response = await xylonaClient.listRoles(create(ListRolesRequestSchema, {}))
    roles.value = response.roles ? [...response.roles] : []
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load roles: ${err.message}`)
  }
}

async function loadLocalUsers() {
  try {
    const response = await xylonaClient.listUsers(create(ListUsersRequestSchema, {}))
    localUsers.value = (response.users ?? []).map((user) => ({
      id: user.id,
      userName: user.userName,
      email: user.email,
    }))
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load users: ${err.message}`)
  }
}

async function loadLocalGrants() {
  try {
    const response = await xylonaClient.listGameServerAccessGrants(
      create(ListGameServerAccessGrantsRequestSchema, { gameServerId: gameServerID.value }),
    )
    localGrants.value = response.grants ? [...response.grants] : []
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load access grants: ${err.message}`)
  }
}

async function grantLocalAccess() {
  if (selectedLocalUserID.value === '' || selectedLocalRoleID.value === '') {
    return
  }
  grantingLocal.value = true
  try {
    await xylonaClient.grantGameServerAccess(
      create(GrantGameServerAccessRequestSchema, {
        gameServerId: gameServerID.value,
        userId: selectedLocalUserID.value,
        roleId: selectedLocalRoleID.value,
      }),
    )
    selectedLocalUserID.value = ''
    selectedLocalRoleID.value = ''
    await loadLocalGrants()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to grant access: ${err.message}`)
  } finally {
    grantingLocal.value = false
  }
}

async function revokeLocalAccess(grantID: string) {
  revokingLocalGrantID.value = grantID
  try {
    await xylonaClient.revokeGameServerAccess(
      create(RevokeGameServerAccessRequestSchema, {
        grantId: grantID,
        gameServerId: gameServerID.value,
      }),
    )
    await loadLocalGrants()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to revoke access: ${err.message}`)
  } finally {
    revokingLocalGrantID.value = ''
  }
}

function confirmRevokeLocal(grant: GameServerAccessGrant) {
  revokeTargetName.value = grant.userName
  revokeTargetGrantID.value = grant.id
  revokeDialogVisible.value = true
}

async function executeRevoke() {
  await revokeLocalAccess(revokeTargetGrantID.value)
  revokeDialogVisible.value = false
}

function formatTimestamp(ts?: { seconds: bigint }) {
  return formatProtoTimestamp(ts)
}

function notifyError(message: string) {
  $q.notify({
    type: 'xylona-error',
    position: 'top',
    caption: message,
    timeout: 5000,
  })
}
</script>

<style scoped>
@keyframes panel-enter {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.access-panel {
  background-color: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  padding: var(--xy-space-md);
  animation: panel-enter calc(var(--xy-animation-duration) * 0.3s) cubic-bezier(0.16, 1, 0.3, 1)
    both;
}

.access-panel--local {
  background-color: color-mix(in srgb, var(--xy-primary) 4%, var(--xy-surface-0));
  border-color: var(--xy-primary-border-soft);
}

.access-section-header {
  display: flex;
  align-items: center;
  font-family: var(--xy-font-display);
  font-size: 0.8rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-text-secondary);
  margin-bottom: var(--xy-space-md);
}

.access-section-header--local {
  color: var(--xy-primary-hover);
}

.grant-list {
  background-color: var(--xy-surface-1);
  border-radius: 6px;
}

.grant-item {
  transition: background-color var(--xy-transition-fast);
}

.grant-item:hover {
  background-color: var(--xy-surface-2);
}

.grant-username {
  font-weight: 500;
}

.role-badge {
  font-family: var(--xy-font-mono);
  font-size: 0.7rem;
  letter-spacing: 0.02em;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.revoke-dialog-header {
  border-bottom: 1px solid var(--xy-danger-border);
  background-color: var(--xy-danger-bg);
}

.grant-enter-active {
  transition:
    opacity var(--xy-transition-base) cubic-bezier(0.16, 1, 0.3, 1),
    transform var(--xy-transition-base) cubic-bezier(0.16, 1, 0.3, 1);
}

.grant-leave-active {
  transition:
    opacity var(--xy-transition-fast) cubic-bezier(0.16, 1, 0.3, 1),
    transform var(--xy-transition-fast) cubic-bezier(0.16, 1, 0.3, 1);
}

.grant-enter-from {
  opacity: 0;
  transform: translateX(-8px);
}

.grant-leave-to {
  opacity: 0;
  transform: translateX(8px);
}
</style>
