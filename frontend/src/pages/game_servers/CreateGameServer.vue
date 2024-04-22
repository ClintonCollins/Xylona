<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <q-card class="full-width">
        <q-card-section>
          <div class="row">
            <div class="text-h6">Create Game Server</div>
          </div>
        </q-card-section>
        <q-card-section>
          <q-form class="q-pa-lg">
            <div class="row wrap q-col-gutter-md justify-between">
              <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name"
                       v-model="newGameServer.name"></q-input>
              <q-select class="col-12 col-xl-6" outlined type="text" label="User" emit-value :options="availableUsers"
                        v-model="newGameServer.userId" option-label="label" map-options options-selected-class="selected-option"></q-select>
              <q-select class="col-12 col-xl-6" outlined type="text" label="Game" emit-value :options="availableGames"
                        v-model="newGameServer.gameId" option-label="label" map-options options-selected-class="selected-option"></q-select>
              <q-select class="col-12 col-xl-6" outlined type="text" label="IP Address" emit-value :options="availableIPs"
                        v-model="newGameServer.ip" option-label="address" options-selected-class="selected-option"></q-select>
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
          <q-btn label="Save" color="primary" @click="addGameServer"></q-btn>
        </q-card-actions>
        <q-inner-loading
          :showing="formSubmitting"
          label="Saving game configuration..."
          label-class="text-primary"
        ></q-inner-loading>
      </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">

import {
  CreateGameServerRequest,
  Game,
  GameServer, IP,
  ListGamesRequest,
  ListGamesResponse,
  ListIPsRequest, ListIPsResponse,
  ListUsersRequest
} from 'src/proto/xylona_pb'
import {onMounted, ref, watch} from 'vue'
import {GetXylonaClient} from 'src/utils/shared'
import {useRouter} from "vue-router";

const router = useRouter()

const newGameServer = ref(new GameServer())
const availableGames = ref<Array<Record<string, string>>>([])
const availableUsers = ref<Array<Record<string, string>>>([])
const availableIPs = ref<Array<IP>>([])
const gamesMap = ref(new Map<string, Game>())

const formSubmitting = ref(false)
const port = ref(0)
const queryPort = ref(0)

onMounted(async () => {
  await getGames()
  await getUsers()
  await getIPs()
})

watch(port, (newVal) => {
  console.log(newGameServer.value.port)
  newGameServer.value.port = BigInt(newVal)
})

watch(queryPort, (newVal) => {
  newGameServer.value.queryPort = BigInt(newVal)
})

watch(() => newGameServer.value.gameId, (newVal) => {
  port.value = Number(gamesMap.value.get(newVal)?.defaultPort ?? 0)
  queryPort.value = Number(gamesMap.value.get(newVal)?.defaultQueryPort ?? 0)
})

async function cancel() {
  router.back()
}

async function getGames() {
  const request = new ListGamesRequest()
  try {
    const response: ListGamesResponse = await GetXylonaClient().listGames(request)
    response.games.forEach((game) => {
      availableGames.value.push({label: game.name, value: game.id})
      gamesMap.value.set(game.id, game)
    })
    if (availableGames.value.length === 0) {
      return
    }
    newGameServer.value.gameId = gamesMap.value.get(availableGames.value[0].value)?.id ?? ''
    newGameServer.value.gameName = gamesMap.value.get(availableGames.value[0].value)?.name ?? ''
    port.value = Number(gamesMap.value.get(availableGames.value[0].value)?.defaultPort ?? 0)
    queryPort.value = Number(gamesMap.value.get(availableGames.value[0].value)?.defaultQueryPort ?? 0)
  } catch (e) {
    console.error(e)
  }
}

async function getUsers() {
  const request = new ListUsersRequest()
  try {
    const response = await GetXylonaClient().listUsers(request)
    response.users.forEach((user) => {
      availableUsers.value.push({label: user.userName, value: user.id})
    })
    if (availableUsers.value.length === 0) {
      return
    }
    newGameServer.value.userId = availableUsers.value[0].value
    newGameServer.value.userName = availableUsers.value[0].label
  } catch (e) {
    console.error(e)
  }
}

async function getIPs() {
  const request = new ListIPsRequest()
  try {
    const response: ListIPsResponse = await GetXylonaClient().listIPs(request)
    response.ips.forEach((ip) => {
      availableIPs.value.push(ip)
      // availableIPs.value.push({label: `${ip.address} ${ip.external ? '(External)' : ''}`, value: ip.address})
      if (ip.external) {
        newGameServer.value.ip = ip
      }
    })
    if (availableIPs.value.length === 0) {
      return
    }
  } catch (e) {
    console.error(e)
  }
}

async function addGameServer() {
  const request = new CreateGameServerRequest()
  request.gameServer = newGameServer.value as GameServer
  request.gameServer.port = BigInt(port.value)
  request.gameServer.queryPort = BigInt(queryPort.value)
  try {
    const response = await GetXylonaClient().createGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
    console.log(response)
  } catch (e) {
    console.error(e)
  }
}

</script>

<style scoped>

</style>
