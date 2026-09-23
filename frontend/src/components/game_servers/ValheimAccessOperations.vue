<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import {
  ExecuteGameServerOperationRequestSchema,
  GameOperationValueSchema,
  GameOperationResultClassification,
  type GameOperationDescriptor,
  type GameOperationResult,
  type ValheimAccessList,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ serverId: string; operations: GameOperationDescriptor[] }>()
const emit = defineEmits<{ refresh: [] }>()
const $q = useQuasar()
const kind = ref('administrators')
const player = ref('')
const search = ref('')
const snapshot = ref<ValheimAccessList>()
const result = ref<GameOperationResult>()
const error = ref('')
const busy = ref(false)
const lists = [
  { label: 'Administrators', value: 'administrators' },
  { label: 'Bans', value: 'bans' },
  { label: 'Permitted identities', value: 'permitted' },
]
const actions = computed(() =>
  props.operations.filter(
    (operation) =>
      operation.id.startsWith(`valheim.access.${kind.value}.`) && !operation.id.endsWith('.list'),
  ),
)
const listOperation = computed(() =>
  props.operations.find((operation) => operation.id === `valheim.access.${kind.value}.list`),
)
const identities = computed(
  () =>
    snapshot.value?.identities.filter((identity) =>
      identity.toLocaleLowerCase().includes(search.value.toLocaleLowerCase()),
    ) ?? [],
)
const classification = computed(() =>
  result.value?.classification === GameOperationResultClassification.CONFIRMED
    ? 'Confirmed: stored file read back'
    : result.value?.classification === GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED
      ? 'Accepted but unverified: refresh before another change'
      : 'Failed',
)
const resultBannerClass = computed(() => {
  switch (result.value?.classification) {
    case GameOperationResultClassification.CONFIRMED:
      return 'xy-banner-info'
    case GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED:
      return 'xy-banner-warning'
    default:
      return 'xy-banner-negative'
  }
})
const actionUnavailableReasons = computed(() => [
  ...new Set(
    actions.value
      .filter((action) => !action.available && action.availabilityReasonText !== '')
      .map((action) => action.availabilityReasonText),
  ),
])
watch(
  kind,
  () => {
    snapshot.value = undefined
    result.value = undefined
    player.value = ''
    search.value = ''
    void refresh()
  },
  { immediate: true },
)
async function refresh() {
  if (!listOperation.value || busy.value) return
  await execute(listOperation.value)
}
async function refreshStoredList() {
  await refresh()
  emit('refresh')
}
// Banning is the harshest change and removals take access away; the rest grant or restore it.
function actionColor(operation: GameOperationDescriptor) {
  if (operation.id === 'valheim.access.bans.add') return 'negative'
  if (operation.id.endsWith('.remove') && operation.id !== 'valheim.access.bans.remove') {
    return 'warning'
  }
  return 'primary'
}
function confirmChange(operation: GameOperationDescriptor) {
  const values = { player: player.value, expected_revision: snapshot.value?.revision ?? '' }
  const review = operation.review
  $q.dialog({
    title: review?.title || operation.name,
    message: [review?.effect || operation.summary, `Identity: ${values.player}.`, review?.caution]
      .filter(Boolean)
      .join(' '),
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: actionColor(operation), label: operation.name, unelevated: true },
  }).onOk(() => void execute(operation, values))
}
async function execute(operation: GameOperationDescriptor, values: Record<string, string> = {}) {
  if (busy.value || !operation.available) return
  busy.value = true
  error.value = ''
  try {
    const response = await GetXylonaClient().executeGameServerOperation(
      create(ExecuteGameServerOperationRequestSchema, {
        gameServerId: props.serverId,
        operationId: operation.id,
        values: operation.fields.map((field) =>
          create(GameOperationValueSchema, {
            fieldId: field.id,
            value: {
              case: 'stringValue',
              value: values[field.id] ?? '',
            },
          }),
        ),
      }),
    )
    result.value = response.result
    if (response.result?.valheimAccessList) snapshot.value = response.result.valheimAccessList
    else if (!operation.id.endsWith('.list')) snapshot.value = undefined
    if (!response.result)
      error.value = 'No operation result was returned. Refresh the stored list before retrying.'
  } catch (err) {
    error.value = ConnectErrorToString(ConnectError.from(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="valheim-access" aria-label="Stored Valheim access" :aria-busy="busy">
    <p>
      Inspect stored identities and stage changes for the next start. Stop the server before making
      changes. In-game enforcement is not verified.
    </p>
    <q-select
      v-model="kind"
      :options="lists"
      emit-value
      map-options
      outlined
      label="Stored access list"
      :disable="busy" />
    <div class="row items-center q-gutter-sm">
      <q-btn
        outline
        icon="refresh"
        label="Refresh stored list"
        :loading="busy"
        :disable="!listOperation?.available"
        @click="refreshStoredList" /><span v-if="listOperation && !listOperation.available">{{
        listOperation.availabilityReasonText
      }}</span>
    </div>
    <p v-if="!listOperation">This list is unavailable for your permissions or node capabilities.</p>
    <p v-if="error" class="valheim-access__banner xy-banner-negative" role="alert">{{ error }}</p>
    <div v-if="result" class="valheim-access__banner" :class="resultBannerClass" role="status">
      <strong>{{ classification }}</strong>
      <p>{{ result.message }}</p>
    </div>
    <template v-if="snapshot">
      <p v-if="snapshot.missing" class="valheim-access__muted">No stored file exists yet.</p>
      <p v-else-if="snapshot.identities.length === 0" class="valheim-access__muted">
        The stored list contains no recognized identities. Empty permitted-list behavior is not
        verified.
      </p>
      <q-input
        v-model="search"
        outlined
        label="Search stored identities"
        clearable
        @clear="search = ''" />
      <p class="valheim-access__muted">
        {{ identities.length }} of {{ snapshot.identities.length }} stored identities
      </p>
      <ul class="valheim-access__identities">
        <li v-for="identity in identities" :key="identity">
          <code>{{ identity }}</code
          ><q-btn
            flat
            dense
            label="Use identity"
            :aria-label="`Use identity ${identity}`"
            :disable="busy"
            @click="player = identity" />
        </li>
      </ul>
      <div v-if="snapshot.diagnostics.length">
        <strong>Stored file diagnostics</strong>
        <ul>
          <li v-for="(diagnostic, index) in snapshot.diagnostics" :key="index">{{ diagnostic }}</li>
        </ul>
      </div>
    </template>
    <template v-if="actions.length">
      <q-input
        v-model="player"
        outlined
        label="Exact platform identity"
        hint="Use the case-sensitive Steam_ followed by 17 digits. Display names are not account identities."
        :disable="busy" />
      <div class="valheim-access__actions">
        <q-btn
          v-for="action in actions"
          :key="action.id"
          :color="actionColor(action)"
          :label="action.name"
          no-caps
          unelevated
          :disable="
            busy || !action.available || !snapshot?.revision || !/^Steam_[0-9]{17}$/.test(player)
          "
          @click="confirmChange(action)" />
      </div>
      <p v-for="reason in actionUnavailableReasons" :key="reason" class="valheim-access__muted">
        {{ reason }}
      </p>
    </template>
  </section>
</template>

<style scoped>
.valheim-access {
  display: grid;
  gap: var(--xy-space-md);
  min-width: 0;
  padding: var(--xy-space-md);
  overflow-wrap: anywhere;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}
.valheim-access p {
  margin: 0;
}
.valheim-access__muted {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}
.valheim-access__banner {
  padding: var(--xy-space-sm) var(--xy-space-base);
}
.valheim-access__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}
.valheim-access__identities {
  max-height: 20rem;
  overflow: auto;
  padding-left: var(--xy-space-lg);
}
.valheim-access__identities li {
  padding-block: var(--xy-space-xs);
}
</style>
