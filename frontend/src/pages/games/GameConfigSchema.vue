<template>
  <div class="xy-page-content">
    <div v-if="loading" class="schema-loading">
      <q-spinner-dots color="primary" size="40px" />
    </div>

    <q-banner v-else-if="loadError" class="xy-banner-negative" role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      <strong>The config schema could not be loaded.</strong> {{ loadError }}
      <template #action>
        <q-btn dense flat icon="refresh" label="Retry" no-caps @click="loadSchema" />
      </template>
    </q-banner>

    <template v-else>
      <header class="schema-page-header">
        <div class="schema-page-header__main">
          <nav aria-label="Breadcrumb">
            <ol class="schema-breadcrumbs">
              <li><router-link class="schema-breadcrumbs__link" to="/games">Games</router-link></li>
              <li>
                <router-link class="schema-breadcrumbs__link" :to="gameEditPath">{{
                  gameName
                }}</router-link>
              </li>
              <li v-if="entry" aria-current="page" class="schema-breadcrumbs__current font-mono">
                {{ entry.path }}
              </li>
            </ol>
          </nav>
          <h1 class="schema-page-title font-display">Config Schema</h1>
        </div>
        <div v-if="entry" class="schema-page-header__actions">
          <span v-if="isDirty" class="schema-page-header__unsaved">Unsaved changes</span>
          <q-btn
            :loading="saving"
            color="primary"
            data-test="save-schema"
            icon="save"
            label="Save Schema"
            no-caps
            @click="handleSave" />
        </div>
      </header>

      <empty-state
        v-if="!entry"
        :description="`${gameName} has no config file at position ${fileIndex + 1}. It may have been removed.`"
        icon="description"
        title="Config file not found">
        <template #actions>
          <q-btn color="primary" :label="`Back to ${gameName}`" no-caps :to="gameEditPath" />
        </template>
      </empty-state>

      <template v-else>
        <div class="schema-file-behavior">
          <q-toggle
            v-model="generateBeforeStart"
            color="primary"
            data-test="generate-toggle"
            label="Create on first start if missing" />
          <div class="schema-file-behavior__hint">
            When the server starts and this file does not exist, Xylona creates it from the schema
            defaults.
          </div>
        </div>

        <config-schema-editor ref="editorRef" :schema="schema" />
      </template>
    </template>
  </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  GetGameConfigSchemasRequestSchema,
  GetGameRequestSchema,
  UpdateGameConfigSchemasRequestSchema,
} from '@/proto/xylona_pb'
import ConfigSchemaEditor from '@/components/games/ConfigSchemaEditor.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'

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

const route = useRoute()
const gameId = route.params.id as string
const fileIndex = Number(route.params.fileIndex)
const gameEditPath = `/games/${gameId}/edit`

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const gameName = ref(gameId)
const schema = ref<JsonSchema>({ type: 'object', properties: {} })
const generateBeforeStart = ref(false)
const allSchemas = ref<ConfigSchemaEntry[]>([])
const editorRef = ref<InstanceType<typeof ConfigSchemaEditor> | null>(null)

const entry = computed<ConfigSchemaEntry | undefined>(() => allSchemas.value[fileIndex])
const isDirty = computed(
  () =>
    (editorRef.value?.isDirty ?? false) ||
    (entry.value !== undefined && generateBeforeStart.value !== entry.value.generate_before_start),
)

useUnsavedChangesGuard(isDirty)

onMounted(async () => {
  await loadSchema()
})

async function loadSchema() {
  loading.value = true
  loadError.value = ''
  try {
    const client = GetXylonaClient()
    const [gameResponse, schemasResponse] = await Promise.all([
      client.getGame(create(GetGameRequestSchema, { id: gameId })),
      client.getGameConfigSchemas(create(GetGameConfigSchemasRequestSchema, { gameId })),
    ])
    gameName.value = gameResponse.game?.name || gameId
    allSchemas.value = schemasResponse.configSchemasJson
      ? (JSON.parse(schemasResponse.configSchemasJson) as ConfigSchemaEntry[])
      : []
    if (entry.value) {
      schema.value = entry.value.schema || { type: 'object', properties: {} }
      generateBeforeStart.value = entry.value.generate_before_start
    }
  } catch (unknownErr: unknown) {
    loadError.value = ConnectErrorToString(ConnectError.from(unknownErr))
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  const current = entry.value
  const updatedSchema = editorRef.value?.buildSchema()
  if (!current || !updatedSchema) {
    return
  }

  saving.value = true
  try {
    const nextSchemas = allSchemas.value.map((candidate, index) =>
      index === fileIndex
        ? {
            ...candidate,
            schema: updatedSchema,
            generate_before_start: generateBeforeStart.value,
          }
        : candidate,
    )
    const request = create(UpdateGameConfigSchemasRequestSchema, {
      gameId,
      configSchemasJson: JSON.stringify(nextSchemas),
    })
    const response = await GetXylonaClient().updateGameConfigSchemas(request)

    if (response.success) {
      // Only a saved schema becomes the new clean baseline.
      allSchemas.value = nextSchemas
      schema.value = updatedSchema
      notifySuccess('Schema saved successfully')
    } else if (response.validationErrors.length > 0) {
      notifyError(response.validationErrors.join(', '))
    }
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, 'Failed to save schema')
  } finally {
    saving.value = false
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

.schema-page-header {
  position: sticky;
  top: var(--xy-header-stack-height, 50px);
  z-index: 10;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm) var(--xy-space-md);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-md);
}

.schema-page-header__main {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.schema-breadcrumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-xs);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--xy-font-size-xs);
}

.schema-breadcrumbs li + li::before {
  content: '/';
  margin-right: var(--xy-space-xs);
  color: var(--xy-text-muted);
}

.schema-breadcrumbs__link {
  color: var(--xy-text-muted);
  text-decoration: none;
  transition: color var(--xy-transition-fast);
}

.schema-breadcrumbs__link:hover {
  color: var(--xy-accent);
}

.schema-breadcrumbs__current {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.schema-page-title {
  margin: 0;
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  line-height: 1.15;
  letter-spacing: 0.02em;
  color: var(--xy-text-primary);
}

.schema-page-header__actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.schema-page-header__unsaved {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.schema-file-behavior {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0 var(--xy-space-sm);
  margin-bottom: var(--xy-space-sm);
}

.schema-file-behavior__hint {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}
</style>
