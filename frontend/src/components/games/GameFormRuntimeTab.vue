<template>
  <section class="form-section form-section--last">
    <h2 class="game-form-sr-only">Runtime</h2>

    <div class="structured-start-stack">
      <start-args-template-editor
        :active-platform="activePlatformResolved ?? undefined"
        :advanced-expanded="runtimeSequenceExpanded"
        :baseline-linux-base-command="baselineLinuxBaseCommand"
        :baseline-linux-template="baselineLinuxStartArgsTemplate"
        :baseline-windows-base-command="baselineWindowsBaseCommand"
        :baseline-windows-template="baselineWindowsStartArgsTemplate"
        :linux-base-command="game.linuxBaseCommand"
        :linux-enabled="game.linuxSupport"
        :linux-template="linuxStartArgsTemplate"
        :windows-base-command="game.windowsBaseCommand"
        :windows-enabled="game.windowsSupport"
        :windows-template="windowsStartArgsTemplate"
        class="structured-start-stack__editor"
        mode="preview"
        @update:active-platform="activePlatform = $event"
        @update:advanced-expanded="updateRuntimeSequenceExpanded"
        @update:linux-template="linuxStartArgsTemplate = $event"
        @update:windows-template="windowsStartArgsTemplate = $event"
        @update:linux-base-command="game.linuxBaseCommand = $event"
        @update:windows-base-command="game.windowsBaseCommand = $event" />

      <section class="runtime-policy-panel">
        <button
          :aria-expanded="String(runtimePolicyExpanded)"
          :aria-label="
            runtimePolicyExpanded ? 'Collapse runtime guardrails' : 'Expand runtime guardrails'
          "
          aria-controls="runtime-policy-panel"
          aria-describedby="runtime-policy-assistive-summary"
          class="runtime-policy-toggle"
          data-testid="runtime-policy-toggle"
          type="button"
          @click="toggleRuntimePolicy">
          <div class="runtime-policy-toggle-copy">
            <span class="runtime-policy-eyebrow font-display">Runtime Guardrails</span>
            <span class="runtime-policy-summary-line text-xy-secondary">
              {{ runtimePolicySummary.join(' · ') }}
            </span>
          </div>

          <div class="runtime-policy-header-actions">
            <span class="runtime-policy-toggle-indicator font-display">
              {{ runtimePolicyExpanded ? 'Hide details' : 'Review guardrails' }}
              <q-icon
                :name="runtimePolicyExpanded ? 'expand_less' : 'expand_more'"
                color="accent"
                size="18px" />
            </span>
          </div>
        </button>

        <span id="runtime-policy-assistive-summary" class="runtime-policy-sr-only">
          {{ runtimePolicyAssistiveSummary }}
        </span>

        <transition name="runtime-policy-panel">
          <div
            v-if="runtimePolicyExpanded"
            id="runtime-policy-panel"
            class="runtime-policy-content"
            data-testid="runtime-policy-panel">
            <div class="runtime-policy-layout">
              <div class="runtime-policy-subsection runtime-policy-subsection--reserved">
                <div class="runtime-policy-card-head">
                  <div class="runtime-policy-card-copy">
                    <div class="runtime-policy-subhead font-display">Reserved arguments</div>
                    <div class="runtime-policy-subcopy text-xy-muted">
                      Protect flags that inherited servers should never override.
                    </div>
                  </div>
                  <span class="runtime-policy-quantity">
                    {{
                      `${startArgBlocklist.length} ${startArgBlocklist.length === 1 ? 'rule' : 'rules'}`
                    }}
                  </span>
                </div>
                <blocklist-editor
                  :blocklist="startArgBlocklist"
                  :show-header="false"
                  @update:blocklist="startArgBlocklist = $event" />
              </div>

              <div class="runtime-policy-rail">
                <div class="runtime-policy-subsection runtime-policy-subsection--owner">
                  <div class="runtime-policy-rail-head">
                    <div class="runtime-policy-card-copy">
                      <div class="runtime-policy-subhead font-display">Owner edits</div>
                      <div class="runtime-policy-subcopy text-xy-muted">
                        Only editable arguments can be tuned downstream.
                      </div>
                    </div>
                    <q-toggle
                      v-model="game.allowStartArgEditing"
                      aria-label="Allow owner edits"
                      color="accent" />
                  </div>
                  <div class="runtime-policy-mini-note text-xy-muted">
                    {{
                      game.allowStartArgEditing
                        ? 'Enabled for editable arguments only.'
                        : 'Only this game definition can change launch arguments.'
                    }}
                  </div>
                </div>

                <div
                  v-if="existingGame && !copyGame"
                  class="runtime-policy-subsection runtime-policy-subsection--impact">
                  <div class="runtime-policy-card-head runtime-policy-card-head--stacked">
                    <div class="runtime-policy-card-copy">
                      <div class="runtime-policy-subhead font-display">Affected servers</div>
                      <div class="runtime-policy-subcopy text-xy-muted">
                        Review inherited servers before saving.
                      </div>
                    </div>
                    <span class="runtime-policy-quantity">
                      {{
                        `${downstreamImpactServers.length} ${downstreamImpactServers.length === 1 ? 'server' : 'servers'}`
                      }}
                    </span>
                  </div>
                  <downstream-impact-panel
                    :servers="downstreamImpactServers"
                    :show-header="false" />
                </div>
              </div>
            </div>
          </div>
        </transition>
      </section>

      <start-args-template-editor
        :active-platform="activePlatformResolved ?? undefined"
        :advanced-expanded="runtimeSequenceExpanded"
        :baseline-linux-base-command="baselineLinuxBaseCommand"
        :baseline-linux-template="baselineLinuxStartArgsTemplate"
        :baseline-windows-base-command="baselineWindowsBaseCommand"
        :baseline-windows-template="baselineWindowsStartArgsTemplate"
        :linux-base-command="game.linuxBaseCommand"
        :linux-enabled="game.linuxSupport"
        :linux-template="linuxStartArgsTemplate"
        :windows-base-command="game.windowsBaseCommand"
        :windows-enabled="game.windowsSupport"
        :windows-template="windowsStartArgsTemplate"
        class="structured-start-stack__advanced"
        mode="advanced"
        @update:active-platform="activePlatform = $event"
        @update:advanced-expanded="updateRuntimeSequenceExpanded"
        @update:linux-template="linuxStartArgsTemplate = $event"
        @update:windows-template="windowsStartArgsTemplate = $event"
        @update:linux-base-command="game.linuxBaseCommand = $event"
        @update:windows-base-command="game.windowsBaseCommand = $event" />
    </div>
  </section>
</template>

<script lang="ts" setup>
import { inject } from 'vue'
import BlocklistEditor from './BlocklistEditor.vue'
import DownstreamImpactPanel from './DownstreamImpactPanel.vue'
import StartArgsTemplateEditor from './StartArgsTemplateEditor.vue'
import { gameFormContextKey } from './GameFormTypes'

const ctx = inject(gameFormContextKey)
if (!ctx) throw new Error('GameFormRuntimeTab must be used inside GameForm')

const {
  game,
  existingGame,
  copyGame,
  activePlatform,
  activePlatformResolved,
  linuxStartArgsTemplate,
  windowsStartArgsTemplate,
  startArgBlocklist,
  baselineLinuxBaseCommand,
  baselineWindowsBaseCommand,
  baselineLinuxStartArgsTemplate,
  baselineWindowsStartArgsTemplate,
  runtimeSequenceExpanded,
  runtimePolicyExpanded,
  runtimePolicySummary,
  runtimePolicyAssistiveSummary,
  toggleRuntimePolicy,
  updateRuntimeSequenceExpanded,
  downstreamImpactServers,
} = ctx
</script>
