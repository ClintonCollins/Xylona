<template>
  <q-card-section>
    <div class="row items-center justify-between">
      <div class="text-h6">Access Management</div>
      <q-btn flat icon="refresh" label="Refresh" :loading="loading" @click="loadData"></q-btn>
    </div>
    <q-banner v-if="isRemoteServer" class="bg-info text-white q-mt-md">
      Managing remote server access through federation proxy ({{
        targetNodeHost || 'remote node'
      }}).
    </q-banner>
  </q-card-section>

  <q-card-section class="q-pt-none">
    <div class="access-section-header">
      <q-icon name="person" size="xs" color="primary" class="q-mr-xs" />
      <span>Local Access</span>
    </div>
    <div class="row q-col-gutter-md q-mb-md">
      <div class="col-12 col-md-4">
        <q-select
          outlined
          dense
          emit-value
          map-options
          v-model="selectedLocalUserID"
          :options="localUserOptions"
          label="User"></q-select>
      </div>
      <div class="col-12 col-md-4">
        <q-select
          outlined
          dense
          emit-value
          map-options
          v-model="selectedLocalRoleID"
          :options="roleOptions"
          label="Role"></q-select>
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

    <q-list bordered separator>
      <q-item v-if="localGrants.length === 0">
        <q-item-section class="text-center q-pa-md">
          <q-icon name="lock_open" size="32px" color="grey-7" class="q-mb-xs" />
          <div class="text-xy-secondary">No local access grants</div>
          <div class="text-caption text-xy-muted">
            Grant users access to this server using the form above.
          </div>
        </q-item-section>
      </q-item>
      <q-item v-for="grant in localGrants" :key="grant.id">
        <q-item-section>
          <q-item-label>{{ grant.userName }}</q-item-label>
          <q-item-label caption>
            Role: {{ grant.roleName }} | Granted by: {{ grant.grantedByUserName }} |
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
            @click="revokeLocalAccess(grant.id)"></q-btn>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card-section>

  <q-separator></q-separator>

  <q-card-section>
    <div class="access-section-header">
      <q-icon name="hub" size="xs" color="accent" class="q-mr-xs" />
      <span>Federated Access</span>
    </div>
    <div class="row q-col-gutter-md q-mb-md">
      <div class="col-12 col-md-3">
        <q-select
          outlined
          dense
          emit-value
          map-options
          v-model="selectedFederatedNodeID"
          :options="remoteNodeOptions"
          label="Remote Node"></q-select>
      </div>
      <div class="col-12 col-md-3">
        <q-select
          outlined
          dense
          emit-value
          map-options
          v-model="selectedRemoteUserID"
          :options="remoteUserOptions"
          label="Remote User"
          :loading="loadingRemoteUsers"
          :disable="selectedFederatedNodeID === ''"></q-select>
      </div>
      <div class="col-12 col-md-3">
        <q-select
          outlined
          dense
          emit-value
          map-options
          v-model="selectedFederatedRoleID"
          :options="roleOptions"
          label="Role"></q-select>
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

    <q-list bordered separator>
      <q-item v-if="federatedGrants.length === 0">
        <q-item-section class="text-center q-pa-md">
          <q-icon name="cloud_off" size="32px" color="grey-7" class="q-mb-xs" />
          <div class="text-xy-secondary">No federated access grants</div>
          <div class="text-caption text-xy-muted">
            Grant remote node users access to this server using the form above.
          </div>
        </q-item-section>
      </q-item>
      <q-item v-for="grant in federatedGrants" :key="grant.id">
        <q-item-section>
          <q-item-label
            >{{ grant.remoteUserName }} ({{
              grant.remoteNodeName || grant.remoteNodeId
            }})</q-item-label
          >
          <q-item-label caption>
            Role: {{ grant.roleName }} | Granted by: {{ grant.grantedByUserName }} |
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
            @click="revokeFederatedAccess(grant.id)"></q-btn>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card-section>
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
.access-section-header {
  display: flex;
  align-items: center;
  font-family: var(--xy-font-display);
  font-size: 1rem;
  font-weight: 600;
  margin-bottom: 12px;
}
</style>
