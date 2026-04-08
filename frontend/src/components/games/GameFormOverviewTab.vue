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
            class="col-12 col-sm-6"
            outlined
            type="text"
            label="Unique ID *"
            :rules="idRules"
            reactive-rules
            lazy-rules
            hint="ID of the game all lowercase. e.g: minecraft" />
          <q-input
            v-model="game.name"
            class="col-12 col-sm-6"
            outlined
            type="text"
            label="Name *"
            :rules="nameRules"
            reactive-rules
            lazy-rules
            hint="Name of the game. e.g: Minecraft" />
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
            class="col-12 col-sm-4"
            outlined
            type="number"
            label="Default Port *"
            :rules="portRules"
            reactive-rules
            lazy-rules
            hint="Default server port. e.g: 25565" />
          <q-input
            v-model.number="defaultQueryPort"
            class="col-12 col-sm-4"
            outlined
            type="number"
            label="Default Query Port *"
            :rules="portRules"
            reactive-rules
            lazy-rules
            hint="Default server query port. e.g: 25565" />
          <q-input
            v-model="game.steamAppid"
            class="col-12 col-sm-4"
            outlined
            type="number"
            label="Steam App ID"
            hint="Steam AppID if it's available on steamcmd. e.g: 294420" />
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
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.windowsSupport }"
            :aria-pressed="game.windowsSupport"
            @click="game.windowsSupport = !game.windowsSupport">
            <span class="feature-dot"></span>
            <span class="feature-label">Windows Support</span>
          </button>
          <button
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.linuxSupport }"
            :aria-pressed="game.linuxSupport"
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
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.usesSteamcmd }"
            :aria-pressed="game.usesSteamcmd"
            @click="game.usesSteamcmd = !game.usesSteamcmd">
            <span class="feature-dot"></span>
            <span class="feature-label">Uses Steamcmd</span>
          </button>
          <button
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.requiresSteamGameServerLoginToken }"
            :aria-pressed="game.requiresSteamGameServerLoginToken"
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
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.requireDedicatedIp }"
            :aria-pressed="game.requireDedicatedIp"
            @click="game.requireDedicatedIp = !game.requireDedicatedIp">
            <span class="feature-dot"></span>
            <span class="feature-label">Requires Dedicated IP</span>
          </button>
          <button
            type="button"
            class="feature-chip"
            :class="{ 'feature-chip--active': game.usesSourceQuery }"
            :aria-pressed="game.usesSourceQuery"
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
        class="platform-tabs"
        role="tablist"
        aria-label="Platform commands">
        <button
          type="button"
          role="tab"
          class="platform-tab"
          :class="{ 'platform-tab--active': activePlatform === 'windows' }"
          :aria-selected="activePlatform === 'windows'"
          @click="activePlatform = 'windows'">
          <q-icon
            name="desktop_windows"
            size="14px"
            :class="
              activePlatform === 'windows'
                ? 'platform-icon-windows-active'
                : 'platform-icon-inactive'
            " />
          <span class="font-display">Windows</span>
        </button>
        <button
          type="button"
          role="tab"
          class="platform-tab"
          :class="{ 'platform-tab--active': activePlatform === 'linux' }"
          :aria-selected="activePlatform === 'linux'"
          @click="activePlatform = 'linux'">
          <q-icon
            name="terminal"
            size="14px"
            :class="
              activePlatform === 'linux' ? 'platform-icon-linux-active' : 'platform-icon-inactive'
            " />
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.windowsStopCommand)"></div>
            <textarea
              v-model="game.windowsStopCommand"
              class="cmd-textarea font-mono"
              rows="1"
              aria-label="Windows stop command"
              placeholder="/stop"></textarea>
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
                  data-testid="windows-install-type"
                  aria-label="Windows install command type"
                  class="cmd-type-select font-mono"
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
                  data-testid="windows-install-shell"
                  aria-label="Windows install shell"
                  class="cmd-type-select font-mono"
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.windowsInstallCommand)"></div>
            <textarea
              v-model="game.windowsInstallCommand"
              data-testid="windows-install-command"
              class="cmd-textarea font-mono"
              rows="2"
              aria-label="Windows install command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></textarea>
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
                  data-testid="windows-update-type"
                  aria-label="Windows update command type"
                  class="cmd-type-select font-mono"
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
                  data-testid="windows-update-shell"
                  aria-label="Windows update shell"
                  class="cmd-type-select font-mono"
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.windowsUpdateCommand)"></div>
            <textarea
              v-model="game.windowsUpdateCommand"
              data-testid="windows-update-command"
              class="cmd-textarea font-mono"
              rows="2"
              aria-label="Windows update command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></textarea>
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
            class="cmd-textarea font-mono"
            rows="1"
            aria-label="Windows working directory"
            placeholder="./server"></textarea>
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.linuxStopCommand)"></div>
            <textarea
              v-model="game.linuxStopCommand"
              class="cmd-textarea font-mono"
              rows="1"
              aria-label="Linux stop command"
              placeholder="/stop"></textarea>
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
                  data-testid="linux-install-type"
                  aria-label="Linux install command type"
                  class="cmd-type-select font-mono"
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
                  data-testid="linux-install-shell"
                  aria-label="Linux install shell"
                  class="cmd-type-select font-mono"
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.linuxInstallCommand)"></div>
            <textarea
              v-model="game.linuxInstallCommand"
              data-testid="linux-install-command"
              class="cmd-textarea font-mono"
              rows="2"
              aria-label="Linux install command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></textarea>
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
                  data-testid="linux-update-type"
                  aria-label="Linux update command type"
                  class="cmd-type-select font-mono"
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
                  data-testid="linux-update-shell"
                  aria-label="Linux update shell"
                  class="cmd-type-select font-mono"
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
              class="cmd-highlight font-mono"
              aria-hidden="true"
              v-html="highlightCommand(game.linuxUpdateCommand)"></div>
            <textarea
              v-model="game.linuxUpdateCommand"
              data-testid="linux-update-command"
              class="cmd-textarea font-mono"
              rows="2"
              aria-label="Linux update command"
              placeholder="steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></textarea>
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
            class="cmd-textarea font-mono"
            rows="1"
            aria-label="Linux working directory"
            placeholder="./server"></textarea>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { CommandType, CommandProcessor } from '@/proto/shared_pb'
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
