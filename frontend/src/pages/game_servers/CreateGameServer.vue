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
            <div class="row q-col-gutter-md wrap">
              <q-input class="col-12 col-xl-6" filled type="text" autofocus label="Name"
                       v-model="newGameServer.name"
                       :model-value="newGameServer.name"></q-input>
              <q-select class="col-12 col-xl-6" filled type="text" label="User" emit-value
                        :options="availableUsers"
                        v-model="newGameServer.userId" :model-value="newGameServer.userName"></q-select>
              <q-select class="col-12 col-xl-6" filled type="text" label="Game" emit-value
                        :options="availableGames"
                        v-model="newGameServer.gameId" :model-value="newGameServer.gameName"></q-select>
              <q-select class="col-12 col-xl-6" filled type="text" label="IP Address" emit-value
                        :options="availableIPs"
                        v-model="newGameServer.ip"
                        :model-value="newGameServer.ip?.address ? newGameServer.ip.address : newGameServer.ip"></q-select>
              <!--              <q-input class="col-12 col-xl-6" filled type="number" label="Max Backups"-->
              <!--                       v-model="newGameServer.maxBackups"-->
              <!--                       :model-value="newGameServer.maxBackups.toString()"></q-input>-->
              <q-input class="col-12 col-xl-6" filled type="text" label="Port"
                       v-model="newGameServer.port"
                       :model-value="newGameServer.port.toString()"></q-input>
              <q-input class="col-12 col-xl-6" filled type="text" label="Query Port"
                       v-model="newGameServer.queryPort"
                       :model-value="newGameServer.queryPort.toString()"></q-input>
              <!--              <q-checkbox class="col-12" filled type="text" label="Backups Enabled"-->
              <!--                          v-model="newGameServer.backupsEnabled"-->
              <!--                          :model-value="newGameServer.backupsEnabled"></q-checkbox>-->
            </div>
          </q-form>
        </q-card-section>
        <q-separator></q-separator>
        <q-card-actions class="q-pa-md" align="right">
          <q-btn flat label="Cancel" @click="$emit('update:show', false)"></q-btn>
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
  GameServer,
  ListGamesRequest,
  ListGamesResponse,
  ListIPsRequest,
  ListUsersRequest
} from "src/proto/xylona_pb";
import {onMounted, ref, Ref, watch} from "vue";
import {GetXylonaClient} from "src/utils/shared";

const formSubmitting = ref(false)
const newGameServer = ref(new GameServer())
const availableGames = ref<Array<Record<string, string>>>([])
const availableUsers = ref<Array<Record<string, string>>>([])
const availableIPs = ref<Array<Record<string, string>>>([])
const gamesMap = ref(new Map<string, Game>())

onMounted(async () => {
  await getGames()
  await getUsers()
  await getIPs()
})

watch(newGameServer, (newVal, oldValue) => {
  console.log(newVal.gameId, oldValue.gameId)
  if (newVal.gameId !== oldValue.gameId) {
    console.log(newVal.gameId)
    newGameServer.value.port = gamesMap.value.get(newVal.gameId)?.defaultPort ?? 0 as unknown as bigint
  }
  console.log(newGameServer.value.ip)
})

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
    newGameServer.value.port = gamesMap.value.get(availableGames.value[0].value)?.defaultPort ?? 0 as unknown as bigint
    newGameServer.value.queryPort = gamesMap.value.get(availableGames.value[0].value)?.defaultQueryPort ?? 0 as unknown as bigint
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
    const response = await GetXylonaClient().listIPs(request)
    response.ips.forEach((ip) => {
      availableIPs.value.push({label: `${ip.address} ${ip.external ? '(External)' : ''}`, value: ip.address})
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
  try {
    const response = await GetXylonaClient().createGameServer(request)
    console.log(response)
  } catch (e) {
    console.error(e)
  }
}

</script>

<style scoped>

</style>
