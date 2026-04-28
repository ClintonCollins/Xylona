<template>
  <!-- eslint-disable vue/no-v-html -- accepted per CLAUDE.md: highlightCommand() syntax highlighting -->
  <!-- Identity & Networking (dense metadata strip) -->
  <section class="form-section form-section--compact">
    <div class="overview-metadata">
      <div class="overview-metadata-group">
        <div class="section-header">
          <span class="section-bar" style="background-color: var(--xy-accent)"></span>
          <h2 class="section-title font-display">Identity</h2>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model="game.id"
            :disable="existingGame"
            :rules="idRules"
            class="col-12 col-sm-6"
            hint="ID of the game all lowercase. e.g: minecraft"
            label="Unique ID *"
            lazy-rules
            outlined
            reactive-rules
            type="text" />
          <q-input
            v-model="game.name"
            :rules="nameRules"
            class="col-12 col-sm-6"
            hint="Name of the game. e.g: Minecraft"
            label="Name *"
            lazy-rules
            outlined
            reactive-rules
            type="text" />
        </div>
      </div>
      <div class="overview-metadata-group">
        <div class="section-header">
          <span class="section-bar" style="background-color: var(--xy-primary)"></span>
          <h2 class="section-title font-display">Networking</h2>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model.number="defaultPort"
            :rules="portRules"
            class="col-12 col-sm-4"
            hint="Default server port. e.g: 25565"
            label="Default Port *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-model.number="defaultQueryPort"
            :rules="portRules"
            class="col-12 col-sm-4"
            hint="Default server query port. e.g: 25565"
            label="Default Query Port *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-model="game.steamAppid"
            class="col-12 col-sm-4"
            hint="Steam AppID if it's available on steamcmd. e.g: 294420"
            label="Steam App ID"
            outlined
            type="number" />
        </div>
      </div>
    </div>
  </section>

  <!-- Features -->
  <section class="form-section">
    <div class="section-header">
      <span class="section-bar" style="background-color: var(--xy-success)"></span>
      <h2 class="section-title font-display">Features</h2>
      <span class="section-line"></span>
    </div>
    <div class="feature-groups">
      <div class="feature-group">
        <span class="feature-group-label text-xy-muted font-display">Platform</span>
        <div class="feature-chips">
          <button
            :aria-pressed="game.windowsSupport"
            :class="{ 'feature-chip--active': game.windowsSupport }"
            class="feature-chip"
            type="button"
            @click="game.windowsSupport = !game.windowsSupport">
            <span class="feature-dot"></span>
            <span class="feature-label">Windows Support</span>
          </button>
          <button
            :aria-pressed="game.linuxSupport"
            :class="{ 'feature-chip--active': game.linuxSupport }"
            class="feature-chip"
            type="button"
            @click="game.linuxSupport = !game.linuxSupport">
            <span class="feature-dot"></span>
            <span class="feature-label">Linux Support</span>
          </button>
        </div>
      </div>
      <div class="feature-group">
        <span class="feature-group-label text-xy-muted font-display">Steam</span>
        <div class="feature-chips">
          <button
            :aria-pressed="game.usesSteamcmd"
            :class="{ 'feature-chip--active': game.usesSteamcmd }"
            class="feature-chip"
            type="button"
            @click="game.usesSteamcmd = !game.usesSteamcmd">
            <span class="feature-dot"></span>
            <span class="feature-label">Uses Steamcmd</span>
          </button>
          <button
            :aria-pressed="game.requiresSteamGameServerLoginToken"
            :class="{ 'feature-chip--active': game.requiresSteamGameServerLoginToken }"
            class="feature-chip"
            type="button"
            @click="
              game.requiresSteamGameServerLoginToken = !game.requiresSteamGameServerLoginToken
            ">
            <span class="feature-dot"></span>
            <span class="feature-label">Steam Login Token Required</span>
          </button>
        </div>
      </div>
      <div class="feature-group">
        <span class="feature-group-label text-xy-muted font-display">Network</span>
        <div class="feature-chips">
          <button
            :aria-pressed="game.bindsToAllIps"
            :class="{ 'feature-chip--active': game.bindsToAllIps }"
            class="feature-chip"
            type="button"
            @click="game.bindsToAllIps = !game.bindsToAllIps">
            <span class="feature-dot"></span>
            <span class="feature-label">Binds to All IPs</span>
          </button>
          <button
            :aria-pressed="game.usesSourceQuery"
            :class="{ 'feature-chip--active': game.usesSourceQuery }"
            class="feature-chip"
            type="button"
            @click="game.usesSourceQuery = !game.usesSourceQuery">
            <span class="feature-dot"></span>
            <span class="feature-label">Uses Source Query</span>
          </button>
        </div>
      </div>
    </div>
  </section>

  <!-- Platform Commands -->
  <section class="form-section form-section--last">
    <div class="section-header">
      <span class="section-bar" style="background-color: var(--xy-warning)"></span>
      <h2 class="section-title font-display">Install &amp; Update</h2>
      <span class="section-line"></span>
    </div>

    <!-- No platforms enabled -->
    <div v-if="!game.windowsSupport && !game.linuxSupport" class="platform-empty text-xy-muted">
      Enable Windows or Linux support above to configure commands.
    </div>

    <!-- Platform tab switcher -->
    <template v-else>
      <div
        v-if="game.windowsSupport && game.linuxSupport"
        aria-label="Platform commands"
        class="platform-tabs"
        role="tablist">
        <button
          :aria-selected="activePlatform === 'windows'"
          :class="{ 'platform-tab--active': activePlatform === 'windows' }"
          class="platform-tab"
          role="tab"
          type="button"
          @click="activePlatform = 'windows'">
          <q-icon
            :class="
              activePlatform === 'windows'
                ? 'platform-icon-windows-active'
                : 'platform-icon-inactive'
            "
            name="desktop_windows"
            size="14px" />
          <span class="font-display">Windows</span>
        </button>
        <button
          :aria-selected="activePlatform === 'linux'"
          :class="{ 'platform-tab--active': activePlatform === 'linux' }"
          class="platform-tab"
          role="tab"
          type="button"
          @click="activePlatform = 'linux'">
          <q-icon
            :class="
              activePlatform === 'linux' ? 'platform-icon-linux-active' : 'platform-icon-inactive'
            "
            name="terminal"
            size="14px" />
          <span class="font-display">Linux</span>
        </button>
      </div>

      <!-- Windows Commands -->
      <div v-if="activePlatformResolved === 'windows'" class="platform-commands">
        <!-- Stop Command -->
        <div class="cmd-block cmd-block--windows">
          <div class="cmd-header">
            <span class="cmd-label">STOP COMMAND</span>
            <q-badge class="cmd-badge font-mono" color="transparent" text-color="grey-6">
              stdin
            </q-badge>
          </div>
          <div class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.windowsStopCommand)"></div>
            <textarea
              v-model="game.windowsStopCommand"
              aria-label="Windows stop command"
              class="cmd-textarea font-mono"
              placeholder="/stop"
              rows="1"></textarea>
          </div>
        </div>

        <!-- Install Command -->
        <div class="cmd-block cmd-block--windows">
          <div class="cmd-header">
            <span class="cmd-label">INSTALL COMMAND</span>
            <div class="cmd-type-row">
              <div class="cmd-type-group">
                <span class="cmd-type-label">Type</span>
                <select
                  :value="game.windowsInstallType"
                  aria-label="Windows install command type"
                  class="cmd-type-select font-mono"
                  data-testid="windows-install-type"
                  @change="
                    game.windowsInstallType = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandType
                  ">
                  <option v-for="opt in commandTypeOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
              <div v-if="isCommandTypeCommand(game.windowsInstallType)" class="cmd-type-group">
                <span class="cmd-type-label">Shell</span>
                <select
                  :value="game.windowsInstallCommandProcessor"
                  aria-label="Windows install shell"
                  class="cmd-type-select font-mono"
                  data-testid="windows-install-shell"
                  @change="
                    game.windowsInstallCommandProcessor = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandProcessor
                  ">
                  <option
                    v-for="opt in windowsCommandProcessorOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
            </div>
          </div>
          <div v-if="isCommandTypeCommand(game.windowsInstallType)" class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.windowsInstallCommand)"></div>
            <textarea
              v-model="game.windowsInstallCommand"
              aria-label="Windows install command"
              class="cmd-textarea font-mono"
              data-testid="windows-install-command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
              rows="2"></textarea>
          </div>
          <div v-else class="cmd-internal font-mono">
            {{ commandTypeSummary(game.windowsInstallType, 'install') }}
          </div>
        </div>

        <!-- Update Command -->
        <div class="cmd-block cmd-block--windows">
          <div class="cmd-header">
            <span class="cmd-label">UPDATE COMMAND</span>
            <div class="cmd-type-row">
              <div class="cmd-type-group">
                <span class="cmd-type-label">Type</span>
                <select
                  :value="game.windowsUpdateType"
                  aria-label="Windows update command type"
                  class="cmd-type-select font-mono"
                  data-testid="windows-update-type"
                  @change="
                    game.windowsUpdateType = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandType
                  ">
                  <option v-for="opt in commandTypeOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
              <div v-if="isCommandTypeCommand(game.windowsUpdateType)" class="cmd-type-group">
                <span class="cmd-type-label">Shell</span>
                <select
                  :value="game.windowsUpdateCommandProcessor"
                  aria-label="Windows update shell"
                  class="cmd-type-select font-mono"
                  data-testid="windows-update-shell"
                  @change="
                    game.windowsUpdateCommandProcessor = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandProcessor
                  ">
                  <option
                    v-for="opt in windowsCommandProcessorOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
            </div>
          </div>
          <div v-if="isCommandTypeCommand(game.windowsUpdateType)" class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.windowsUpdateCommand)"></div>
            <textarea
              v-model="game.windowsUpdateCommand"
              aria-label="Windows update command"
              class="cmd-textarea font-mono"
              data-testid="windows-update-command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
              rows="2"></textarea>
          </div>
          <div v-else class="cmd-internal font-mono">
            {{ commandTypeSummary(game.windowsUpdateType, 'update') }}
          </div>
        </div>

        <!-- Working Directory -->
        <div class="cmd-block cmd-block--windows">
          <div class="cmd-header">
            <span class="cmd-label">WORKING DIRECTORY</span>
          </div>
          <textarea
            v-model="game.windowsWorkingDirectory"
            aria-label="Windows working directory"
            class="cmd-textarea font-mono"
            placeholder="./server"
            rows="1"></textarea>
        </div>
      </div>

      <!-- Linux Commands -->
      <div v-if="activePlatformResolved === 'linux'" class="platform-commands">
        <!-- Stop Command -->
        <div class="cmd-block cmd-block--linux">
          <div class="cmd-header">
            <span class="cmd-label">STOP COMMAND</span>
            <q-badge class="cmd-badge font-mono" color="transparent" text-color="grey-6">
              stdin
            </q-badge>
          </div>
          <div class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.linuxStopCommand)"></div>
            <textarea
              v-model="game.linuxStopCommand"
              aria-label="Linux stop command"
              class="cmd-textarea font-mono"
              placeholder="/stop"
              rows="1"></textarea>
          </div>
        </div>

        <!-- Install Command -->
        <div class="cmd-block cmd-block--linux">
          <div class="cmd-header">
            <span class="cmd-label">INSTALL COMMAND</span>
            <div class="cmd-type-row">
              <div class="cmd-type-group">
                <span class="cmd-type-label">Type</span>
                <select
                  :value="game.linuxInstallType"
                  aria-label="Linux install command type"
                  class="cmd-type-select font-mono"
                  data-testid="linux-install-type"
                  @change="
                    game.linuxInstallType = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandType
                  ">
                  <option v-for="opt in commandTypeOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
              <div v-if="isCommandTypeCommand(game.linuxInstallType)" class="cmd-type-group">
                <span class="cmd-type-label">Shell</span>
                <select
                  :value="game.linuxInstallCommandProcessor"
                  aria-label="Linux install shell"
                  class="cmd-type-select font-mono"
                  data-testid="linux-install-shell"
                  @change="
                    game.linuxInstallCommandProcessor = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandProcessor
                  ">
                  <option
                    v-for="opt in linuxCommandProcessorOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
            </div>
          </div>
          <div v-if="isCommandTypeCommand(game.linuxInstallType)" class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.linuxInstallCommand)"></div>
            <textarea
              v-model="game.linuxInstallCommand"
              aria-label="Linux install command"
              class="cmd-textarea font-mono"
              data-testid="linux-install-command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
              rows="2"></textarea>
          </div>
          <div v-else class="cmd-internal font-mono">
            {{ commandTypeSummary(game.linuxInstallType, 'install') }}
          </div>
        </div>

        <!-- Update Command -->
        <div class="cmd-block cmd-block--linux">
          <div class="cmd-header">
            <span class="cmd-label">UPDATE COMMAND</span>
            <div class="cmd-type-row">
              <div class="cmd-type-group">
                <span class="cmd-type-label">Type</span>
                <select
                  :value="game.linuxUpdateType"
                  aria-label="Linux update command type"
                  class="cmd-type-select font-mono"
                  data-testid="linux-update-type"
                  @change="
                    game.linuxUpdateType = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandType
                  ">
                  <option v-for="opt in commandTypeOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
              <div v-if="isCommandTypeCommand(game.linuxUpdateType)" class="cmd-type-group">
                <span class="cmd-type-label">Shell</span>
                <select
                  :value="game.linuxUpdateCommandProcessor"
                  aria-label="Linux update shell"
                  class="cmd-type-select font-mono"
                  data-testid="linux-update-shell"
                  @change="
                    game.linuxUpdateCommandProcessor = Number(
                      ($event.target as HTMLSelectElement).value,
                    ) as CommandProcessor
                  ">
                  <option
                    v-for="opt in linuxCommandProcessorOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>
            </div>
          </div>
          <div v-if="isCommandTypeCommand(game.linuxUpdateType)" class="cmd-input-wrap">
            <div
              aria-hidden="true"
              class="cmd-highlight font-mono"
              v-html="highlightCommand(game.linuxUpdateCommand)"></div>
            <textarea
              v-model="game.linuxUpdateCommand"
              aria-label="Linux update command"
              class="cmd-textarea font-mono"
              data-testid="linux-update-command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
              rows="2"></textarea>
          </div>
          <div v-else class="cmd-internal font-mono">
            {{ commandTypeSummary(game.linuxUpdateType, 'update') }}
          </div>
        </div>

        <!-- Working Directory -->
        <div class="cmd-block cmd-block--linux">
          <div class="cmd-header">
            <span class="cmd-label">WORKING DIRECTORY</span>
          </div>
          <textarea
            v-model="game.linuxWorkingDirectory"
            aria-label="Linux working directory"
            class="cmd-textarea font-mono"
            placeholder="./server"
            rows="1"></textarea>
        </div>
      </div>
    </template>
  </section>
</template>

<script lang="ts" setup>
import { inject } from 'vue'
import { CommandProcessor, CommandType } from '@/proto/shared_pb'
import { gameFormContextKey } from './GameFormTypes'

const ctx = inject(gameFormContextKey)
if (!ctx) throw new Error('GameFormOverviewTab must be used inside GameForm')

const {
  game,
  existingGame,
  defaultPort,
  defaultQueryPort,
  activePlatform,
  activePlatformResolved,
  idRules,
  nameRules,
  portRules,
  commandTypeOptions,
  linuxCommandProcessorOptions,
  windowsCommandProcessorOptions,
  isCommandTypeCommand,
  commandTypeSummary,
  highlightCommand,
} = ctx
</script>
