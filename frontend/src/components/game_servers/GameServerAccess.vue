<template>
  <div class="access-page xy-page-content">
    <page-header subtitle="Give users a role on this server." title="Access">
      <template #actions>
        <q-btn :loading="loading" flat icon="refresh" label="Refresh" no-caps @click="loadData" />
      </template>
    </page-header>

    <q-inner-loading :showing="loading" label="Loading access data..." />

    <section v-if="!loading" aria-labelledby="access-grants-heading" class="access-panel">
      <h2 id="access-grants-heading" class="xy-section-overline">
        Access grants
        <span v-if="localGrants.length > 0">· {{ localGrants.length }}</span>
      </h2>
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

      <empty-state
        v-if="localGrants.length === 0"
        description="Grant users access to this server using the form above."
        icon="shield"
        title="No access grants" />
      <q-list v-else aria-live="polite" bordered class="grant-list" separator>
        <transition-group name="grant">
          <q-item v-for="grant in localGrants" :key="grant.id" class="grant-item">
            <q-item-section avatar>
              <q-icon class="text-xy-muted" name="person" />
            </q-item-section>
            <q-item-section>
              <q-item-label class="grant-username">{{ grant.userName }}</q-item-label>
              <q-item-label caption>
                <q-badge :label="grant.roleName" class="q-mr-xs" color="grey-8" />
                Granted by {{ grant.grantedByUserName }} on
                <span class="xy-num">{{ formatTimestamp(grant.createdAt) }}</span>
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-btn
                :aria-label="`Revoke ${grant.userName}'s access`"
                :loading="revokingLocalGrantID === grant.id"
                color="negative"
                flat
                icon="delete"
                label="Revoke"
                no-caps
                @click="confirmRevokeLocal(grant)"></q-btn>
            </q-item-section>
          </q-item>
        </transition-group>
      </q-list>
    </section>

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
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
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
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import { GetXylonaClient } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
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
    notifyError(`Failed to load roles: ${connectErrorMessage(unknownError)}`)
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
    notifyError(`Failed to load users: ${connectErrorMessage(unknownError)}`)
  }
}

async function loadLocalGrants() {
  try {
    const response = await xylonaClient.listGameServerAccessGrants(
      create(ListGameServerAccessGrantsRequestSchema, { gameServerId: gameServerID.value }),
    )
    localGrants.value = response.grants ? [...response.grants] : []
  } catch (unknownError: unknown) {
    notifyError(`Failed to load access grants: ${connectErrorMessage(unknownError)}`)
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
    notifyError(`Failed to grant access: ${connectErrorMessage(unknownError)}`)
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
    notifyError(`Failed to revoke access: ${connectErrorMessage(unknownError)}`)
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

.access-page {
  position: relative;
}

.access-panel {
  background-color: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  padding: var(--xy-space-md);
  animation: panel-enter calc(var(--xy-animation-duration) * 0.3s) cubic-bezier(0.16, 1, 0.3, 1)
    both;
}

.access-panel .xy-section-overline {
  margin-top: 0;
  margin-bottom: var(--xy-space-md);
}

.grant-list {
  background-color: var(--xy-surface-1);
  border-radius: var(--xy-radius-md);
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
