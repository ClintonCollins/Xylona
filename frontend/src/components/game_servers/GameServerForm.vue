<template>
  <q-card-section>
    <div class="row">
      <div class="text-h6" v-text="existingGameServerId ? 'Edit Game Server' : 'Create Game Server'"></div>
    </div>
  </q-card-section>
  <q-card-section>
    <q-form class="q-pa-lg">
      <div class="row wrap q-col-gutter-md justify-between">
        <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name"
                 v-model="gameServer.name"></q-input>
        <q-select class="col-12 col-xl-6" outlined type="text" label="User" emit-value
                  :options="availableUsers"
                  v-model="gameServer.userId" option-label="label" map-options
                  options-selected-class="selected-option"></q-select>
        <q-select class="col-12 col-xl-6" outlined type="text" label="Game" emit-value
                  :options="availableGames"
                  v-model="gameServer.gameId" option-label="label" map-options
                  options-selected-class="selected-option"></q-select>
        <q-select class="col-12 col-xl-6" outlined type="text" label="Node" emit-value
                  :options="nodes"
                  v-model="gameServer.nodeId" option-label="name" map-options option-value="id"
                  options-selected-class="selected-option"></q-select>
        <q-select class="col-12 col-xl-6" outlined type="text" label="IP Address" emit-value
                  :options="availableIPs"
                  v-model="gameServer.ip" option-label="address"
                  options-selected-class="selected-option"></q-select>
        <q-input v-if="props.existingGameServerId !== undefined" class="col-12 col-xl-6" outlined
                 type="text"
                 label="Start Command"
                 v-model="gameServer.startCommand"></q-input>
        <q-input class="col-12 col-xl-6" outlined type="text" label="Set Players"
                 v-model.number="setPlayers"></q-input>
        <q-input class="col-12 col-xl-6" outlined type="text" label="Max Players"
                 v-model.number="maxPlayers"></q-input>
        <q-input v-if="gameServer.gameId === 'minecraft'" class="col-12 col-xl-6" outlined type="text"
                 label="Max Memory MB"
                 v-model.number="maxMemoryMB"></q-input>
        <q-input class="col-12 col-xl-6" outlined type="text" label="Port"
                 v-model.number="port"></q-input>
        <q-input class="col-12 col-xl-6" outlined type="text" label="Query Port"
                 v-model.number="queryPort"></q-input>
      </div>
    </q-form>
  </q-card-section>
  <q-separator></q-separator>
  <q-card-actions class="q-pa-md" align="right">
    <q-btn flat label="Cancel" @click="cancel"></q-btn>
    <q-btn label="Save" color="primary" @click="submitGameServer"></q-btn>
  </q-card-actions>
  <q-inner-loading
    :showing="formSubmitting"
    label="Saving game configuration..."
    label-class="text-primary"
  ></q-inner-loading>
</template>

<script setup lang="ts">
import {create} from '@bufbuild/protobuf'
import {GetXylonaClient} from '@/utils/shared'
import {onMounted, Ref, ref, watch} from 'vue'
import {useRouter} from 'vue-router'
import {
  CreateGameServerRequest, CreateGameServerRequestSchema, EditGameServerRequest, EditGameServerRequestSchema, Game,
  GameServer, GameServerSchema, IP, Node
} from 'src/proto/shared_pb'
import {
  GetGameServerRequest, GetGameServerRequestSchema,
  ListGamesRequest, ListGamesRequestSchema, ListGamesResponse, ListIPsRequest, ListIPsRequestSchema, ListIPsResponse,
  ListNodesRequest, ListNodesRequestSchema, ListNodesResponse,
  ListUsersRequest,
  ListUsersRequestSchema
} from 'src/proto/xylona_pb'

const router = useRouter()

const props = defineProps({
  existingGameServerId: {
    type: String,
    required: false,
    default: undefined
  }
})

// Is this a new game server or an existing one?
const newGameServer: Ref<boolean> = ref(true)

const gameServer = ref(create(GameServerSchema, {}))
const availableGames = ref<Array<Record<string, string>>>([])
const availableUsers = ref<Array<Record<string, string>>>([])
const availableIPs = ref<Array<IP>>([])
const gamesMap = ref(new Map<string, Game>())
const nodes = ref<Array<Node>>([])

const formSubmitting = ref(false)
const port = ref(0)
const queryPort = ref(0)
const setPlayers = ref(0)
const maxPlayers = ref(0)
const maxMemoryMB = ref(1024)

onMounted(async () => {
  if (props.existingGameServerId) {
    await getGameServerDetails()
  }
  await getGames()
  await getNodes()
  await getUsers()
  await getIPs()
})

watch(port, (newVal) => {
  gameServer.value.port = BigInt(newVal)
})

watch(queryPort, (newVal) => {
  gameServer.value.queryPort = BigInt(newVal)
})

watch(() => gameServer.value.gameId, (newVal) => {
  port.value = Number(gamesMap.value.get(newVal)?.defaultPort ?? 0)
  queryPort.value = Number(gamesMap.value.get(newVal)?.defaultQueryPort ?? 0)
  maxPlayers.value = Number(gamesMap.value.get(newVal)?.defaultMaxPlayers ?? 0)
  setPlayers.value = Number(gamesMap.value.get(newVal)?.defaultMaxPlayers ?? 0)
})

async function cancel() {
  router.back()
}

async function getGameServerDetails() {
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = props.existingGameServerId
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }
    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
  }
}

async function getGames() {
  const request: ListGamesRequest = create(ListGamesRequestSchema, {})
  try {
    const response: ListGamesResponse = await GetXylonaClient().listGames(request)
    response.games.forEach((game) => {
      availableGames.value.push({label: game.name, value: game.id})
      gamesMap.value.set(game.id, game)
    })
    if (availableGames.value.length === 0) {
      return
    }
    let foundGame = gamesMap.value.get(gameServer.value.gameId)
    if (foundGame !== undefined) {
      gameServer.value.gameId = foundGame.id
    } else {
      foundGame = gamesMap.value.get(availableGames.value[0].value)
    }
    gameServer.value.gameId = foundGame.id ?? ''
    gameServer.value.gameName = foundGame.name ?? ''
    port.value = Number(foundGame.defaultPort ?? 0)
    queryPort.value = Number(foundGame.defaultQueryPort ?? 0)
    maxPlayers.value = Number(foundGame.defaultMaxPlayers ?? 0)
    setPlayers.value = Number(foundGame.defaultMaxPlayers ?? 0)
  } catch (e) {
    console.error(e)
  }
}

async function getNodes() {
  const request: ListNodesRequest = create(ListNodesRequestSchema, {})
  try {
    const response: ListNodesResponse = await GetXylonaClient().listNodes(request)
    nodes.value = []
    response.nodes.forEach((node) => {
      nodes.value.push(node)
    })
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    console.error(err.message)
  }
}

async function getUsers() {
  const request: ListUsersRequest = create(ListUsersRequestSchema, {})
  try {
    const response = await GetXylonaClient().listUsers(request)
    response.users.forEach((user) => {
      availableUsers.value.push({label: user.userName, value: user.id})
    })
    if (availableUsers.value.length === 0) {
      return
    }
    gameServer.value.userId = availableUsers.value[0].value
    gameServer.value.userName = availableUsers.value[0].label
  } catch (e) {
    console.error(e)
  }
}

async function getIPs() {
  const request: ListIPsRequest = create(ListIPsRequestSchema, {})
  try {
    const response: ListIPsResponse = await GetXylonaClient().listIPs(request)
    response.ips.forEach((ip) => {
      availableIPs.value.push(ip)
      // availableIPs.value.push({label: `${ip.address} ${ip.external ? '(External)' : ''}`, value: ip.address})
      if (ip.external) {
        gameServer.value.ip = ip
      }
    })
    if (availableIPs.value.length === 0) {
      return
    }
  } catch (e) {
    console.error(e)
  }
}

async function updateGameServer() {
  const request: EditGameServerRequest = create(EditGameServerRequestSchema, {})
  request.gameServer = gameServer.value as GameServer
  request.serverId = props.existingGameServerId
  request.gameServer.port = BigInt(port.value)
  request.gameServer.queryPort = BigInt(queryPort.value)
  request.gameServer.ip = gameServer.value.ip
  try {
    const response = await GetXylonaClient().editGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
  } catch (e) {
    console.error(e)
  }
}

async function createGameServer() {
  const request: CreateGameServerRequest = create(CreateGameServerRequestSchema, {})
  request.gameServer = gameServer.value as GameServer
  request.gameServer.port = BigInt(port.value)
  request.gameServer.queryPort = BigInt(queryPort.value)
  request.gameServer.nodeId = gameServer.value.nodeId
  try {
    const response = await GetXylonaClient().createGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
  } catch (e) {
    console.error(e)
  }
}

async function submitGameServer() {
  if (props.existingGameServerId) {
    await updateGameServer()
  } else {
    await createGameServer()
  }
}

</script>

<style scoped>

</style>
