<script setup lang="ts">
import { ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import {
  ClearJoinPasswordRequestSchema,
  GetJoinPasswordStateRequestSchema,
  SetJoinPasswordRequestSchema,
  type JoinPasswordState,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ serverId: string; canEdit: boolean }>()
const state = ref<JoinPasswordState>()
const password = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const result = ref('')
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
async function save(clearing: boolean) {
  if (saving.value || !props.canEdit) return
  saving.value = true
  error.value = ''
  try {
    const client = GetXylonaClient()
    const response = clearing
      ? await client.clearJoinPassword(
          create(ClearJoinPasswordRequestSchema, { serverId: props.serverId }),
        )
      : await client.setJoinPassword(
          create(SetJoinPasswordRequestSchema, {
            serverId: props.serverId,
            password: password.value,
          }),
        )
    state.value = response.state
    password.value = ''
    result.value = clearing
      ? 'Password protection disabled. Applies on next start.'
      : 'Join password configured. Applies on next start.'
  } catch (err) {
    error.value = ConnectErrorToString(ConnectError.from(err))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="join-password" aria-label="Join password" :aria-busy="loading">
    <h3>Join password</h3>
    <p>Optionally require a password for players to join. Changes apply on next start.</p>
    <p v-if="loading" role="status">Loading password state…</p>
    <p v-if="state">
      {{ state.configured ? 'Password protection enabled' : 'Password protection disabled' }}
    </p>
    <p v-if="state && !state.supported">
      Managed join passwords are not supported for this server.
    </p>
    <ul v-if="state?.validationIssues.length">
      <li v-for="issue in state.validationIssues" :key="issue">{{ issue }}</li>
    </ul>
    <div v-if="error" role="alert">
      {{ error }} <q-btn flat label="Retry state" :disable="loading || saving" @click="load" />
    </div>
    <p v-if="result" role="status">{{ result }}</p>
    <div v-if="canEdit && state?.supported" class="join-password__editor">
      <q-input
        v-model="password"
        outlined
        type="password"
        autocomplete="new-password"
        label="New join password"
        :disable="saving"
        hint="At least 5 characters; must not appear in the server name." />
      <div class="row q-gutter-sm">
        <q-btn
          outline
          :label="state.configured ? 'Replace password' : 'Set password'"
          :disable="loading || saving || password.length === 0"
          @click="save(false)" />
        <q-btn
          v-if="state.configured"
          flat
          color="negative"
          label="Disable password protection"
          :disable="loading || saving"
          @click="save(true)" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.join-password {
  min-width: 0;
  overflow-wrap: anywhere;
}
.join-password h3 {
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-lg);
}
.join-password__editor {
  display: grid;
  gap: var(--xy-space-md);
}
</style>
