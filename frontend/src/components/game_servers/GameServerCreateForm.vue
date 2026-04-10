<template>
  <game-server-form-shell
    :form-submitting="formSubmitting"
    :loading="loading"
    :save-disabled="loading"
    :save-ready="createDeploymentReady"
    breadcrumb-label="New Server"
    guidance="Fields marked * are required."
    header-title="Create Game Server"
    subtitle="Set the server route and limits. The footer only flags what still blocks save."
    @cancel="cancel"
    @save="submitGameServer">
    <q-form ref="formRef" class="server-form-layout" greedy>
      <section class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--accent">
            <q-icon name="badge" size="14px" />
          </span>
          <span class="section-title font-display">Identity</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model="gameServer.name"
            :rules="serverNameRules"
            autofocus
            class="col-12 col-md-6"
            label="Server Name *"
            lazy-rules
            maxlength="80"
            outlined
            reactive-rules
            type="text" />
          <q-select
            v-model="gameServer.gameId"
            :options="availableGames"
            :rules="gameRules"
            class="col-12 col-md-6"
            emit-value
            label="Game *"
            lazy-rules
            map-options
            option-label="label"
            outlined
            reactive-rules
            @update:model-value="onGameSelected" />
        </div>
      </section>

      <section class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--primary">
            <q-icon name="hub" size="14px" />
          </span>
          <span class="section-title font-display">Placement</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-select
            v-model="gameServer.userId"
            :options="availableUsers"
            :rules="ownerRules"
            class="col-12 col-md-4"
            emit-value
            label="Owner *"
            lazy-rules
            map-options
            option-label="label"
            outlined
            reactive-rules />
          <q-select
            v-model="gameServer.nodeId"
            :options="nodes"
            :rules="nodeRules"
            class="col-12 col-md-4"
            emit-value
            label="Node *"
            lazy-rules
            map-options
            option-label="name"
            option-value="id"
            outlined
            reactive-rules />
          <q-select
            v-model="gameServer.ip"
            :options="availableIPs"
            :rules="ipRules"
            class="col-12 col-md-4"
            label="IP Address *"
            lazy-rules
            option-label="address"
            outlined
            reactive-rules />
        </div>
      </section>

      <section class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--success">
            <q-icon name="lan" size="14px" />
          </span>
          <span class="section-title font-display">Networking</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model.number="portModel"
            :error="showPortAvailabilityError"
            :error-message="portAvailabilityErrorMessage"
            :rules="portRules"
            class="col-12 col-sm-6"
            label="Port *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-model.number="queryPortModel"
            :rules="queryPortRules"
            class="col-12 col-sm-6"
            label="Query Port *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
        </div>
      </section>

      <section class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--muted">
            <q-icon name="terminal" size="14px" />
          </span>
          <span class="section-title font-display">Launch</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model="gameServer.serverExecutable"
            class="col-12 col-lg-6"
            hint="Optional override for the {{SERVER_EXECUTABLE}} launch placeholder."
            label="Server Executable"
            outlined
            type="text" />
        </div>
      </section>

      <section class="form-section form-section--last">
        <div class="section-header">
          <span class="section-icon section-icon--warning">
            <q-icon name="memory" size="14px" />
          </span>
          <span class="section-title font-display">Capacity</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model.number="setPlayersModel"
            :rules="setPlayersRules"
            class="col-12 col-sm-6 col-lg-4"
            label="Set Players *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-model.number="maxPlayersModel"
            :rules="maxPlayersRules"
            class="col-12 col-sm-6 col-lg-4"
            label="Max Players *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-if="isMinecraftGame"
            v-model.number="maxMemoryModel"
            :rules="maxMemoryRules"
            class="col-12 col-lg-4"
            label="Max Memory MB *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
        </div>
      </section>

      <section class="form-section form-section--summary">
        <Transition mode="out-in" name="deployment-state">
          <div v-if="createDeploymentReady" key="ready" class="deployment-ready">
            <div class="deployment-ready-icon">
              <q-icon name="task_alt" size="16px" />
            </div>
            <div class="deployment-ready-content">
              <span class="deployment-ready-label font-display">Ready to Deploy</span>
              <span class="deployment-ready-value">{{ deploymentReadyText }}</span>
            </div>
          </div>

          <div v-else key="review" class="deployment-review">
            <div class="deployment-review-heading">
              <span class="section-icon section-icon--muted">
                <q-icon name="fact_check" size="14px" />
              </span>
              <div class="deployment-review-copy">
                <span class="deployment-review-title font-display">
                  Needs Attention
                  <span aria-hidden="true" class="deployment-review-dot"></span>
                </span>
                <span class="deployment-review-subtitle text-xy-muted">
                  Only blocking or conflicting setup appears here.
                </span>
              </div>
            </div>

            <div class="deployment-summary">
              <article
                v-for="item in createDeploymentWarningItems"
                :key="item.label"
                class="deployment-summary-item deployment-summary-item--warning">
                <div class="deployment-summary-icon is-warning">
                  <q-icon :name="item.icon" size="15px" />
                </div>
                <div class="deployment-summary-content">
                  <span class="deployment-summary-label">{{ item.label }}</span>
                  <span class="deployment-summary-value">{{ item.value }}</span>
                </div>
              </article>
            </div>
          </div>
        </Transition>
      </section>
    </q-form>
  </game-server-form-shell>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'

import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import GameServerFormShell from './GameServerFormShell.vue'
import { useGameServerPortAvailability } from './game-server-port-availability'
import { useGameServerFormState } from './game-server-form-state'
import { CreateGameServerRequest, CreateGameServerRequestSchema } from '@/proto/shared_pb'

const router = useRouter()
const $q = useQuasar()

const {
  availableGames,
  availableIPs,
  availableUsers,
  deploymentReady,
  deploymentReadyText,
  deploymentWarningItems,
  formRef,
  formSubmitting,
  gameRules,
  gameServer,
  initialize,
  ipRules,
  isMinecraftGame,
  loading,
  maxMemoryModel,
  maxMemoryRules,
  maxPlayersModel,
  maxPlayersRules,
  nodeRules,
  nodes,
  onGameSelected,
  ownerRules,
  portModel,
  portRules,
  queryPortModel,
  queryPortRules,
  resetSubmissionState,
  selectedGame,
  serverNameRules,
  setPlayersModel,
  setPlayersRules,
  startSubmitting,
  validateBeforeSave,
} = useGameServerFormState({
  loadProvisioningOptions: true,
})

const {
  ensurePortAvailabilityBeforeSave,
  portAvailabilityBlocking,
  portAvailabilityChecking,
  portAvailabilityMessage,
  portAvailabilityState,
  portAvailabilityVisible,
} = useGameServerPortAvailability({
  enabled: computed(() => !loading.value),
  gameServer,
  selectedGame,
})

const createDeploymentReady = computed(
  () => deploymentReady.value && !portAvailabilityBlocking.value && !portAvailabilityChecking.value,
)
const showPortAvailabilityError = computed(() => portAvailabilityBlocking.value)
const portAvailabilityErrorMessage = computed(() =>
  portAvailabilityBlocking.value ? portAvailabilityMessage.value : '',
)

const createDeploymentWarningItems = computed(() => {
  if (
    !portAvailabilityVisible.value ||
    portAvailabilityState.value === 'available' ||
    portAvailabilityState.value === 'unavailable'
  ) {
    return deploymentWarningItems.value
  }

  const networkWarningValue =
    portAvailabilityState.value === 'checking'
      ? 'Checking live availability for this IP and port pair.'
      : portAvailabilityMessage.value

  return [
    ...deploymentWarningItems.value.filter((item) => item.label !== 'Network'),
    {
      label: 'Network',
      value: networkWarningValue,
      icon: 'lan',
    },
  ]
})

onMounted(async () => {
  await initialize()
})

async function cancel() {
  router.back()
}

async function submitGameServer() {
  const formValid = await validateBeforeSave(
    'Complete the required fields before saving this server.',
  )
  if (!formValid) {
    return
  }

  const portsAvailable = await ensurePortAvailabilityBeforeSave()
  if (!portsAvailable) {
    $q.notify({
      type: 'warning',
      position: 'top',
      caption: portAvailabilityMessage.value,
      icon: 'report_problem',
    })
    return
  }

  startSubmitting()

  try {
    const request: CreateGameServerRequest = create(CreateGameServerRequestSchema, {})
    request.gameServer = gameServer.value

    const response = await GetXylonaClient().createGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Game server created successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to create game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    resetSubmissionState()
  }
}
</script>
