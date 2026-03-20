<template>
  <div class="xy-page-content">
    <div v-if="loading" class="schema-loading">
      <q-spinner-dots size="40px" color="primary" />
    </div>

    <config-schema-editor
      v-else
      :file-path="filePath"
      :schema="schema"
      @save="handleSave"
      @back="router.back()" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { GetXylonaClient, ConnectErrorToString } from '@/utils/shared'
import {
  GetGameConfigSchemasRequestSchema,
  UpdateGameConfigSchemasRequestSchema,
} from '@/proto/xylona_pb'
import ConfigSchemaEditor from '@/components/games/ConfigSchemaEditor.vue'

interface SchemaProperty {
  type?: string
  title?: string
  description?: string
  default?: unknown
  enum?: string[]
  minimum?: number
  maximum?: number
  maxLength?: number
  'x-managed'?: { source: string }
  'x-allow-multiple'?: boolean
  [key: string]: unknown
}

interface JsonSchema {
  type: string
  properties: Record<string, SchemaProperty>
  required?: string[]
}

interface ConfigSchemaEntry {
  path: string
  format: string
  category: string
  generate_before_start: boolean
  schema?: JsonSchema
  [key: string]: unknown
}

const $q = useQuasar()
const route = useRoute()
const router = useRouter()
const gameId = route.params.id as string
const fileIndex = Number(route.params.fileIndex)

const loading = ref(true)
const filePath = ref('')
const schema = ref<JsonSchema>({ type: 'object', properties: {} })
const allSchemas = ref<ConfigSchemaEntry[]>([])

onMounted(async () => {
  await loadSchema()
})

async function loadSchema() {
  loading.value = true
  try {
    const request = create(GetGameConfigSchemasRequestSchema, { gameId })
    const response = await GetXylonaClient().getGameConfigSchemas(request)

    if (response.configSchemasJson) {
      allSchemas.value = JSON.parse(response.configSchemasJson) as ConfigSchemaEntry[]
      const entry = allSchemas.value[fileIndex]
      if (entry) {
        filePath.value = entry.path
        schema.value = entry.schema || { type: 'object', properties: {} }
      }
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loading.value = false
  }
}

async function handleSave(updatedSchema: JsonSchema) {
  const entry = allSchemas.value[fileIndex]
  if (!entry) {
    $q.notify({
      type: 'xylona-error',
      caption: 'Schema entry not found. Save the game first, then try again.',
      position: 'top',
      timeout: 5000,
    })
    return
  }

  try {
    // Update this file's schema in the array
    entry.schema = updatedSchema

    const request = create(UpdateGameConfigSchemasRequestSchema, {
      gameId,
      configSchemasJson: JSON.stringify(allSchemas.value),
    })
    const response = await GetXylonaClient().updateGameConfigSchemas(request)

    if (response.success) {
      $q.notify({
        type: 'xylona-success',
        caption: 'Schema saved successfully',
        position: 'top',
        timeout: 3000,
      })
    } else if (response.validationErrors.length > 0) {
      $q.notify({
        type: 'xylona-error',
        caption: response.validationErrors.join(', '),
        position: 'top',
        timeout: 5000,
      })
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  }
}
</script>

<style scoped>
.schema-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 300px;
}
</style>
