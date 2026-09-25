<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import {
  ClearJoinPasswordRequestSchema,
  GetJoinPasswordStateRequestSchema,
  SetJoinPasswordRequestSchema,
  type JoinPasswordState,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ serverId: string; canEdit: boolean }>()
const $q = useQuasar()
const state = ref<JoinPasswordState>()
const password = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const result = ref('')
// A typed password is a pending change; the settings form's Save commits it.
const dirty = computed(() => password.value.length > 0)
watch(
  () => props.serverId,
  () => {
    state.value = undefined
    password.value = ''
    void load()
  },
  { immediate: true },
)
async function load() {
  loading.value = true
  error.value = ''
  try {
    state.value = (
      await GetXylonaClient().getJoinPasswordState(
        create(GetJoinPasswordStateRequestSchema, { serverId: props.serverId }),
      )
    ).state
  } catch (err) {
    error.value = ConnectErrorToString(ConnectError.from(err))
  } finally {
    loading.value = false
  }
}
async function save(): Promise<void> {
  if (!dirty.value || !props.canEdit) return
  saving.value = true
  error.value = ''
  try {
    const response = await GetXylonaClient().setJoinPassword(
      create(SetJoinPasswordRequestSchema, {
        serverId: props.serverId,
        password: password.value,
      }),
    )
    state.value = response.state
    password.value = ''
    result.value = 'Join password configured. Applies on next start.'
  } catch (err) {
    error.value = ConnectErrorToString(ConnectError.from(err))
    throw err
  } finally {
    saving.value = false
  }
}
function confirmClear() {
  $q.dialog({
    title: 'Disable password protection?',
    message: 'Players can join without a password after the next start.',
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Disable protection' },
    persistent: true,
  }).onOk(() => void clear())
}
async function clear() {
  if (saving.value || !props.canEdit) return
  saving.value = true
  error.value = ''
  try {
    const response = await GetXylonaClient().clearJoinPassword(
      create(ClearJoinPasswordRequestSchema, { serverId: props.serverId }),
    )
    state.value = response.state
    password.value = ''
    result.value = 'Password protection disabled. Applies on next start.'
  } catch (err) {
    error.value = ConnectErrorToString(ConnectError.from(err))
  } finally {
    saving.value = false
  }
}

defineExpose({ dirty, save })
</script>

<template>
  <section class="form-section join-password" aria-label="Join password" :aria-busy="loading">
    <div class="section-header">
      <span class="section-icon">
        <q-icon name="lock" size="14px" />
      </span>
      <h3 class="section-title">Join Password</h3>
      <q-badge
        v-if="state?.supported"
        :color="state.configured ? 'positive' : 'grey-8'"
        :label="state.configured ? 'Protected' : 'Open'"
        data-testid="join-password-state" />
      <span class="section-line"></span>
    </div>
    <p class="join-password__copy">
      Optionally require a password for players to join. Changes apply on next start.
    </p>
    <p v-if="loading" class="join-password__copy" role="status">Loading password state…</p>
    <p v-if="state && !state.supported" class="join-password__copy">
      Managed join passwords are not supported for this server.
    </p>
    <ul v-if="state?.validationIssues.length" class="join-password__copy">
      <li v-for="issue in state.validationIssues" :key="issue">{{ issue }}</li>
    </ul>
    <q-banner v-if="error" class="xy-banner-negative q-mb-md" dense role="alert" rounded>
      {{ error }}
      <template #action>
        <q-btn flat label="Retry state" :disable="loading || saving" @click="load" />
      </template>
    </q-banner>
    <p v-if="result" class="join-password__copy" role="status">{{ result }}</p>
    <div v-if="canEdit && state?.supported" class="join-password__editor">
      <q-input
        v-model="password"
        outlined
        type="password"
        autocomplete="new-password"
        label="New join password"
        :disable="saving"
        hint="At least 5 characters; must not appear in the server name. Save changes applies it." />
      <div>
        <q-btn
          v-if="state.configured"
          flat
          color="negative"
          label="Disable password protection"
          no-caps
          :disable="loading || saving"
          @click="confirmClear" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.join-password {
  min-width: 0;
  overflow-wrap: anywhere;
}
.join-password__copy {
  margin: 0 0 var(--xy-space-md);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: var(--xy-line-height-base);
}
.join-password__editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--xy-space-md);
  align-items: start;
}
@media (max-width: 599px) {
  .join-password__editor {
    grid-template-columns: 1fr;
  }
}
</style>
