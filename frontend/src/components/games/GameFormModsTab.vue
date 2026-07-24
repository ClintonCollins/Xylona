<template>
  <!-- Mods -->
  <section class="form-section form-section--last">
    <div class="section-header">
      <span class="section-bar" style="background-color: var(--xy-info)"></span>
      <h2 class="section-title font-display">Mods</h2>
      <span class="section-line"></span>
    </div>
    <div :class="game.modProfile ? 'mods-layout' : 'mods-layout-single'">
      <!-- Sidebar only visible when mods are enabled -->
      <aside v-if="game.modProfile" class="mods-rail">
        <div class="mods-rail-intro">
          <span class="mods-rail-eyebrow font-display">Setup Guide</span>
          <div class="mods-rail-title font-display">
            Define how servers pull mods beyond the base install.
          </div>
          <div class="mods-rail-copy text-xy-muted">
            Choose one source and one install folder. Paste the provider reference Xylona should
            use, such as a slug, workshop ID, or provider JSON payload.
          </div>
        </div>
      </aside>

      <div class="mods-workspace">
        <div v-if="managedModConfig" class="typed-config-managed mods-workspace-card text-xy-muted">
          This game uses advanced mod configuration outside the simple editor. Hidden internal mod
          settings will be preserved when you save this game.
        </div>

        <template v-else>
          <div
            v-if="variantModSummaries.length > 0"
            class="typed-config-card mods-workspace-card"
            data-testid="variant-mod-support-card">
            <div class="typed-config-header mods-workspace-header">
              <div>
                <div class="typed-config-title font-display">Variant Mod Support</div>
                <div class="typed-config-copy text-xy-muted">
                  Mod support for this game is defined per server software variant.
                </div>
              </div>
            </div>
            <div class="variant-mod-list">
              <div v-for="variant in variantModSummaries" :key="variant.id" class="variant-mod-row">
                <span class="variant-mod-name">{{ variant.name }}</span>
                <template v-if="variant.hasModSupport">
                  <span class="mods-status-chip mods-status-chip--active">
                    {{ variant.sourceLabels.join(' · ') }}
                  </span>
                  <span v-if="variant.installPath" class="mods-status-chip">
                    {{ variant.installPath }}
                  </span>
                </template>
                <span v-else class="variant-mod-none text-xy-muted">No mod support</span>
              </div>
            </div>
          </div>

          <div class="typed-config-card mods-workspace-card">
            <div class="typed-config-header mods-workspace-header">
              <div>
                <div class="typed-config-title font-display">
                  {{ variantModSummaries.length > 0 ? 'Game-wide Default' : 'Mod Support' }}
                </div>
                <div class="typed-config-copy text-xy-muted">
                  {{ gameModProfileCopy }}
                </div>
              </div>
              <q-btn
                v-if="!game.modProfile"
                color="accent"
                flat
                label="Enable Mod Support"
                no-caps
                @click="addGameModProfile" />
              <q-btn
                v-else
                color="negative"
                flat
                label="Remove"
                no-caps
                @click="clearGameModProfile" />
            </div>

            <template v-if="game.modProfile">
              <div class="mods-status-row">
                <span class="mods-status-chip mods-status-chip--active"> Support enabled </span>
                <span class="mods-status-chip"> Provider · {{ activeModSourceLabel }} </span>
                <span class="mods-status-chip">
                  {{ game.modProfile.installPath ? 'Install path ready' : 'Install path pending' }}
                </span>
              </div>

              <div class="typed-config-fields">
                <q-input
                  v-model="game.modProfile.installPath"
                  hint="Where downloaded mods should be written"
                  label="Install Path"
                  outlined
                  persistent-hint />

                <q-select
                  v-model="game.modProfile.sources[0].id"
                  :options="modSourceOptions"
                  emit-value
                  label="Mod Source"
                  map-options
                  outlined
                  @update:model-value="onModSourceProviderChanged(game.modProfile.sources[0])" />

                <q-input
                  :autogrow="getModSourceConfig(game.modProfile.sources[0].id).mode === 'json'"
                  :hint="getModSourceConfig(game.modProfile.sources[0].id).primaryHint"
                  :label="getModSourceConfig(game.modProfile.sources[0].id).primaryLabel"
                  :model-value="readModSourceDisplayValue(game.modProfile.sources[0])"
                  :placeholder="getModSourceConfig(game.modProfile.sources[0].id).placeholder"
                  :type="
                    getModSourceConfig(game.modProfile.sources[0].id).mode === 'json'
                      ? 'textarea'
                      : 'text'
                  "
                  outlined
                  persistent-hint
                  @update:model-value="
                    updateModSourceDisplayValue(game.modProfile.sources[0], $event)
                  " />
              </div>
            </template>
          </div>
        </template>
      </div>
    </div>
  </section>
</template>

<script lang="ts" setup>
import { computed, inject } from 'vue'
import { gameFormContextKey } from './GameFormTypes'

const ctx = inject(gameFormContextKey)
if (!ctx) throw new Error('GameFormModsTab must be used inside GameForm')

const {
  game,
  managedModConfig,
  variantModSummaries,
  modSourceOptions,
  activeModSourceLabel,
  addGameModProfile,
  clearGameModProfile,
  onModSourceProviderChanged,
  readModSourceDisplayValue,
  updateModSourceDisplayValue,
  getModSourceConfig,
} = ctx

const gameModProfileCopy = computed(() => {
  if (game.value.modProfile) {
    return 'Configure the download provider and install path for server mods.'
  }
  if (variantModSummaries.value.length > 0) {
    return 'No game-wide default. Variants without their own mod support inherit this setting.'
  }
  return 'Mod support is off. Enable it to configure a download provider for this game.'
})
</script>
