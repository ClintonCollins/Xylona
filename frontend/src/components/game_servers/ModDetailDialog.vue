<template>
  <q-dialog
    :model-value="show"
    aria-labelledby="mod-detail-dialog-title"
    maximized
    transition-hide="slide-down"
    transition-show="slide-up"
    @update:model-value="(val: boolean) => emit('update:show', val)">
    <q-card class="mod-detail-dialog">
      <!-- Loading -->
      <div v-if="loading" class="mod-detail-loading">
        <q-spinner color="primary" size="2rem" />
        <span class="text-xy-muted">Loading mod details...</span>
      </div>

      <!-- Error -->
      <div v-else-if="errorMessage" class="mod-detail-error">
        <q-icon aria-hidden="true" color="negative" name="error" size="2rem" />
        <div class="text-xy-secondary">{{ errorMessage }}</div>
        <q-btn color="primary" label="Close" no-caps @click="emit('update:show', false)" />
      </div>

      <!-- Content -->
      <template v-else-if="details">
        <!-- Header -->
        <q-card-section class="mod-detail-header">
          <div class="mod-detail-identity">
            <div class="mod-detail-icon-wrapper">
              <img
                v-if="details.iconUrl"
                :alt="`${details.name} icon`"
                :src="details.iconUrl"
                class="mod-detail-icon-img" />
              <div
                v-else
                :style="{ background: iconGradient(details.name) }"
                aria-hidden="true"
                class="mod-detail-icon-fallback">
                {{ details.name.charAt(0).toUpperCase() }}
              </div>
            </div>

            <div class="mod-detail-meta">
              <div class="mod-detail-title-row">
                <h2 id="mod-detail-dialog-title" class="mod-detail-name">{{ details.name }}</h2>
                <span
                  :style="sourceBadgeStyle(details.source)"
                  :title="sourceDisplayName(details.source)"
                  class="source-badge">
                  {{ sourceLabel(details.source) }}
                </span>
              </div>
              <div v-if="details.author" class="mod-detail-author text-xy-muted">
                by {{ details.author }}
              </div>
              <div class="mod-detail-stats">
                <span class="mod-detail-stat text-xy-muted">
                  <q-icon aria-hidden="true" name="download" size="xs" />
                  {{ formatDownloads(details.downloads) }} downloads
                </span>
                <span v-if="details.license" class="mod-detail-stat text-xy-muted">
                  <q-icon aria-hidden="true" name="gavel" size="xs" />
                  {{ details.license }}
                </span>
                <span v-if="details.updatedAt" class="mod-detail-stat text-xy-muted">
                  <q-icon aria-hidden="true" name="update" size="xs" />
                  Updated {{ formatDate(details.updatedAt) }}
                </span>
              </div>
            </div>
          </div>

          <q-btn
            aria-label="Close"
            dense
            flat
            icon="close"
            round
            @click="emit('update:show', false)" />
        </q-card-section>

        <q-separator />

        <!-- Tabs -->
        <q-tabs
          v-model="activeTab"
          active-color="primary"
          align="left"
          class="mod-detail-tabs"
          dense
          indicator-color="primary"
          narrow-indicator>
          <q-tab label="Description" name="description" no-caps />
          <q-tab label="Versions" name="versions" no-caps />
          <q-tab v-if="details.galleryImages.length > 0" label="Gallery" name="gallery" no-caps />
        </q-tabs>

        <q-separator />

        <!-- Tab panels -->
        <q-tab-panels v-model="activeTab" animated class="mod-detail-panels">
          <!-- Description tab -->
          <q-tab-panel class="mod-detail-description-panel" name="description">
            <!-- Categories -->
            <div v-if="details.categories.length > 0" class="mod-detail-categories">
              <q-badge
                v-for="cat in details.categories"
                :key="cat"
                :label="cat"
                class="bg-xy-surface-3 text-xy-secondary" />
            </div>

            <!-- Dependencies -->
            <div v-if="currentVersionDeps.length > 0" class="mod-detail-deps">
              <div class="mod-detail-section-title text-xy-muted">Dependencies</div>
              <div class="mod-detail-dep-list">
                <span
                  v-for="dep in currentVersionDeps"
                  :key="dep.sourceId"
                  :class="{ 'dep-required': dep.required }"
                  class="mod-detail-dep-chip">
                  {{ dep.name || dep.sourceId }}
                  <span v-if="dep.required" class="dep-required-label">required</span>
                </span>
              </div>
            </div>

            <!-- Body: Modrinth sends Markdown; the others only have a summary. -->
            <!-- eslint-disable-next-line vue/no-v-html -- renderModDescription converts and sanitizes third-party Markdown/HTML before rendering. -->
            <div class="mod-detail-body" v-html="descriptionHtml"></div>
          </q-tab-panel>

          <!-- Versions tab -->
          <q-tab-panel class="mod-detail-versions-panel" name="versions">
            <div
              v-if="versions.length === 0"
              class="text-xy-muted"
              style="text-align: center; padding: 2rem">
              No versions available.
            </div>
            <p v-else-if="gameVersion" class="mod-version-hint text-xy-muted">
              Newest first. Versions marked “Supports {{ gameVersion }}” match this server.
            </p>
            <div
              v-for="ver in versions"
              :key="ver.versionId"
              :class="{ 'mod-version-row--selected': selectedVersionId === ver.versionId }"
              class="mod-version-row">
              <div class="mod-version-info">
                <span class="mod-version-string font-mono">{{ ver.versionString }}</span>
                <span v-if="supportsServer(ver)" class="mod-version-match">
                  <q-icon aria-hidden="true" name="check_circle" size="xs" />
                  Supports {{ gameVersion }}
                </span>
                <span v-if="ver.gameVersions.length > 0" class="mod-version-game text-xy-muted">
                  {{ newestFirst(ver.gameVersions).slice(0, 3).join(', ') }}
                  <template v-if="ver.gameVersions.length > 3">
                    +{{ ver.gameVersions.length - 3 }} more
                  </template>
                </span>
                <span v-if="ver.publishedAt" class="mod-version-date text-xy-muted">
                  {{ formatDate(ver.publishedAt) }}
                </span>
              </div>
              <div class="mod-version-meta">
                <span v-if="ver.fileSize > 0n" class="mod-version-size text-xy-muted font-mono">
                  {{ formatBytes(ver.fileSize) }}
                </span>
                <q-btn
                  :aria-pressed="selectedVersionId === ver.versionId"
                  :flat="selectedVersionId !== ver.versionId"
                  :icon="selectedVersionId === ver.versionId ? 'check' : undefined"
                  :label="selectedVersionId === ver.versionId ? 'Selected' : 'Select'"
                  :outline="selectedVersionId === ver.versionId"
                  color="primary"
                  dense
                  no-caps
                  size="sm"
                  @click="selectedVersionId = ver.versionId" />
              </div>
              <div v-if="ver.changelog" class="mod-version-changelog text-xy-muted">
                {{ ver.changelog }}
              </div>
            </div>
          </q-tab-panel>

          <!-- Gallery tab -->
          <q-tab-panel
            v-if="details.galleryImages.length > 0"
            class="mod-detail-gallery-panel"
            name="gallery">
            <div class="mod-gallery-grid">
              <img
                v-for="(img, idx) in details.galleryImages"
                :key="idx"
                :alt="`${details.name} screenshot ${idx + 1}`"
                :src="img"
                class="mod-gallery-img"
                loading="lazy" />
            </div>
          </q-tab-panel>
        </q-tab-panels>

        <!-- Footer -->
        <q-separator />
        <q-card-actions class="mod-detail-footer">
          <q-select
            v-model="selectedVersionId"
            :options="versionOptions"
            class="mod-detail-version-select"
            dense
            emit-value
            label="Version"
            map-options
            option-label="label"
            option-value="value"
            outlined />

          <q-space />

          <q-btn
            v-if="details.sourceUrl"
            :href="details.sourceUrl"
            color="primary"
            flat
            icon="open_in_new"
            label="View on Source"
            no-caps
            rel="noopener noreferrer"
            target="_blank"
            type="a" />

          <q-btn
            v-if="isInstalled"
            color="positive"
            disable
            icon="check_circle"
            label="Already Installed"
            no-caps />
          <q-btn
            v-else
            :disable="!selectedVersionId"
            :loading="installing"
            color="primary"
            icon="download"
            label="Install"
            no-caps
            @click="handleInstall" />
        </q-card-actions>
      </template>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { Timestamp } from '@bufbuild/protobuf/wkt'
import type { ModDependency, ModDetails, ModVersion } from '@/proto/shared_pb'
import { GetModDetailsRequestSchema, GetModVersionsRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { renderModDescription } from '@/utils/mod-description'
import {
  formatDownloads,
  iconGradient,
  sourceBadgeStyle,
  sourceDisplayName,
  sourceLabel,
} from '@/utils/mod-sources'

interface Props {
  show: boolean
  gameServerId: string
  source: string
  sourceId: string
  isInstalled: boolean
  /** The server's game version, used to pick and mark compatible versions. */
  gameVersion?: string
  installing?: boolean
}

const props = withDefaults(defineProps<Props>(), { gameVersion: '', installing: false })

const emit = defineEmits<{
  'update:show': [value: boolean]
  install: [source: string, sourceId: string, version: ModVersion, name: string]
}>()

const loading = ref(false)
const errorMessage = ref('')
const details = ref<ModDetails | undefined>(undefined)
const versions = ref<ModVersion[]>([])
const selectedVersionId = ref('')
const activeTab = ref('description')

const versionOptions = computed(() =>
  versions.value.map((v) => ({
    label: v.versionString,
    value: v.versionId,
  })),
)

// Providers without a long-form body (Hangar, Thunderstore) still have the
// summary the browse card showed.
const descriptionHtml = computed(() =>
  renderModDescription(details.value?.body || details.value?.description || ''),
)

const currentVersionDeps = computed((): ModDependency[] => {
  if (!selectedVersionId.value) return []
  const ver = versions.value.find((v) => v.versionId === selectedVersionId.value)
  return ver?.dependencies ?? []
})

watch(
  () => props.show,
  (isOpen) => {
    if (isOpen && props.source && props.sourceId) {
      void loadDetails()
    } else if (!isOpen) {
      // Reset state on close
      details.value = undefined
      versions.value = []
      selectedVersionId.value = ''
      activeTab.value = 'description'
      errorMessage.value = ''
    }
  },
)

async function loadDetails(): Promise<void> {
  loading.value = true
  errorMessage.value = ''

  try {
    const client = GetXylonaClient()

    const [detailsResp, versionsResp] = await Promise.all([
      client.getModDetails(
        create(GetModDetailsRequestSchema, {
          gameServerId: props.gameServerId,
          source: props.source,
          sourceId: props.sourceId,
        }),
      ),
      client.getModVersions(
        create(GetModVersionsRequestSchema, {
          gameServerId: props.gameServerId,
          source: props.source,
          sourceId: props.sourceId,
        }),
      ),
    ])

    details.value = detailsResp.details
    versions.value = versionsResp.versions

    // Versions arrive newest first: prefer the newest one this server supports.
    const preferred = versions.value.find(supportsServer) ?? versions.value[0]
    if (preferred) {
      selectedVersionId.value = preferred.versionId
    }
  } catch (err: unknown) {
    if (err instanceof ConnectError) {
      errorMessage.value = ConnectErrorToString(err)
    } else {
      errorMessage.value = 'Failed to load mod details.'
    }
  } finally {
    loading.value = false
  }
}

function handleInstall(): void {
  const version = versions.value.find((v) => v.versionId === selectedVersionId.value)
  if (!version) return
  emit('install', props.source, props.sourceId, version, details.value?.name || props.sourceId)
}

function supportsServer(version: ModVersion): boolean {
  return props.gameVersion !== '' && version.gameVersions.includes(props.gameVersion)
}

function newestFirst(gameVersions: string[]): string[] {
  return [...gameVersions].sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))
}

// --- Display helpers ---

function formatBytes(bytes: bigint): string {
  const num = Number(bytes)
  const sizes = ['B', 'KB', 'MB', 'GB']
  if (num === 0) return '0 B'
  const i = Math.floor(Math.log(num) / Math.log(1024))
  return `${(num / Math.pow(1024, i)).toFixed(1)} ${sizes[i]}`
}

function formatDate(ts: Timestamp | undefined): string {
  if (!ts) return ''
  const date = new Date(Number(ts.seconds) * 1000)
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}
</script>

<style scoped>
.mod-detail-dialog {
  display: flex;
  flex-direction: column;
  background-color: var(--xy-surface-0);
  max-width: 900px;
  margin: 0 auto;
  height: 100%;
}

/* ---- Loading / Error ---- */
.mod-detail-loading,
.mod-detail-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-2xl);
  flex: 1;
}

/* ---- Header ---- */
.mod-detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md) var(--xy-space-lg);
}

.mod-detail-identity {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-md);
  flex: 1;
  min-width: 0;
}

.mod-detail-icon-wrapper {
  width: 64px;
  height: 64px;
  border-radius: var(--xy-radius-lg);
  overflow: hidden;
  flex-shrink: 0;
}

.mod-detail-icon-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mod-detail-icon-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--xy-font-size-xl);
  font-weight: 700;
  color: var(--xy-text-on-color);
}

.mod-detail-meta {
  flex: 1;
  min-width: 0;
}

.mod-detail-title-row {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-detail-name {
  font-size: var(--xy-font-size-xl);
  font-weight: 600;
  color: var(--xy-text-primary);
  margin: 0;
}

.mod-detail-author {
  font-size: var(--xy-font-size-sm);
  margin-bottom: var(--xy-space-xs);
}

.mod-detail-stats {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  flex-wrap: wrap;
}

.mod-detail-stat {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: var(--xy-font-size-xs);
}

/* ---- Tabs ---- */
.mod-detail-tabs {
  background-color: var(--xy-surface-1);
}

.mod-detail-panels {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background-color: var(--xy-surface-0);
}

/* ---- Description tab ---- */
.mod-detail-description-panel {
  padding: var(--xy-space-lg);
}

.mod-detail-categories {
  display: flex;
  gap: var(--xy-space-xs);
  flex-wrap: wrap;
  margin-bottom: var(--xy-space-md);
}

.mod-detail-section-title {
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  margin-bottom: var(--xy-space-xs);
}

.mod-detail-deps {
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background-color: var(--xy-surface-1);
  border-radius: var(--xy-radius-md);
}

.mod-detail-dep-list {
  display: flex;
  gap: var(--xy-space-xs);
  flex-wrap: wrap;
}

.mod-detail-dep-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid transparent;
  border-radius: var(--xy-radius-sm);
  background-color: var(--xy-surface-2);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.dep-required {
  background-color: var(--xy-warning-bg-faint);
  border-color: var(--xy-warning-border);
}

.dep-required-label {
  font-size: var(--xy-font-size-2xs);
  text-transform: uppercase;
  color: var(--xy-warning);
  font-weight: 600;
}

.mod-detail-body {
  font-size: var(--xy-font-size-sm);
  line-height: 1.6;
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.mod-detail-body :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.mod-detail-body :deep(th),
.mod-detail-body :deep(td) {
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  text-align: left;
}

.mod-detail-body :deep(a) {
  color: var(--xy-primary);
}

.mod-detail-body :deep(:is(h1, h2, h3, h4, h5, h6)) {
  color: var(--xy-text-primary);
  margin-top: 1em;
  margin-bottom: 0.5em;
  font-size: var(--xy-font-size-base);
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: 0;
}

/* Quasar's global heading sizes are for page titles, not third-party prose. */
.mod-detail-body :deep(h1) {
  font-size: var(--xy-font-size-xl);
}

.mod-detail-body :deep(h2) {
  font-size: var(--xy-font-size-lg);
}

.mod-detail-body :deep(code) {
  background-color: var(--xy-surface-2);
  padding: 2px 6px;
  border-radius: var(--xy-radius-sm);
  font-size: 0.85em;
}

.mod-detail-body :deep(pre) {
  background-color: var(--xy-surface-2);
  padding: var(--xy-space-md);
  border-radius: var(--xy-radius-md);
  overflow-x: auto;
}

/* ---- Versions tab ---- */
.mod-detail-versions-panel {
  padding: var(--xy-space-sm);
}

.mod-version-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
  transition: background-color var(--xy-transition-fast);
}

.mod-version-row:hover {
  background-color: var(--xy-surface-1);
}

.mod-version-row--selected {
  background-color: color-mix(in srgb, var(--xy-primary) 8%, var(--xy-surface-0));
  outline: 1px solid var(--xy-primary-border-soft);
  outline-offset: -1px;
}

.mod-version-info {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.mod-version-string {
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
  color: var(--xy-text-primary);
}

.mod-version-game {
  font-size: var(--xy-font-size-2xs);
}

.mod-version-hint {
  margin: 0;
  padding: var(--xy-space-xs) var(--xy-space-md) var(--xy-space-sm);
  font-size: var(--xy-font-size-xs);
}

.mod-version-match {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-2xs);
  color: var(--xy-success);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  white-space: nowrap;
}

.mod-version-date {
  font-size: var(--xy-font-size-2xs);
}

.mod-version-meta {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-version-size {
  font-size: var(--xy-font-size-2xs);
}

.mod-version-changelog {
  flex-basis: 100%;
  font-size: var(--xy-font-size-xs);
  padding-left: var(--xy-space-md);
  max-height: 3.6em;
  overflow: hidden;
  line-height: 1.2;
}

/* ---- Gallery tab ---- */
.mod-detail-gallery-panel {
  padding: var(--xy-space-md);
}

.mod-gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--xy-space-md);
}

.mod-gallery-img {
  width: 100%;
  border-radius: var(--xy-radius-md);
  object-fit: cover;
}

/* ---- Footer ---- */
.mod-detail-footer {
  padding: var(--xy-space-sm) var(--xy-space-lg);
  background-color: var(--xy-surface-1);
  flex-shrink: 0;
}

.mod-detail-version-select {
  min-width: 200px;
}

/* ---- Source badge ---- */
.source-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: var(--xy-radius-sm);
  font-size: var(--xy-font-size-2xs);
  font-weight: 700;
  flex-shrink: 0;
}

/* ---- Mobile ---- */
@media (max-width: 599px) {
  .mod-detail-header {
    padding: var(--xy-space-sm) var(--xy-space-md);
  }

  .mod-detail-icon-wrapper {
    width: 48px;
    height: 48px;
  }

  .mod-detail-name {
    font-size: var(--xy-font-size-base);
  }

  .mod-version-info {
    flex-wrap: wrap;
  }

  .mod-detail-footer {
    flex-wrap: wrap;
    gap: var(--xy-space-sm);
  }

  .mod-detail-version-select {
    min-width: 0;
    flex: 1 1 100%;
  }
}
</style>
