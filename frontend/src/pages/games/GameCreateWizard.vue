<script lang="ts" setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import SteamAppSearch from '@/components/games/SteamAppSearch.vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { GetSteamAppDetailsRequestSchema } from '@/proto/xylona_pb'
import type { SteamAppDetails } from '@/proto/shared_pb'

const router = useRouter()

// Wizard state
type WizardStep = 'select' | 'search' | 'preview'
const step = ref<WizardStep>('select')
const stepNumber = computed(() => ['select', 'search', 'preview'].indexOf(step.value) + 1)
const wizardTitle = ref<HTMLElement | null>(null)
const selectedApp = ref<{ appId: string; name: string } | null>(null)
const details = ref<SteamAppDetails | null>(null)
const detailsLoading = ref(false)
const detailsError = ref('')

// Changing step unmounts the focused control, so focus falls to <body>. Move it to the new
// step's heading once the out-in transition has inserted it (the first step has only the h1).
function focusStepHeading(el: Element): void {
  const heading = el.querySelector<HTMLElement>('h2') ?? wizardTitle.value
  heading?.focus()
}

// Helpers
function stripDedicatedServer(name: string): string {
  return name.replace(/\s*dedicated\s*server\s*/i, '').trim()
}

function toSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}

// Actions
function selectCustom(): void {
  void router.push('/games/create')
}

function selectSteamCMD(): void {
  step.value = 'search'
}

function goBackToSelect(): void {
  step.value = 'select'
  selectedApp.value = null
  details.value = null
  detailsError.value = ''
}

function goBackToSearch(): void {
  step.value = 'search'
  details.value = null
  detailsError.value = ''
}

async function onAppSelected(app: { appId: string; name: string }): Promise<void> {
  selectedApp.value = app
  step.value = 'preview'
  detailsLoading.value = true
  detailsError.value = ''
  details.value = null

  try {
    const client = GetXylonaClient()
    const req = create(GetSteamAppDetailsRequestSchema, { appId: app.appId })
    const response = await client.getSteamAppDetails(req)
    if (response.detailsAvailable && response.details) {
      details.value = response.details
    } else {
      detailsError.value =
        'Details not available for this app. You can still continue with basic info.'
    }
  } catch (err: unknown) {
    console.error('Failed to fetch Steam app details:', err)
    detailsError.value = 'Failed to fetch details: ' + ConnectErrorToString(ConnectError.from(err))
  } finally {
    detailsLoading.value = false
  }
}

function launchConfigForPlatform(platform: 'windows' | 'linux') {
  if (!details.value?.launchConfigs?.length) {
    return null
  }

  const exactMatch = details.value.launchConfigs.find(
    (launchConfig) => launchConfig.os.toLowerCase() === platform,
  )
  return exactMatch ?? details.value.launchConfigs[0]
}

function continueToForm(): void {
  const appName = details.value
    ? stripDedicatedServer(details.value.name)
    : selectedApp.value?.name
      ? stripDedicatedServer(selectedApp.value.name)
      : ''
  const slug = toSlug(appName)

  const wizardState = {
    name: appName,
    slug: slug,
    steamAppId: selectedApp.value?.appId || '',
    usesSteamcmd: true,
    windowsSupport: details.value?.windowsSupport ?? false,
    linuxSupport: details.value?.linuxSupport ?? false,
    linuxBaseCommand: launchConfigForPlatform('linux')?.executable || '',
    windowsBaseCommand: launchConfigForPlatform('windows')?.executable || '',
    linuxStartArgsTemplate: '',
    windowsStartArgsTemplate: '',
  }

  void router.push({
    path: '/games/create',
    state: { wizardState },
  })
}

// Preview computed helpers
function previewName(): string {
  if (details.value) {
    return stripDedicatedServer(details.value.name)
  }
  return selectedApp.value?.name ? stripDedicatedServer(selectedApp.value.name) : ''
}

function platformText(): string {
  if (!details.value) return 'Unknown'
  const platforms: string[] = []
  if (details.value.windowsSupport) platforms.push('Windows')
  if (details.value.linuxSupport) platforms.push('Linux')
  return platforms.length > 0 ? platforms.join(', ') : 'None detected'
}
</script>

<template>
  <q-page class="wizard-page">
    <div class="wizard-container">
      <h1 ref="wizardTitle" aria-describedby="wizard-progress" class="wizard-title" tabindex="-1">
        Add a Game
      </h1>
      <p id="wizard-progress" class="wizard-progress">Step {{ stepNumber }} of 3</p>

      <!-- Step: Select type -->
      <transition mode="out-in" name="wizard-fade" @after-enter="focusStepHeading">
        <div v-if="step === 'select'" key="select" class="wizard-step">
          <p class="wizard-subtitle">Choose how you want to set up your game server.</p>

          <div class="selection-cards">
            <button
              aria-label="SteamCMD setup"
              class="selection-card"
              type="button"
              @click="selectSteamCMD">
              <div class="selection-card__icon">
                <q-icon name="cloud_download" size="3rem" />
              </div>
              <div class="selection-card__title">SteamCMD</div>
              <div class="selection-card__description">
                Game server installed and updated via SteamCMD
              </div>
            </button>

            <button
              aria-label="Custom setup"
              class="selection-card"
              type="button"
              @click="selectCustom">
              <div class="selection-card__icon">
                <q-icon name="settings" size="3rem" />
              </div>
              <div class="selection-card__title">Custom</div>
              <div class="selection-card__description">Manual setup for any game server</div>
            </button>
          </div>
        </div>

        <!-- Step: SteamCMD Search -->
        <div v-else-if="step === 'search'" key="search" class="wizard-step">
          <div class="wizard-step-header">
            <q-btn
              aria-label="Back to selection"
              class="text-xy-muted"
              flat
              icon="arrow_back"
              round
              @click="goBackToSelect" />
            <h2 aria-describedby="wizard-progress" class="wizard-step-title" tabindex="-1">
              Look Up Steam AppID
            </h2>
          </div>
          <p class="wizard-subtitle">
            Look up the game's dedicated server on Steam to fill in its details.
          </p>

          <div class="search-wrapper">
            <steam-app-search @select="onAppSelected" />
          </div>
        </div>

        <!-- Step: Preview -->
        <div v-else-if="step === 'preview'" key="preview" class="wizard-step">
          <div class="wizard-step-header">
            <q-btn
              aria-label="Back to search"
              class="text-xy-muted"
              flat
              icon="arrow_back"
              round
              @click="goBackToSearch" />
            <h2 aria-describedby="wizard-progress" class="wizard-step-title" tabindex="-1">
              Review Details
            </h2>
          </div>

          <!-- Loading state -->
          <div v-if="detailsLoading" class="preview-loading">
            <q-spinner-orbit color="accent" size="3rem" />
            <p class="text-xy-secondary q-mt-md">Fetching app details...</p>
          </div>

          <!-- Error state (non-blocking) -->
          <q-banner v-if="detailsError && !detailsLoading" class="q-mb-md xy-banner-warning" dense>
            <template #avatar>
              <q-icon name="warning_amber" size="sm" />
            </template>
            {{ detailsError }}
          </q-banner>

          <!-- Preview content -->
          <div v-if="!detailsLoading" class="preview-content">
            <div class="preview-card">
              <div class="preview-card__header">
                <q-icon class="text-accent" name="cloud_download" size="1.5rem" />
                <span class="preview-card__app-id">App ID: {{ selectedApp?.appId }}</span>
              </div>

              <div class="preview-card__field">
                <span class="preview-card__label">Game Name</span>
                <span class="preview-card__value">{{ previewName() || 'N/A' }}</span>
              </div>

              <div class="preview-card__field">
                <span class="preview-card__label">Platforms</span>
                <span class="preview-card__value">
                  <template v-if="details">
                    <q-icon
                      v-if="details.windowsSupport"
                      class="q-mr-xs"
                      name="desktop_windows"
                      size="1.2rem" />
                    <q-icon
                      v-if="details.linuxSupport"
                      class="q-mr-xs"
                      name="terminal"
                      size="1.2rem" />
                    {{ platformText() }}
                  </template>
                  <template v-else>Unknown</template>
                </span>
              </div>

              <div v-if="details?.installDirectory" class="preview-card__field">
                <span class="preview-card__label">Install Directory</span>
                <span class="preview-card__value mono">{{ details.installDirectory }}</span>
              </div>

              <div
                v-if="details?.launchConfigs && details.launchConfigs.length > 0"
                class="preview-card__field">
                <span class="preview-card__label">Executable</span>
                <span class="preview-card__value mono">{{
                  details.launchConfigs[0].executable
                }}</span>
              </div>
            </div>

            <div class="preview-info">
              <q-icon class="text-accent q-mr-xs" name="info_outline" size="1rem" />
              <span class="text-xy-secondary text-caption">
                These details will be used to pre-fill the game creation form. You can edit
                everything on the next page.
              </span>
            </div>

            <div class="preview-actions">
              <q-btn
                color="primary"
                icon-right="arrow_forward"
                label="Continue"
                no-caps
                unelevated
                @click="continueToForm" />
            </div>
          </div>
        </div>
      </transition>
    </div>
  </q-page>
</template>

<style lang="scss" scoped>
.wizard-page {
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: var(--xy-space-3xl) var(--xy-space-md);
}

.wizard-container {
  width: 100%;
  max-width: 700px;
}

.wizard-step {
  width: 100%;
}

.wizard-title {
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-2xl);
  font-weight: 700;
  color: var(--xy-text-primary);
  margin: 0 0 var(--xy-space-xs) 0;
  text-align: center;
}

/* Programmatic focus targets after a step change, like #main-content. */
.wizard-title:focus,
.wizard-step-title:focus {
  outline: none;
}

.wizard-progress {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  text-align: center;
  margin: 0 0 var(--xy-space-lg) 0;
}

.wizard-subtitle {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  text-align: center;
  margin: 0 0 var(--xy-space-xl) 0;
}

.wizard-step-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-xs);
}

.wizard-step-title {
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-xl);
  font-weight: 700;
  color: var(--xy-text-primary);
  margin: 0;
}

/* Selection cards */
.selection-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--xy-space-md);
}

@media (max-width: 599px) {
  .selection-cards {
    grid-template-columns: 1fr;
  }
}

.selection-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-base),
    box-shadow var(--xy-transition-base),
    transform var(--xy-transition-fast);
  text-align: center;
  color: inherit;
  font-family: inherit;
  font-size: inherit;

  &:hover {
    border-color: var(--xy-accent);
    box-shadow: 0 0 16px var(--xy-accent-muted);
    transform: translateY(-2px);
  }

  &:focus-visible {
    outline: 2px solid var(--xy-accent);
    outline-offset: 2px;
  }

  &__icon {
    color: var(--xy-accent);
  }

  &__title {
    font-family: var(--xy-font-display);
    font-size: var(--xy-font-size-lg);
    font-weight: 700;
    color: var(--xy-text-primary);
  }

  &__description {
    font-size: var(--xy-font-size-sm);
    color: var(--xy-text-secondary);
    line-height: 1.4;
  }
}

/* Search */
.search-wrapper {
  margin-top: var(--xy-space-lg);
}

/* Preview */
.preview-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--xy-space-3xl) 0;
}

.preview-content {
  margin-top: var(--xy-space-md);
}

.preview-card {
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  padding: var(--xy-space-lg);

  &__header {
    display: flex;
    align-items: center;
    gap: var(--xy-space-sm);
    padding-bottom: var(--xy-space-md);
    margin-bottom: var(--xy-space-md);
    border-bottom: 1px solid var(--xy-border);
  }

  &__app-id {
    font-family: var(--xy-font-mono);
    font-size: var(--xy-font-size-sm);
    color: var(--xy-text-secondary);
  }

  &__field {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--xy-space-md);
    padding: var(--xy-space-sm) 0;

    & + & {
      border-top: 1px solid var(--xy-border);
    }
  }

  &__label {
    font-size: var(--xy-font-size-sm);
    color: var(--xy-text-secondary);
  }

  &__value {
    font-size: var(--xy-font-size-sm);
    color: var(--xy-text-primary);
    display: flex;
    align-items: center;
    min-width: 0;
    overflow-wrap: anywhere;

    &.mono {
      font-family: var(--xy-font-mono);
      font-size: var(--xy-font-size-sm);
    }
  }
}

.preview-info {
  display: flex;
  align-items: flex-start;
  margin-top: var(--xy-space-md);
  padding: var(--xy-space-sm);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-md);
}

.preview-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--xy-space-lg);
}

/* Transitions */
.wizard-fade-enter-active,
.wizard-fade-leave-active {
  transition: opacity var(--xy-transition-base);
}

.wizard-fade-enter-from,
.wizard-fade-leave-to {
  opacity: 0;
}
</style>
