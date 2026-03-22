<template>
  <div v-if="softwareOptions.length > 0">
    <q-card-section>
      <div class="text-h6">Server Software</div>
    </q-card-section>
    <q-card-section>
      <div class="row wrap q-col-gutter-md">
        <div class="col-12 col-md-6" aria-label="Software">
          <q-select
            v-model="selectedSoftwareId"
            outlined
            label="Software"
            emit-value
            map-options
            :display-value="selectedSoftwareName || undefined"
            :options="softwareSelectOptions"
            :loading="loadingOptions"
            :disable="saving"
            options-selected-class="selected-option"
            @update:model-value="onSoftwareChange" />
        </div>
        <q-select
          v-if="selectedSoftwareId !== '' && versions.length > 0"
          v-model="selectedVersionId"
          class="col-12 col-md-6"
          outlined
          label="Version"
          emit-value
          map-options
          :options="versionSelectOptions"
          :loading="loadingVersions"
          :disable="saving || loadingVersions"
          options-selected-class="selected-option" />
      </div>
      <div class="row q-mt-md">
        <q-btn
          label="Apply"
          color="primary"
          :loading="saving"
          :disable="!canApply"
          @click="showConfirmDialog = true" />
      </div>
    </q-card-section>

    <q-dialog
      v-model="showConfirmDialog"
      persistent
      backdrop-filter="brightness(15%)"
      aria-labelledby="software-confirm-title">
      <q-card>
        <q-card-section>
          <div id="software-confirm-title" class="text-h6">Change Server Software</div>
        </q-card-section>
        <q-card-section>
          <p>
            Switching server software may affect installed mods. Mods will be preserved but may not
            be compatible with {{ selectedSoftwareName }}. Continue?
          </p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn label="Cancel" color="neutral" flat @click="showConfirmDialog = false" />
          <q-btn label="Confirm" color="primary" @click="applyServerSoftware" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'

import {
  GetServerSoftwareOptionsRequestSchema,
  GetServerSoftwareVersionsRequestSchema,
  SetServerSoftwareRequestSchema,
} from '@/proto/xylona_pb'
import type { ServerSoftwareOption, SoftwareVersion } from '@/proto/shared_pb'
import { GetXylonaClient } from '@/utils/shared'

interface Props {
  gameServerId: string
  gameId: string
  currentSoftware?: string
}

const props = defineProps<Props>()

const $q = useQuasar()

const softwareOptions = ref<ServerSoftwareOption[]>([])
const versions = ref<SoftwareVersion[]>([])
const selectedSoftwareId = ref('')
const selectedVersionId = ref('')
const loadingOptions = ref(false)
const loadingVersions = ref(false)
const saving = ref(false)
const showConfirmDialog = ref(false)

const softwareSelectOptions = computed(() =>
  softwareOptions.value.map((opt) => ({
    label: opt.name,
    value: opt.id,
  })),
)

const versionSelectOptions = computed(() =>
  versions.value.map((v) => ({
    label: v.versionString || v.versionId,
    value: v.versionId,
  })),
)

const selectedSoftwareName = computed(() => {
  const found = softwareOptions.value.find((opt) => opt.id === selectedSoftwareId.value)
  return found?.name ?? ''
})

const selectedSoftwareHasJarSource = computed(() => {
  const found = softwareOptions.value.find((opt) => opt.id === selectedSoftwareId.value)
  return (found?.jarSource ?? '') !== ''
})

const canApply = computed(() => {
  if (selectedSoftwareId.value === '' || saving.value || loadingVersions.value) return false
  if (versions.value.length > 0) return selectedVersionId.value !== ''
  return true
})

onMounted(async () => {
  await fetchSoftwareOptions()
})

async function fetchSoftwareOptions(): Promise<void> {
  loadingOptions.value = true
  try {
    const response = await GetXylonaClient().getServerSoftwareOptions(
      create(GetServerSoftwareOptionsRequestSchema, {
        gameId: props.gameId,
      }),
    )
    softwareOptions.value = response.options

    // Pre-select the current software if set on the game server.
    if (props.currentSoftware) {
      const match = softwareOptions.value.find((opt) => opt.id === props.currentSoftware)
      if (match) {
        selectedSoftwareId.value = match.id
        await fetchVersions()
      }
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    console.error('Failed to fetch server software options:', err)
  } finally {
    loadingOptions.value = false
  }
}

async function fetchVersions(): Promise<void> {
  if (selectedSoftwareId.value === '') {
    versions.value = []
    return
  }
  if (!selectedSoftwareHasJarSource.value) {
    versions.value = []
    return
  }
  loadingVersions.value = true
  try {
    const response = await GetXylonaClient().getServerSoftwareVersions(
      create(GetServerSoftwareVersionsRequestSchema, {
        gameId: props.gameId,
        softwareId: selectedSoftwareId.value,
      }),
    )
    versions.value = response.versions
    if (versions.value.length > 0) {
      selectedVersionId.value = versions.value[0].versionId
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    console.error('Failed to fetch software versions:', err)
    $q.notify({
      caption: `Failed to fetch versions: ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loadingVersions.value = false
  }
}

async function onSoftwareChange(): Promise<void> {
  selectedVersionId.value = ''
  versions.value = []
  await fetchVersions()
}

async function applyServerSoftware(): Promise<void> {
  showConfirmDialog.value = false
  saving.value = true
  try {
    await GetXylonaClient().setServerSoftware(
      create(SetServerSoftwareRequestSchema, {
        gameServerId: props.gameServerId,
        softwareId: selectedSoftwareId.value,
        versionId: selectedVersionId.value,
      }),
    )
    $q.notify({
      caption: `Server software changed to ${selectedSoftwareName.value}`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    $q.notify({
      caption: `Failed to change server software: ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped></style>
