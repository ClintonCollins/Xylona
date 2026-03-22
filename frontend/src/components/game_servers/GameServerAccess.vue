<template>
  <q-inner-loading :showing="loading" label="Loading access data..." />

  <template v-if="!loading">
    <q-card-section>
      <div class="row items-center justify-between">
        <div class="text-h6">
          <q-icon name="admin_panel_settings" size="sm" class="text-xy-muted q-mr-xs" />
          Access Management
        </div>
        <q-btn flat icon="refresh" label="Refresh" :loading="loading" @click="loadData"></q-btn>
      </div>
      <q-banner v-if="isRemoteServer" class="bg-info text-white q-mt-md">
        Managing remote server access through federation proxy ({{
          targetNodeHost || 'remote node'
        }}).
      </q-banner>
    </q-card-section>

    <q-card-section class="q-pt-none">
      <div class="access-panel access-panel--local">
        <div class="access-section-header access-section-header--local">
          <q-icon name="person" size="xs" color="primary" class="q-mr-xs" />
          <span>Local Access</span>
          <q-badge
            v-if="localGrants.length > 0"
            :label="localGrants.length"
            color="primary"
            class="q-ml-sm" />
        </div>
        <div class="row q-col-gutter-md q-mb-md">
          <div class="col-12 col-md-4">
            <q-select
              v-model="selectedLocalUserID"
              outlined
              dense
              emit-value
              map-options
              :options="localUserOptions"
              label="User"></q-select>
          </div>
          <div class="col-12 col-md-4">
            <q-select
              v-model="selectedLocalRoleID"
              outlined
              dense
              emit-value
              map-options
              :options="roleOptions"
              label="Role"
              aria-label="Local role"></q-select>
          </div>
          <div class="col-12 col-md-4">
            <q-btn
              class="full-width"
              color="primary"
              label="Grant Local Access"
              :loading="grantingLocal"
              :disable="!selectedLocalUserID || !selectedLocalRoleID"
              @click="grantLocalAccess"></q-btn>
          </div>
        </div>

        <q-list bordered separator aria-live="polite" class="grant-list">
          <q-item v-if="localGrants.length === 0">
            <q-item-section class="empty-state q-pa-lg">
              <q-icon
                name="shield"
                size="40px"
                color="primary"
                class="q-mb-sm"
                style="opacity: 0.3" />
              <div class="text-xy-secondary text-body1">No local access grants</div>
              <div class="text-caption text-xy-muted">
                Grant users access to this server using the form above.
              </div>
            </q-item-section>
          </q-item>
          <transition-group name="grant">
            <q-item v-for="grant in localGrants" :key="grant.id" class="grant-item">
              <q-item-section avatar>
                <q-icon name="person" color="primary" />
              </q-item-section>
              <q-item-section>
                <q-item-label class="grant-username">{{ grant.userName }}</q-item-label>
                <q-item-label caption>
                  <q-badge
                    :label="grant.roleName"
                    outline
                    color="primary"
                    class="q-mr-xs role-badge" />
                  Granted by {{ grant.grantedByUserName }} on
                  {{ formatTimestamp(grant.createdAt) }}
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-btn
                  flat
                  color="negative"
                  icon="delete"
                  label="Revoke"
                  :loading="revokingLocalGrantID === grant.id"
                  @click="confirmRevokeLocal(grant)"></q-btn>
              </q-item-section>
            </q-item>
          </transition-group>
        </q-list>
      </div>
    </q-card-section>

    <q-card-section>
      <div class="access-panel access-panel--federated">
        <div class="access-section-header access-section-header--federated">
          <q-icon name="hub" size="xs" color="accent" class="q-mr-xs" />
          <span>Federated Access</span>
          <q-badge
            v-if="federatedGrants.length > 0"
            :label="federatedGrants.length"
            color="accent"
            class="q-ml-sm" />
        </div>
        <div class="row q-col-gutter-md q-mb-md">
          <div class="col-12 col-md-3">
            <q-select
              v-model="selectedFederatedNodeID"
              outlined
              dense
              emit-value
              map-options
              :options="remoteNodeOptions"
              label="Remote Node"></q-select>
          </div>
          <div class="col-12 col-md-3">
            <q-select
              v-model="selectedRemoteUserID"
              outlined
              dense
              emit-value
              map-options
              :options="remoteUserOptions"
              label="Remote User"
              :loading="loadingRemoteUsers"
              :disable="selectedFederatedNodeID === ''"
              no-options-label="No remote users found"></q-select>
          </div>
          <div class="col-12 col-md-3">
            <q-select
              v-model="selectedFederatedRoleID"
              outlined
              dense
              emit-value
              map-options
              :options="roleOptions"
              label="Role"
              aria-label="Federated role"></q-select>
          </div>
          <div class="col-12 col-md-3">
            <q-btn
              class="full-width"
              color="primary"
              label="Grant Federated Access"
              :loading="grantingFederated"
              :disable="!canGrantFederated"
              @click="grantFederatedAccess"></q-btn>
          </div>
        </div>

        <q-list bordered separator aria-live="polite" class="grant-list">
          <q-item v-if="federatedGrants.length === 0">
            <q-item-section class="empty-state q-pa-lg">
              <q-icon
                name="cloud_off"
                size="40px"
                color="accent"
                class="q-mb-sm"
                style="opacity: 0.3" />
              <div class="text-xy-secondary text-body1">No federated access grants</div>
              <div class="text-caption text-xy-muted">
                Grant remote node users access to this server using the form above.
              </div>
            </q-item-section>
          </q-item>
          <transition-group name="grant">
            <q-item v-for="grant in federatedGrants" :key="grant.id" class="grant-item">
              <q-item-section avatar>
                <q-icon name="hub" color="accent" />
              </q-item-section>
              <q-item-section>
                <q-item-label class="grant-username">
                  {{ grant.remoteUserName }}
                  <span class="text-xy-muted text-caption q-ml-xs">
                    {{ grant.remoteNodeName || grant.remoteNodeId }}
                  </span>
                </q-item-label>
                <q-item-label caption>
                  <q-badge
                    :label="grant.roleName"
                    outline
                    color="accent"
                    class="q-mr-xs role-badge" />
                  Granted by {{ grant.grantedByUserName }} on
                  {{ formatTimestamp(grant.createdAt) }}
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-btn
                  flat
                  color="negative"
                  icon="delete"
                  label="Revoke"
                  :loading="revokingFederatedGrantID === grant.id"
                  @click="confirmRevokeFederated(grant)"></q-btn>
              </q-item-section>
            </q-item>
          </transition-group>
        </q-list>
      </div>
    </q-card-section>
  </template>

  <q-dialog v-model="revokeDialogVisible" aria-labelledby="revoke-dialog-title">
    <q-card style="min-width: min(400px, 90vw)" class="revoke-dialog">
      <q-card-section class="revoke-dialog-header">
        <div class="row items-center no-wrap">
          <q-icon name="warning" size="sm" color="negative" class="q-mr-sm" />
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
          color="negative"
          label="Revoke Access"
          icon="delete"
          :loading="revokingLocalGrantID !== '' || revokingFederatedGrantID !== ''"
          @click="executeRevoke"></q-btn>
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import type { Node } from 'src/proto/shared_pb'
import {
  type FederatedAccessGrantInfo,
  type GameServerAccessGrant,
  GetGameServerRequestSchema,
  GrantFederatedAccessRequestSchema,
  GrantGameServerAccessRequestSchema,
  ListFederatedAccessGrantsRequestSchema,
  ListGameServerAccessGrantsRequestSchema,
  ListNodesRequestSchema,
  ListRemoteNodeUsersRequestSchema,
  ListRolesRequestSchema,
  ListUsersRequestSchema,
  type RemoteUser,
  RevokeFederatedAccessRequestSchema,
  RevokeGameServerAccessRequestSchema,
  type Role,
} from 'src/proto/xylona_pb'
import {
  formatProtoTimestamp,
  hostFromBaseURL,
} from 'src/components/game_servers/game-server-access-utils'
import { GetXylonaClient } from 'src/utils/shared'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

const $q = useQuasar()
const route = useRoute()
const gameServerID = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)

const loading = ref(false)
const loadingRemoteUsers = ref(false)
const grantingLocal = ref(false)
const grantingFederated = ref(false)
const revokingLocalGrantID = ref('')
const revokingFederatedGrantID = ref('')

const roles = ref<Role[]>([])
const localUsers = ref<{ id: string; userName: string; email: string }[]>([])
const localGrants = ref<GameServerAccessGrant[]>([])
const federatedGrants = ref<FederatedAccessGrantInfo[]>([])
const nodes = ref<Node[]>([])
const remoteUsers = ref<RemoteUser[]>([])

const selectedLocalUserID = ref('')
const selectedLocalRoleID = ref('')
const selectedFederatedNodeID = ref('')
const selectedRemoteUserID = ref('')
const selectedFederatedRoleID = ref('')

const isRemoteServer = ref(false)
const targetNodeID = ref('')
const targetNodeHost = ref('')

const revokeDialogVisible = ref(false)
const revokeTargetName = ref('')
const revokeTargetGrantID = ref('')
const revokeTargetType = ref<'local' | 'federated'>('local')

const roleOptions = computed(() => {
  return roles.value.map((role) => ({ label: role.name, value: role.id }))
})

const localUserOptions = computed(() => {
  return localUsers.value.map((user) => ({
    label: `${user.userName} (${user.email || 'no email'})`,
    value: user.id,
  }))
})

const remoteNodeOptions = computed(() => {
  return nodes.value
    .filter((node) => !node.local)
    .map((node) => ({ label: `${node.name} (${node.baseUrl})`, value: node.id }))
})

const remoteUserOptions = computed(() => {
  return remoteUsers.value.map((user) => ({
    label: `${user.userName} (${user.email || 'no email'})`,
    value: user.userId,
  }))
})

const canGrantFederated = computed(() => {
  return (
    selectedFederatedNodeID.value !== '' &&
    selectedRemoteUserID.value !== '' &&
    selectedFederatedRoleID.value !== ''
  )
})

const xylonaClient = GetXylonaClient()

watch(selectedFederatedNodeID, async (nodeID) => {
  selectedRemoteUserID.value = ''
  remoteUsers.value = []
  if (nodeID === '') {
    return
  }
  await loadRemoteNodeUsers(nodeID)
})

onMounted(async () => {
  await loadData()
})

async function loadData() {
  loading.value = true
  try {
    await loadServerClientTarget()
    await Promise.all([
      loadRoles(),
      loadLocalUsers(),
      loadNodes(),
      loadLocalGrants(),
      loadFederatedGrants(),
    ])
  } finally {
    loading.value = false
  }
}

async function loadServerClientTarget() {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    const gameServer = response.gameServer
    if (gameServer === undefined) {
      return
    }

    const nodesResponse = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    const localNode = nodesResponse.nodes.find((node) => node.local)
    if (localNode && gameServer.nodeId !== '' && gameServer.nodeId !== localNode.id) {
      isRemoteServer.value = true
      targetNodeID.value = gameServer.nodeId
      targetNodeHost.value = hostFromBaseURL(gameServer.nodeHost)
      return
    }
    isRemoteServer.value = false
    targetNodeID.value = ''
    targetNodeHost.value = ''
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to resolve server context: ${err.message}`)
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
    if (isRemoteServer.value && targetNodeID.value !== '') {
      const response = await xylonaClient.listRemoteNodeUsers(
        create(ListRemoteNodeUsersRequestSchema, { nodeId: targetNodeID.value }),
      )
      localUsers.value = (response.users ?? []).map((user) => ({
        id: user.userId,
        userName: user.userName,
        email: user.email,
      }))
      return
    }

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

async function loadNodes() {
  try {
    const response = await xylonaClient.listNodes(create(ListNodesRequestSchema, {}))
    nodes.value = response.nodes ? [...response.nodes] : []
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load nodes: ${err.message}`)
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
    notifyError(`Failed to load local access grants: ${err.message}`)
  }
}

async function loadFederatedGrants() {
  try {
    const response = await xylonaClient.listFederatedAccessGrants(
      create(ListFederatedAccessGrantsRequestSchema, { gameServerId: gameServerID.value }),
    )
    federatedGrants.value = response.grants ? [...response.grants] : []
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load federated access grants: ${err.message}`)
  }
}

async function loadRemoteNodeUsers(nodeID: string) {
  loadingRemoteUsers.value = true
  try {
    const response = await xylonaClient.listRemoteNodeUsers(
      create(ListRemoteNodeUsersRequestSchema, { nodeId: nodeID }),
    )
    remoteUsers.value = response.users ? [...response.users] : []
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to load remote users: ${err.message}`)
  } finally {
    loadingRemoteUsers.value = false
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
    notifyError(`Failed to grant local access: ${err.message}`)
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
    notifyError(`Failed to revoke local access: ${err.message}`)
  } finally {
    revokingLocalGrantID.value = ''
  }
}

async function grantFederatedAccess() {
  if (!canGrantFederated.value) {
    return
  }
  grantingFederated.value = true
  try {
    const selectedRemoteUser = remoteUsers.value.find(
      (user) => user.userId === selectedRemoteUserID.value,
    )
    await xylonaClient.grantFederatedAccess(
      create(GrantFederatedAccessRequestSchema, {
        gameServerId: gameServerID.value,
        remoteNodeId: selectedFederatedNodeID.value,
        remoteUserId: selectedRemoteUserID.value,
        remoteUserName: selectedRemoteUser?.userName ?? selectedRemoteUserID.value,
        roleId: selectedFederatedRoleID.value,
      }),
    )
    selectedFederatedRoleID.value = ''
    selectedRemoteUserID.value = ''
    await loadFederatedGrants()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to grant federated access: ${err.message}`)
  } finally {
    grantingFederated.value = false
  }
}

async function revokeFederatedAccess(grantID: string) {
  revokingFederatedGrantID.value = grantID
  try {
    await xylonaClient.revokeFederatedAccess(
      create(RevokeFederatedAccessRequestSchema, {
        grantId: grantID,
        gameServerId: gameServerID.value,
      }),
    )
    await loadFederatedGrants()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    notifyError(`Failed to revoke federated access: ${err.message}`)
  } finally {
    revokingFederatedGrantID.value = ''
  }
}

function confirmRevokeLocal(grant: GameServerAccessGrant) {
  revokeTargetName.value = grant.userName
  revokeTargetGrantID.value = grant.id
  revokeTargetType.value = 'local'
  revokeDialogVisible.value = true
}

function confirmRevokeFederated(grant: FederatedAccessGrantInfo) {
  revokeTargetName.value = `${grant.remoteUserName} (${grant.remoteNodeName || grant.remoteNodeId})`
  revokeTargetGrantID.value = grant.id
  revokeTargetType.value = 'federated'
  revokeDialogVisible.value = true
}

async function executeRevoke() {
  if (revokeTargetType.value === 'local') {
    await revokeLocalAccess(revokeTargetGrantID.value)
  } else {
    await revokeFederatedAccess(revokeTargetGrantID.value)
  }
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
  border-left: 3px solid transparent;
  animation: panel-enter calc(var(--xy-animation-duration) * 0.3s) cubic-bezier(0.16, 1, 0.3, 1)
    both;
}

.access-panel--local {
  border-left-color: var(--xy-primary);
}

.access-panel--federated {
  border-left-color: var(--xy-accent);
  animation-delay: calc(var(--xy-animation-duration) * 0.1s);
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

.access-section-header--federated {
  color: var(--xy-accent-hover);
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
