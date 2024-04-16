<template>
  <q-card-section>
    <div class="row">
      <div class="text-h6">Your Game Server</div>
    </div>
  </q-card-section>
  <div class="row q-gutter-lg">
    <div class="column col-lg-3 col-12 q-gutter-lg">
      <q-list bordered separator>
        <q-item clickable>
          <q-item-section>Name</q-item-section>
          <q-item-section side>{{ gameServer.name }}</q-item-section>
        </q-item>
        <q-item clickable>
          <q-item-section>Status</q-item-section>
          <q-item-section side>{{ gameServer.status }}</q-item-section>
        </q-item>
        <q-item clickable>
          <q-item-section>Game</q-item-section>
          <q-item-section side>{{ gameServer.gameName }}</q-item-section>
        </q-item>
        <q-item clickable>
          <q-item-section>IP</q-item-section>
          <q-item-section side>{{ gameServer.ip?.address }}</q-item-section>
        </q-item>
        <q-item clickable>
          <q-item-section>Port</q-item-section>
          <q-item-section side>{{ gameServer.port }}</q-item-section>
        </q-item>
        <!--        <q-item clickable>-->
        <!--          <q-item-section>Max Players</q-item-section>-->
        <!--          <q-item-section side>{{ gameServer.maxPlayers}}</q-item-section>-->
        <!--        </q-item>-->
        <!--        <q-item clickable>-->
        <!--          <q-item-section>Memory</q-item-section>-->
        <!--          <q-item-section side>{{ gameServer.memoryBytes}} / {{ gameServer.maxMemoryMb}}</q-item-section>-->
        <!--        </q-item>-->
      </q-list>
    </div>
    <div class="column col-lg-8 col-12">
      <q-scroll-area ref="consoleScrollArea" id="consoleContainer">
        <code class="q-pb-md" id="consoleCodeEl" style="white-space: pre-wrap" v-html="gameServerOutput"></code>
      </q-scroll-area>
      <q-input id="consoleInput" hint="Send to console"
               dense square filled name="consoleInput"
               v-model="serverInput">
        <template v-slot:append>
          <q-btn flat color="primary" icon="send" name="send"></q-btn>
        </template>
      </q-input>
    </div>
  </div>
  <div class="row q-gutter-lg">
    <div class="col q-gutter-md">
      <q-btn :disable="disableStartButton" color="green-10" label="Start" @click="startGameServer"></q-btn>
      <q-btn :disable="disableStopButton" color="red-14" label="Stop" @click="stopGameServer"></q-btn>
    </div>
  </div>
  <q-card-section>
  </q-card-section>
</template>

<script setup lang="ts">
import {useRoute, useRouter} from "vue-router";
import {onMounted, Ref, ref} from "vue";
import {
  GameServer,
  GetGameServerRequest, ReadGameServerOutputRequest,
  ReadGameServerOutputResponse,
  StartGameServerRequest
} from "src/proto/xylona_pb";
import {useToolbarNavQTabsStore} from "stores/xylona";
import {GetXylonaClient} from "src/utils/shared";
import {QScrollArea} from "quasar";


const gameServerOutput = ref("")
const router = useRouter()
const route = useRoute()
const gameServer: Ref<GameServer> = ref(new GameServer()) as Ref<GameServer>
const serverInput = ref("")
const gameServerId: Ref<string> = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
const disableStartButton = ref(false)
const disableStopButton = ref(false)
const consoleScrollArea = ref<QScrollArea | null>(null)

onMounted(async () => {
  await getGameServerOutput()
  await getGameServerDetails()
  streamGameServerOutput()
})

useToolbarNavQTabsStore().changeTabs([
  {name: "Console", to: "/game-servers/" + route.params.id + "/console", icon: "terminal", exact: true},
  {name: "Files", to: "/game-servers/" + route.params.id + "/files", icon: "folder", exact: true},
])
const serverInfoList = ref([
  {
    label: "Name",
    value: gameServer.value.name
  },
  {
    label: "Status",
    value: "Loading..."
  },
  {
    label: "Game",
    value: "Loading..."
  },
  {
    label: "IP",
    value: "Loading..."
  },
  {
    label: "Port",
    value: "Loading..."
  },
  {
    label: "Players",
    value: "Loading..."
  },
  {
    label: "Memory",
    value: "Loading..."
  }
])

async function getGameServerDetails() {
  const request = new GetGameServerRequest()
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }
    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
  }
}

async function startGameServer() {
  const request = new StartGameServerRequest()
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().startGameServer(request)
  } catch (e) {
    console.error(e)
  }
}

async function stopGameServer() {
  const request = new StartGameServerRequest()
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().stopGameServer(request)
  } catch (e) {
    console.error(e)
  }
}

async function getGameServerOutput() {
  const request = new ReadGameServerOutputRequest()
  try {
    request.serverId = gameServerId.value
    const response: ReadGameServerOutputResponse = await GetXylonaClient().readGameServerOutput(request)
    gameServerOutput.value = response.output
    if (consoleScrollArea.value === null) {
      return
    }
    consoleScrollArea.value.setScrollPercentage("vertical", 100, 0)
  } catch (e) {
    console.error(e)
  }
}

function streamGameServerOutput() {
  const apiWebsocket = new WebSocket(`wss://localhost/api/websocket/game_server_stream_output/${gameServerId.value}`)
  apiWebsocket.onopen = () => {
    console.log("Websocket opened")
  }
  apiWebsocket.onmessage = (event) => {
    gameServerOutput.value += event.data
    if (consoleScrollArea.value === null) {
      return
    }
    consoleScrollArea.value.setScrollPercentage("vertical", 100, 0)
  }
}

</script>

<style scoped>
#consoleContainer {
  height: 400px;
  overflow-y: auto;
  overflow-x: hidden;
  white-space: pre-wrap;
  max-width: 100%;
  background-color: rgba(168, 167, 167, 0.07);
}

#consoleInput {
  background-color: rgba(211, 207, 207, 0.1);
  border: none;
}
</style>
