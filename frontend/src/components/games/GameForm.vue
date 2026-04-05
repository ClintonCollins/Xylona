<template>
  <!-- eslint-disable vue/no-v-html -- accepted per CLAUDE.md: highlightCommand() syntax highlighting -->
  <div class="game-form-wrapper full-width">
    <!-- Sentinel for sticky detection -->
    <div ref="stickySentinel" class="sticky-sentinel"></div>
    <!-- Header -->
    <div class="game-form-header" :class="{ 'is-stuck': isStuck }">
      <div class="game-form-header-left">
        <nav class="game-form-breadcrumbs" aria-label="Breadcrumb">
          <ol class="breadcrumb-list">
            <li>
              <router-link to="/games" class="breadcrumb-link">Games</router-link>
            </li>
            <li aria-current="page">
              <span class="breadcrumb-current">{{ breadcrumbLabel }}</span>
            </li>
          </ol>
        </nav>
        <h1 class="game-form-title font-display">{{ formTitle }}</h1>
      </div>
      <div class="game-form-header-actions">
        <router-link
          v-if="!existingGame && !copyGame"
          to="/games/new"
          class="game-form-guided-link text-caption">
          Use guided setup
        </router-link>
        <q-btn flat label="Cancel" :disable="submitting" @click="handleCancel" />
        <q-btn
          label="Save"
          color="primary"
          :loading="submitting"
          :disable="loading"
          @click="submit" />
      </div>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="game-form-loading">
      <q-spinner-dots size="40px" color="primary" />
      <div class="text-xy-secondary q-mt-sm">Loading game details...</div>
    </div>

    <div v-else class="game-form-body">
      <q-form ref="formRef">
        <div class="game-form-tabs-panel">
          <div class="game-form-tabs-shell">
            <div class="game-form-tabs" role="tablist" aria-label="Game editor sections">
              <button
                v-for="tab in formTabs"
                :id="formTabID(tab.id)"
                :key="tab.id"
                :data-testid="`game-form-tab-${tab.id}`"
                type="button"
                role="tab"
                class="game-form-tab"
                :class="{ 'game-form-tab--active': activeFormTab === tab.id }"
                :aria-controls="formTabPanelID(tab.id)"
                :aria-selected="activeFormTab === tab.id"
                :tabindex="activeFormTab === tab.id ? 0 : -1"
                :title="tab.copy"
                @keydown="handleFormTabKeydown($event, tab.id)"
                @click="activeFormTab = tab.id">
                <span class="game-form-tab__label font-display">{{ tab.label }}</span>
              </button>
            </div>
          </div>

          <!-- Tab descriptions available via tooltip on tab buttons -->
        </div>

        <div
          v-show="activeFormTab === 'overview'"
          :id="formTabPanelID('overview')"
          data-testid="game-form-tab-panel-overview"
          role="tabpanel"
          :aria-hidden="activeFormTab !== 'overview'"
          :inert="activeFormTab !== 'overview'"
          :aria-labelledby="formTabID('overview')"
          class="game-form-tab-panel">
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
                      game.requiresSteamGameServerLoginToken =
                        !game.requiresSteamGameServerLoginToken
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
            <div
              v-if="!game.windowsSupport && !game.linuxSupport"
              class="platform-empty text-xy-muted">
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
                      activePlatform === 'linux'
                        ? 'platform-icon-linux-active'
                        : 'platform-icon-inactive'
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
                          <option
                            v-for="opt in commandTypeOptions"
                            :key="opt.value"
                            :value="opt.value">
                            {{ opt.label }}
                          </option>
                        </select>
                      </div>
                      <div
                        v-if="isCommandTypeCommand(game.windowsInstallType)"
                        class="cmd-type-group">
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
                          <option
                            v-for="opt in commandTypeOptions"
                            :key="opt.value"
                            :value="opt.value">
                            {{ opt.label }}
                          </option>
                        </select>
                      </div>
                      <div
                        v-if="isCommandTypeCommand(game.windowsUpdateType)"
                        class="cmd-type-group">
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
                          <option
                            v-for="opt in commandTypeOptions"
                            :key="opt.value"
                            :value="opt.value">
                            {{ opt.label }}
                          </option>
                        </select>
                      </div>
                      <div
                        v-if="isCommandTypeCommand(game.linuxInstallType)"
                        class="cmd-type-group">
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
                          <option
                            v-for="opt in commandTypeOptions"
                            :key="opt.value"
                            :value="opt.value">
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
        </div>

        <div
          v-show="activeFormTab === 'runtime'"
          :id="formTabPanelID('runtime')"
          data-testid="game-form-tab-panel-runtime"
          role="tabpanel"
          :aria-hidden="activeFormTab !== 'runtime'"
          :inert="activeFormTab !== 'runtime'"
          :aria-labelledby="formTabID('runtime')"
          class="game-form-tab-panel">
          <!-- Structured Start Command -->
          <section class="form-section form-section--last">
            <h2 class="game-form-sr-only">Runtime</h2>

            <div class="structured-start-stack">
              <start-args-template-editor
                mode="preview"
                class="structured-start-stack__editor"
                :active-platform="activePlatformResolved ?? undefined"
                :advanced-expanded="runtimeSequenceExpanded"
                :baseline-linux-template="baselineLinuxStartArgsTemplate"
                :baseline-windows-template="baselineWindowsStartArgsTemplate"
                :baseline-linux-base-command="baselineLinuxBaseCommand"
                :baseline-windows-base-command="baselineWindowsBaseCommand"
                :linux-template="linuxStartArgsTemplate"
                :windows-template="windowsStartArgsTemplate"
                :linux-base-command="game.linuxBaseCommand"
                :windows-base-command="game.windowsBaseCommand"
                :linux-enabled="game.linuxSupport"
                :windows-enabled="game.windowsSupport"
                @update:active-platform="activePlatform = $event"
                @update:advanced-expanded="updateRuntimeSequenceExpanded"
                @update:linux-template="linuxStartArgsTemplate = $event"
                @update:windows-template="windowsStartArgsTemplate = $event"
                @update:linux-base-command="game.linuxBaseCommand = $event"
                @update:windows-base-command="game.windowsBaseCommand = $event" />

              <section class="runtime-policy-panel">
                <button
                  type="button"
                  class="runtime-policy-toggle"
                  data-testid="runtime-policy-toggle"
                  :aria-label="
                    runtimePolicyExpanded
                      ? 'Collapse runtime guardrails'
                      : 'Expand runtime guardrails'
                  "
                  aria-describedby="runtime-policy-assistive-summary"
                  :aria-expanded="String(runtimePolicyExpanded)"
                  aria-controls="runtime-policy-panel"
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
                        size="18px"
                        color="accent" />
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
                    data-testid="runtime-policy-panel"
                    class="runtime-policy-content">
                    <div class="runtime-policy-layout">
                      <div class="runtime-policy-subsection runtime-policy-subsection--reserved">
                        <div class="runtime-policy-card-head">
                          <div class="runtime-policy-card-copy">
                            <div class="runtime-policy-subhead font-display">
                              Reserved arguments
                            </div>
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
                              color="accent"
                              aria-label="Allow owner edits" />
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
                              <div class="runtime-policy-subhead font-display">
                                Affected servers
                              </div>
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
                mode="advanced"
                class="structured-start-stack__advanced"
                :active-platform="activePlatformResolved ?? undefined"
                :advanced-expanded="runtimeSequenceExpanded"
                :baseline-linux-template="baselineLinuxStartArgsTemplate"
                :baseline-windows-template="baselineWindowsStartArgsTemplate"
                :baseline-linux-base-command="baselineLinuxBaseCommand"
                :baseline-windows-base-command="baselineWindowsBaseCommand"
                :linux-template="linuxStartArgsTemplate"
                :windows-template="windowsStartArgsTemplate"
                :linux-base-command="game.linuxBaseCommand"
                :windows-base-command="game.windowsBaseCommand"
                :linux-enabled="game.linuxSupport"
                :windows-enabled="game.windowsSupport"
                @update:active-platform="activePlatform = $event"
                @update:advanced-expanded="updateRuntimeSequenceExpanded"
                @update:linux-template="linuxStartArgsTemplate = $event"
                @update:windows-template="windowsStartArgsTemplate = $event"
                @update:linux-base-command="game.linuxBaseCommand = $event"
                @update:windows-base-command="game.windowsBaseCommand = $event" />
            </div>
          </section>
        </div>

        <div
          v-show="activeFormTab === 'mods'"
          :id="formTabPanelID('mods')"
          data-testid="game-form-tab-panel-mods"
          role="tabpanel"
          :aria-hidden="activeFormTab !== 'mods'"
          :inert="activeFormTab !== 'mods'"
          :aria-labelledby="formTabID('mods')"
          class="game-form-tab-panel">
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
                    Choose one source and one install folder. Paste the provider reference Xylona
                    should use, such as a slug, workshop ID, or provider JSON payload.
                  </div>
                </div>
              </aside>

              <div class="mods-workspace">
                <div
                  v-if="managedModConfig"
                  class="typed-config-managed mods-workspace-card text-xy-muted">
                  This game uses advanced mod configuration outside the simple editor. Hidden
                  internal mod settings will be preserved when you save this game.
                </div>

                <div v-else class="typed-config-card mods-workspace-card">
                  <div class="typed-config-header mods-workspace-header">
                    <div>
                      <div class="typed-config-title font-display">Mod Support</div>
                      <div class="typed-config-copy text-xy-muted">
                        {{
                          game.modProfile
                            ? 'Configure the download provider and install path for server mods.'
                            : 'Mod support is off. Enable it to configure a download provider for this game.'
                        }}
                      </div>
                    </div>
                    <q-btn
                      v-if="!game.modProfile"
                      flat
                      no-caps
                      color="accent"
                      label="Enable Mod Support"
                      @click="addGameModProfile" />
                    <q-btn
                      v-else
                      flat
                      no-caps
                      color="negative"
                      label="Remove"
                      @click="clearGameModProfile" />
                  </div>

                  <template v-if="game.modProfile">
                    <div class="mods-status-row">
                      <span class="mods-status-chip mods-status-chip--active">
                        Support enabled
                      </span>
                      <span class="mods-status-chip"> Provider · {{ activeModSourceLabel }} </span>
                      <span class="mods-status-chip">
                        {{
                          game.modProfile.installPath
                            ? 'Install path ready'
                            : 'Install path pending'
                        }}
                      </span>
                    </div>

                    <div class="typed-config-fields">
                      <q-input
                        v-model="game.modProfile.installPath"
                        outlined
                        label="Install Path"
                        hint="Where downloaded mods should be written"
                        persistent-hint />

                      <q-select
                        v-model="game.modProfile.sources[0].id"
                        :options="modSourceOptions"
                        emit-value
                        map-options
                        outlined
                        label="Mod Source"
                        @update:model-value="
                          onModSourceProviderChanged(game.modProfile.sources[0])
                        " />

                      <q-input
                        :model-value="readModSourceDisplayValue(game.modProfile.sources[0])"
                        outlined
                        :type="
                          getModSourceConfig(game.modProfile.sources[0].id).mode === 'json'
                            ? 'textarea'
                            : 'text'
                        "
                        :autogrow="
                          getModSourceConfig(game.modProfile.sources[0].id).mode === 'json'
                        "
                        :label="getModSourceConfig(game.modProfile.sources[0].id).primaryLabel"
                        :hint="getModSourceConfig(game.modProfile.sources[0].id).primaryHint"
                        :placeholder="getModSourceConfig(game.modProfile.sources[0].id).placeholder"
                        persistent-hint
                        @update:model-value="
                          updateModSourceDisplayValue(game.modProfile.sources[0], $event)
                        " />
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </section>
        </div>

        <div
          v-show="activeFormTab === 'config'"
          :id="formTabPanelID('config')"
          data-testid="game-form-tab-panel-config"
          role="tabpanel"
          :aria-hidden="activeFormTab !== 'config'"
          :inert="activeFormTab !== 'config'"
          :aria-labelledby="formTabID('config')"
          class="game-form-tab-panel">
          <!-- Configuration Files -->
          <section class="form-section form-section--last">
            <div class="section-header">
              <span class="section-bar" style="background-color: var(--xy-purple)"></span>
              <h2 class="section-title font-display">Configuration Files</h2>
              <span class="section-line"></span>
            </div>
            <config-schema-list v-model="configSchemas" @edit-schema="navigateToSchemaEditor" />
          </section>
        </div>
      </q-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar, QForm } from 'quasar'
import {
  AddGameRequest,
  AddGameRequestSchema,
  EditGameRequest,
  EditGameRequestSchema,
  GetGameRequest,
  GetGameRequestSchema,
  GetGameResponse,
  ListGameServersRequestSchema,
  type ListGameServersResponse,
  UpdateGameConfigSchemasRequestSchema,
} from '@/proto/xylona_pb'
import { GetXylonaClient, ConnectErrorToString } from '@/utils/shared'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, Ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  CommandType,
  CommandProcessor,
  Game,
  GameSchema,
  type ModSource,
  ModProfileSchema,
  ModSourceSchema,
  UpdateProviderConfigSchema,
} from '@/proto/shared_pb'
import ConfigSchemaList from './ConfigSchemaList.vue'
import type { ConfigSchemaEntry } from './ConfigSchemaList.vue'
import BlocklistEditor from './BlocklistEditor.vue'
import DownstreamImpactPanel from './DownstreamImpactPanel.vue'
import StartArgsTemplateEditor from './StartArgsTemplateEditor.vue'
import {
  parseStartArgBlocklist,
  parseStartArgsPatches,
  parseStartArgsTemplate,
  serializeStartArgBlocklist,
  serializeStartArgsTemplate,
  type StartArgBlock,
  type StartArgBlocklistEntry,
} from '@/components/game_servers/start-args'
import {
  applySimpleGameConfig,
  getCommandProcessorOptions,
  getCommandTypeOptions,
  getModSourceConfig,
  getModSourceOptions,
  isCommandTypeCommand,
  isManagedGameConfig,
  isManagedModConfig,
  readModSourcePrimaryValue,
  writeModSourcePrimaryValue,
} from './game-form-provider-fields'
import { normalizeSteamAppID } from './game-form-normalization'

const $q = useQuasar()
const router = useRouter()
const formRef = ref<QForm | null>(null)
const stickySentinel = ref<HTMLElement | null>(null)
const isStuck = ref(false)
let stickyObserver: IntersectionObserver | null = null
const defaultPort: Ref<number | null> = ref(null)
const defaultQueryPort: Ref<number | null> = ref(null)
const loading = ref(false)
const submitting = ref(false)
const savedSuccessfully = ref(false)
let initialSnapshot = ''

type Platform = 'windows' | 'linux'
type GameFormTabID = 'overview' | 'runtime' | 'mods' | 'config'
const gameFormTabHistoryStateKey = 'xylonaGameFormTab'

const formTabs: Array<{ id: GameFormTabID; label: string; copy: string }> = [
  {
    id: 'overview',
    label: 'Overview',
    copy: 'Identity, ports, platform support, and install or update commands.',
  },
  {
    id: 'runtime',
    label: 'Runtime',
    copy: 'Launch sequence authoring, base command, and advanced runtime policy.',
  },
  {
    id: 'mods',
    label: 'Mods',
    copy: 'Optional mod source wiring for custom game definitions.',
  },
  {
    id: 'config',
    label: 'Config Files',
    copy: 'Managed configuration files and schema definitions for game servers.',
  },
]

const activeFormTab = ref<GameFormTabID>(readActiveFormTabFromHistory())
const activePlatform = ref<Platform>('windows')
const runtimeSequenceExpanded = ref(false)
const runtimePolicyExpanded = ref(false)
function focusFormTab(tabID: GameFormTabID): void {
  void nextTick(() => {
    const tabElement = document.getElementById(formTabID(tabID))
    if (!(tabElement instanceof HTMLButtonElement)) {
      return
    }

    tabElement.focus()
  })
}

function cycleFormTab(fromTabID: GameFormTabID, step: number): void {
  const currentIndex = formTabs.findIndex((tab) => tab.id === fromTabID)
  if (currentIndex === -1) {
    return
  }

  const nextIndex = (currentIndex + step + formTabs.length) % formTabs.length
  const nextTabID = formTabs[nextIndex].id
  activeFormTab.value = nextTabID
  focusFormTab(nextTabID)
}

function handleFormTabKeydown(event: KeyboardEvent, tabID: GameFormTabID): void {
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    event.preventDefault()
    cycleFormTab(tabID, 1)
    return
  }

  if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    event.preventDefault()
    cycleFormTab(tabID, -1)
    return
  }

  if (event.key === 'Home') {
    event.preventDefault()
    const firstTabID = formTabs[0].id
    activeFormTab.value = firstTabID
    focusFormTab(firstTabID)
    return
  }

  if (event.key === 'End') {
    event.preventDefault()
    const lastTabID = formTabs[formTabs.length - 1].id
    activeFormTab.value = lastTabID
    focusFormTab(lastTabID)
  }
}

function isCompactRuntimeViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }

  return window.matchMedia('(max-width: 900px)').matches
}

function toggleRuntimePolicy(): void {
  const nextValue = !runtimePolicyExpanded.value
  runtimePolicyExpanded.value = nextValue
  if (nextValue && isCompactRuntimeViewport()) {
    runtimeSequenceExpanded.value = false
  }
}

function updateRuntimeSequenceExpanded(value: boolean): void {
  runtimeSequenceExpanded.value = value
  if (value && isCompactRuntimeViewport()) {
    runtimePolicyExpanded.value = false
  }
}

function formTabID(tabID: GameFormTabID): string {
  return `game-form-tab-${tabID}`
}

function formTabPanelID(tabID: GameFormTabID): string {
  return `game-form-tab-panel-${tabID}`
}

function isGameFormTabID(value: unknown): value is GameFormTabID {
  return value === 'overview' || value === 'runtime' || value === 'mods' || value === 'config'
}

function readActiveFormTabFromHistory(): GameFormTabID {
  if (typeof window === 'undefined') {
    return 'overview'
  }

  const historyTab = window.history.state?.[gameFormTabHistoryStateKey]
  return isGameFormTabID(historyTab) ? historyTab : 'overview'
}

function persistActiveFormTabToHistory(tabID: GameFormTabID): void {
  if (typeof window === 'undefined') {
    return
  }

  window.history.replaceState(
    {
      ...(window.history.state ?? {}),
      [gameFormTabHistoryStateKey]: tabID,
    },
    '',
  )
}

// Resolved platform accounts for single-platform cases
const activePlatformResolved = computed<Platform | null>(() => {
  if (game.value.windowsSupport && game.value.linuxSupport) {
    return activePlatform.value
  }
  if (game.value.windowsSupport) return 'windows'
  if (game.value.linuxSupport) return 'linux'
  return null
})

const formTitle = computed(() => {
  if (existingGame.value) {
    return `Editing ${game.value.name || 'Game'}`
  }
  return 'Add Game'
})

const breadcrumbLabel = computed(() => {
  if (existingGame.value) {
    return game.value.name || 'Game'
  }
  if (copyGame.value) {
    return 'New Game (Copy)'
  }
  return 'New Game'
})

// --- Validation rules ---

const idRules = [
  (val: string) => (val && val.trim().length > 0) || 'Unique ID is required',
  (val: string) =>
    /^[a-z0-9_-]+$/.test(val) || 'Only lowercase letters, numbers, hyphens, and underscores',
]

const nameRules = [(val: string) => (val && val.trim().length > 0) || 'Name is required']

const portRules = [
  (val: number | null | string) =>
    (val !== null && val !== '' && val !== undefined) || 'Port is required',
  (val: number | null | string) => {
    const num = Number(val)
    return (Number.isInteger(num) && num >= 1 && num <= 65535) || 'Must be 1-65535'
  },
]

const commandTypeOptions = getCommandTypeOptions()
const linuxCommandProcessorOptions = getCommandProcessorOptions('linux')
const windowsCommandProcessorOptions = getCommandProcessorOptions('windows')

const props = defineProps({
  existingGameId: {
    type: String,
    required: false,
    default: '',
  },
  copyGameId: {
    type: String,
    required: false,
    default: '',
  },
})

const game: Ref<Game> = ref(create(GameSchema)) as Ref<Game>
const existingGame = ref(false)
const copyGame = ref(false)
const gameID = ref('')
const configSchemas = ref<ConfigSchemaEntry[]>([])
const baselineLinuxBaseCommand = ref('')
const baselineWindowsBaseCommand = ref('')
const baselineLinuxStartArgsTemplate = ref<StartArgBlock[]>([])
const baselineWindowsStartArgsTemplate = ref<StartArgBlock[]>([])
const linuxStartArgsTemplate = ref<StartArgBlock[]>([])
const windowsStartArgsTemplate = ref<StartArgBlock[]>([])
const startArgBlocklist = ref<StartArgBlocklistEntry[]>([])
const downstreamImpactServers = ref<Array<{ name: string; patchCount: number }>>([])
const modSourceOptions = getModSourceOptions()
const managedTypedConfig = computed(() => isManagedGameConfig(game.value))
const managedModConfig = computed(() => isManagedModConfig(game.value))
const activeModSourceLabel = computed(() => {
  const sourceID = game.value.modProfile?.sources[0]?.id
  if (!sourceID) {
    return 'No provider'
  }

  return modSourceOptions.find((option) => option.value === sourceID)?.label ?? sourceID
})
const runtimePolicySummary = computed(() => {
  const summary = [
    game.value.allowStartArgEditing ? 'Owners on' : 'Owners off',
    `${startArgBlocklist.value.length} reserved`,
  ]

  if (existingGame.value && !copyGame.value) {
    summary.push(`${downstreamImpactServers.value.length} affected`)
  }

  return summary
})

const runtimePolicyAssistiveSummary = computed(
  () => `Runtime guardrails. ${runtimePolicySummary.value.join('. ')}.`,
)

watch(
  activeFormTab,
  (tabID) => {
    persistActiveFormTabToHistory(tabID)
  },
  { immediate: true },
)

function ensureTypedGameConfig(): void {
  if (!game.value.updateProvider) {
    game.value.updateProvider = create(UpdateProviderConfigSchema, {})
  }
  if (!Array.isArray(game.value.variants)) {
    game.value.variants = []
  }
  if (game.value.modProfile && game.value.modProfile.sources.length === 0) {
    game.value.modProfile.sources.push(createEmptyModSource())
  }
}

function createEmptyModProfile() {
  return create(ModProfileSchema, {
    installPath: '',
    sources: [createEmptyModSource()],
  })
}

function createEmptyModSource() {
  return create(ModSourceSchema, {
    id: 'modrinth',
    searchParamsJson: '',
  })
}

function onModSourceProviderChanged(source: ModSource): void {
  source.searchParamsJson = ''
}

function readModSourceDisplayValue(source: ModSource): string {
  return readModSourcePrimaryValue(source.id, source.searchParamsJson)
}

function updateModSourceDisplayValue(source: ModSource, value: string | number | null): void {
  const nextValue = typeof value === 'string' ? value : value == null ? '' : String(value)
  source.searchParamsJson = writeModSourcePrimaryValue(
    source.id,
    source.searchParamsJson,
    nextValue,
  )
}

function addGameModProfile(): void {
  game.value.modProfile = createEmptyModProfile()
}

function clearGameModProfile(): void {
  game.value.modProfile = undefined
}

function syncSimpleGameConfig(): void {
  ensureTypedGameConfig()
  if (managedTypedConfig.value) {
    return
  }
  applySimpleGameConfig(game.value)
}

function syncStructuredStartArgsFromGame(): void {
  linuxStartArgsTemplate.value = parseStartArgsTemplate(game.value.linuxStartArgsTemplate)
  windowsStartArgsTemplate.value = parseStartArgsTemplate(game.value.windowsStartArgsTemplate)
  startArgBlocklist.value = parseStartArgBlocklist(game.value.startArgBlocklist)
}

function captureRuntimeBaselineFromCurrentState(): void {
  baselineLinuxBaseCommand.value = game.value.linuxBaseCommand
  baselineWindowsBaseCommand.value = game.value.windowsBaseCommand
  baselineLinuxStartArgsTemplate.value = cloneStartArgTemplate(linuxStartArgsTemplate.value)
  baselineWindowsStartArgsTemplate.value = cloneStartArgTemplate(windowsStartArgsTemplate.value)
}

function syncStructuredStartArgsToGame(): void {
  game.value.linuxStartArgsTemplate = serializeStartArgsTemplate(linuxStartArgsTemplate.value)
  game.value.windowsStartArgsTemplate = serializeStartArgsTemplate(windowsStartArgsTemplate.value)
  game.value.startArgBlocklist = serializeStartArgBlocklist(startArgBlocklist.value)
}

function commandTypeSummary(commandType: CommandType, operation: 'install' | 'update'): string {
  switch (commandType) {
    case CommandType.NONE:
      return `No ${operation} step will run for this platform.`
    case CommandType.STEAMCMD:
      return `Xylona will generate the SteamCMD ${operation} command automatically from the Steam App ID.`
    case CommandType.PAPERMC:
      return `Xylona will handle the PaperMC ${operation} flow internally.`
    case CommandType.MOJANG:
      return `Xylona will handle the Mojang ${operation} flow internally.`
    default:
      return `Use a shell command for this ${operation} step.`
  }
}

// --- Command syntax highlighting ---

function escapeHTML(str: string): string {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightCommand(cmd: string): string {
  if (!cmd) return ''
  const escaped = escapeHTML(cmd)
  return escaped.replace(/(\S+)/g, (token) => {
    // Flags: starts with - or + (JVM flags like -XX:+UseZGC)
    if (/^[-+]/.test(token)) {
      return `<span class="cmd-hl-flag">${token}</span>`
    }
    // Known binaries / jar files
    if (
      /\.(jar|exe|sh|bat)$/i.test(token) ||
      /^(java|steamcmd|python|node|bash|sh|cmd)$/i.test(token)
    ) {
      return `<span class="cmd-hl-binary">${token}</span>`
    }
    return token
  })
}

// --- Dirty tracking via snapshot comparison ---

function takeSnapshot(): string {
  return JSON.stringify(
    {
      game: game.value,
      defaultPort: defaultPort.value,
      defaultQueryPort: defaultQueryPort.value,
      configSchemas: configSchemas.value,
      linuxStartArgsTemplate: linuxStartArgsTemplate.value,
      windowsStartArgsTemplate: windowsStartArgsTemplate.value,
      startArgBlocklist: startArgBlocklist.value,
      downstreamImpactServers: downstreamImpactServers.value,
    },
    (_key, value) => (typeof value === 'bigint' ? value.toString() : value),
  )
}

function cloneStartArgTemplate(template: StartArgBlock[]): StartArgBlock[] {
  return template.map((block) => ({
    ...block,
    tokens: [...block.tokens],
  }))
}

const isDirty = computed(() => {
  if (!initialSnapshot) return false
  return takeSnapshot() !== initialSnapshot
})

// --- Expose dirty state for route-level navigation guards ---

defineExpose({
  isDirty,
  savedSuccessfully,
})

// --- Data loading ---

async function getGameDetailsFromID() {
  loading.value = true
  const request: GetGameRequest = create(GetGameRequestSchema, {})
  try {
    request.id = gameID.value
    const response: GetGameResponse = await GetXylonaClient().getGame(request)
    if (response.game === undefined) {
      return
    }
    game.value = response.game
    ensureTypedGameConfig()
    defaultPort.value = Number(response.game.defaultPort)
    defaultQueryPort.value = Number(response.game.defaultQueryPort)
    if (response.game.configSchemas) {
      try {
        configSchemas.value = JSON.parse(response.game.configSchemas) as ConfigSchemaEntry[]
      } catch {
        configSchemas.value = []
      }
    }
    if (copyGame.value) {
      game.value.id = ''
      game.value.name = `${game.value.name} (Copy)`
    }
    syncStructuredStartArgsFromGame()
    captureRuntimeBaselineFromCurrentState()
    if (existingGame.value && !copyGame.value) {
      await loadDownstreamImpact(response.game.id)
    } else {
      downstreamImpactServers.value = []
    }
    // Set initial platform tab based on loaded data
    if (game.value.windowsSupport) {
      activePlatform.value = 'windows'
    } else if (game.value.linuxSupport) {
      activePlatform.value = 'linux'
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: `Failed to load game: ${ConnectErrorToString(err)}`,
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loading.value = false
    // Snapshot after load settles so isDirty compares against the loaded state
    await nextTick()
    initialSnapshot = takeSnapshot()
  }
}

async function loadDownstreamImpact(gameId: string) {
  try {
    const response: ListGameServersResponse = await GetXylonaClient().listGameServers(
      create(ListGameServersRequestSchema, {}),
    )
    downstreamImpactServers.value = response.gameServers
      .filter((server) => server.gameId === gameId)
      .map((server) => ({
        name: server.name,
        patchCount: parseStartArgsPatches(server.startArgsPatches).length,
      }))
  } catch (unknownErr: unknown) {
    downstreamImpactServers.value = []
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-warning',
      caption: `Failed to load downstream impact: ${ConnectErrorToString(err)}`,
      position: 'top',
      timeout: 3500,
    })
  }
}

onMounted(async () => {
  // Sticky header detection
  if (stickySentinel.value) {
    stickyObserver = new IntersectionObserver(
      ([entry]) => {
        isStuck.value = !entry.isIntersecting
      },
      { threshold: 0 },
    )
    stickyObserver.observe(stickySentinel.value)
  }

  if (props.existingGameId !== '') {
    existingGame.value = true
    gameID.value = props.existingGameId
  }
  if (props.copyGameId !== '') {
    copyGame.value = true
    gameID.value = props.copyGameId
  }
  if (existingGame.value || copyGame.value) {
    await getGameDetailsFromID()
  } else {
    // New game — check for wizard pre-fill state
    const wizardState = history.state?.wizardState
    if (wizardState) {
      game.value.name = wizardState.name || ''
      game.value.id = wizardState.slug || ''
      game.value.steamAppid = wizardState.steamAppId || ''
      game.value.usesSteamcmd = wizardState.usesSteamcmd ?? false
      game.value.windowsSupport = wizardState.windowsSupport ?? false
      game.value.linuxSupport = wizardState.linuxSupport ?? false
      if (wizardState.installCommand) {
        if (game.value.linuxSupport) game.value.linuxInstallCommand = wizardState.installCommand
        if (game.value.windowsSupport) game.value.windowsInstallCommand = wizardState.installCommand
      }
      if (wizardState.updateCommand) {
        if (game.value.linuxSupport) game.value.linuxUpdateCommand = wizardState.updateCommand
        if (game.value.windowsSupport) game.value.windowsUpdateCommand = wizardState.updateCommand
      }
      if (wizardState.linuxBaseCommand) {
        game.value.linuxBaseCommand = wizardState.linuxBaseCommand
      }
      if (wizardState.windowsBaseCommand) {
        game.value.windowsBaseCommand = wizardState.windowsBaseCommand
      }
      if (wizardState.linuxStartArgsTemplate) {
        game.value.linuxStartArgsTemplate = wizardState.linuxStartArgsTemplate
      }
      if (wizardState.windowsStartArgsTemplate) {
        game.value.windowsStartArgsTemplate = wizardState.windowsStartArgsTemplate
      }
      ensureTypedGameConfig()
      syncSimpleGameConfig()
      syncStructuredStartArgsFromGame()
      captureRuntimeBaselineFromCurrentState()
      downstreamImpactServers.value = []
      // Set platform tab to first enabled
      if (game.value.windowsSupport) activePlatform.value = 'windows'
      else if (game.value.linuxSupport) activePlatform.value = 'linux'
    }
    ensureTypedGameConfig()
    syncStructuredStartArgsFromGame()
    captureRuntimeBaselineFromCurrentState()
    // Snapshot after pre-fill settles
    await nextTick()
    initialSnapshot = takeSnapshot()
  }
})

onBeforeUnmount(() => {
  stickyObserver?.disconnect()
})

function syncConfigSchemas() {
  game.value.configSchemas =
    configSchemas.value.length > 0 ? JSON.stringify(configSchemas.value) : ''
}

async function submit() {
  const valid = await formRef.value?.validate()
  if (!valid) {
    $q.notify({
      type: 'xylona-error',
      caption: 'Please fix the validation errors before saving.',
      position: 'top',
      timeout: 3000,
    })
    return
  }

  submitting.value = true
  syncConfigSchemas()
  syncSimpleGameConfig()
  try {
    if (existingGame.value) {
      await updateExistingGame()
    } else {
      await addNewGame()
    }
  } finally {
    submitting.value = false
  }
}

function handleCancel() {
  // The onBeforeRouteLeave guard handles the unsaved changes prompt
  router.back()
}

async function navigateToSchemaEditor(fileIndex: number) {
  const id = existingGame.value ? gameID.value : ''
  if (!id) return

  // Persist current config schemas before navigating so the editor can load them
  try {
    const request = create(UpdateGameConfigSchemasRequestSchema, {
      gameId: id,
      configSchemasJson: JSON.stringify(configSchemas.value),
    })
    await GetXylonaClient().updateGameConfigSchemas(request)
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: `Failed to save schemas before editing: ${ConnectErrorToString(err)}`,
      position: 'top',
      timeout: 5000,
    })
    return
  }

  // Schema navigation is intentional, skip the dirty guard
  initialSnapshot = takeSnapshot()
  await router.push({ path: `/games/${id}/config-schema/${fileIndex}` })
}

async function addNewGame() {
  const request: AddGameRequest = create(AddGameRequestSchema, {})
  game.value.steamAppid = normalizeSteamAppID(game.value.steamAppid)
  syncSimpleGameConfig()
  syncStructuredStartArgsToGame()
  request.game = game.value
  request.game.defaultPort = BigInt(defaultPort.value ?? 0)
  request.game.defaultQueryPort = BigInt(defaultQueryPort.value ?? 0)
  try {
    const response = await GetXylonaClient().addGame(request)
    const savedGameID = response.game?.id || request.game.id || game.value.id
    savedSuccessfully.value = true
    captureRuntimeBaselineFromCurrentState()
    initialSnapshot = takeSnapshot()
    $q.notify({
      caption: `${game.value.name} added successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    if (savedGameID) {
      await router.push({ path: `/games/${savedGameID}/edit` })
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error adding game: ${ConnectErrorToString(err)}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  }
}

async function updateExistingGame() {
  const request: EditGameRequest = create(EditGameRequestSchema, {})
  game.value.steamAppid = normalizeSteamAppID(game.value.steamAppid)
  syncSimpleGameConfig()
  syncStructuredStartArgsToGame()
  request.game = game.value as Game
  request.game.defaultPort = BigInt(defaultPort.value ?? 0)
  request.game.defaultQueryPort = BigInt(defaultQueryPort.value ?? 0)
  try {
    await GetXylonaClient().editGame(request)
    savedSuccessfully.value = true
    captureRuntimeBaselineFromCurrentState()
    initialSnapshot = takeSnapshot()
    $q.notify({
      caption: `${game.value.name} updated successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error updating game: ${ConnectErrorToString(err)}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  }
}
</script>

<style scoped>
.game-form-wrapper {
  width: 100%;
  --game-form-sticky-stack-offset: calc(50px + 4rem);
}

/* ---- Header ---- */

.game-form-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-md);
  padding: calc(var(--xy-space-sm) + 2px) var(--xy-space-lg) calc(var(--xy-space-xs) + 2px);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  border-radius: 8px 8px 0 0;
  position: sticky;
  top: var(--xy-toolbar-height);
  z-index: 10;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}

.game-form-header.is-stuck {
  border-bottom: 2px solid var(--xy-accent);
  box-shadow: var(--xy-shadow-md);
  border-radius: 0;
}

.sticky-sentinel {
  height: 0;
  overflow: hidden;
}

.game-form-header-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.structured-start-stack {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
}

.runtime-policy-panel {
  display: flex;
  flex-direction: column;
  gap: 0;
  border-radius: 18px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-gradient-subtle), var(--xy-surface-1);
  box-shadow: var(--xy-shadow-md);
  overflow: hidden;
}

.runtime-policy-toggle {
  width: 100%;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  padding: 1rem 1rem 0.9rem;
  border: none;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.runtime-policy-toggle-copy {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

.runtime-policy-eyebrow {
  font-size: 0.74rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.runtime-policy-summary-line {
  max-width: 38rem;
  font-size: 0.84rem;
  line-height: 1.45;
  color: color-mix(in srgb, var(--xy-accent) 12%, var(--xy-text-secondary) 88%);
}

.runtime-policy-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 0.65rem;
}

.runtime-policy-toggle-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.42rem;
  min-height: 2.15rem;
  padding: 0.38rem 0.78rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 30%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-0) 92%);
  color: color-mix(in srgb, var(--xy-accent) 34%, var(--xy-text-primary) 66%);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast),
    transform 120ms ease-out,
    box-shadow var(--xy-transition-fast);
}

.runtime-policy-toggle:hover .runtime-policy-toggle-indicator {
  border-color: color-mix(in srgb, var(--xy-accent) 42%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 11%, var(--xy-surface-0) 89%);
}

.runtime-policy-toggle:active .runtime-policy-toggle-indicator {
  transform: translateY(1px);
}

.runtime-policy-toggle-indicator :deep(.q-icon) {
  transition: transform 180ms ease-out;
}

.runtime-policy-toggle:hover .runtime-policy-toggle-indicator :deep(.q-icon) {
  transform: scale(1.08);
}

@media (prefers-reduced-motion: reduce) {
  .runtime-policy-panel-enter-active,
  .runtime-policy-panel-leave-active,
  .runtime-policy-toggle-indicator,
  .runtime-policy-toggle-indicator :deep(.q-icon) {
    transition: none;
  }
}

.runtime-policy-toggle:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--xy-accent) 62%, transparent);
  outline-offset: 2px;
}

.runtime-policy-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.runtime-policy-content {
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  padding: 0 clamp(16px, 1.7vw, 22px) clamp(16px, 1.7vw, 22px);
  border-top: 1px solid color-mix(in srgb, var(--xy-border) 82%, transparent);
}

.runtime-policy-panel-enter-active,
.runtime-policy-panel-leave-active {
  transition:
    opacity 180ms cubic-bezier(0.25, 1, 0.5, 1),
    transform 240ms cubic-bezier(0.22, 1, 0.36, 1);
}

.runtime-policy-panel-enter-from,
.runtime-policy-panel-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.runtime-policy-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(240px, 292px);
  gap: 0.8rem;
  padding-top: 0.85rem;
}

.runtime-policy-rail {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.runtime-policy-subsection {
  display: flex;
  flex-direction: column;
  gap: 0.62rem;
  padding: 0.82rem;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 72%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 66%, transparent);
}

.runtime-policy-subsection--impact {
  border-color: color-mix(in srgb, var(--xy-info) 20%, var(--xy-border) 80%);
  background: color-mix(in srgb, var(--xy-info) 4%, var(--xy-surface-0) 96%);
}

.runtime-policy-subsection--reserved {
  min-width: 0;
  border-color: color-mix(in srgb, var(--xy-accent) 18%, var(--xy-border) 82%);
  background: color-mix(in srgb, var(--xy-accent) 3%, var(--xy-surface-0) 97%);
}

.runtime-policy-subhead {
  color: var(--xy-text-primary);
  font-size: 0.88rem;
}

.runtime-policy-subcopy {
  font-size: 0.8rem;
  line-height: 1.4;
}

.runtime-policy-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.9rem;
  padding: 0.8rem 0.85rem;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 16%, var(--xy-border) 84%);
  background: color-mix(in srgb, var(--xy-accent) 3%, var(--xy-surface-1) 97%);
}

.runtime-policy-rail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.9rem;
}

.runtime-policy-switch-copy {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.runtime-policy-switch-title {
  color: var(--xy-text-primary);
  font-size: 0.92rem;
  font-weight: 600;
}

.runtime-policy-switch-note {
  font-size: 0.82rem;
  line-height: 1.4;
}

.runtime-policy-mini-note {
  font-size: 0.75rem;
  line-height: 1.4;
}

.runtime-policy-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.9rem;
}

.runtime-policy-card-head--stacked {
  align-items: center;
}

.runtime-policy-card-copy {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

.runtime-policy-quantity {
  display: inline-flex;
  align-items: center;
  min-height: 1.75rem;
  padding: 0.16rem 0.58rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 32%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-accent) 24%, var(--xy-text-primary) 76%);
  font-size: 0.72rem;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .runtime-policy-toggle {
    flex-direction: column;
    gap: 0.85rem;
  }

  .runtime-policy-header-actions {
    width: 100%;
    align-items: flex-start;
  }

  .runtime-policy-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .runtime-policy-switch-row,
  .runtime-policy-rail-head,
  .runtime-policy-card-head {
    flex-direction: column;
    align-items: flex-start;
  }
}

.game-form-breadcrumbs {
  font-size: 0.76rem;
  line-height: 1;
}

.breadcrumb-list {
  display: flex;
  align-items: center;
  gap: 4px;
  list-style: none;
  margin: 0;
  padding: 0;
}

.breadcrumb-list li + li::before {
  content: '/';
  color: var(--xy-text-muted);
  opacity: 0.5;
  margin-right: 4px;
}

.breadcrumb-link {
  color: var(--xy-text-muted);
  text-decoration: none;
  transition: color var(--xy-transition-fast);
}

.breadcrumb-link:hover {
  color: var(--xy-accent);
}

.breadcrumb-current {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.game-form-title {
  margin: 0;
  font-size: 1.12rem;
  font-weight: 600;
  line-height: 1.1;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.game-form-guided-link {
  color: var(--xy-accent);
  margin-right: 8px;
  text-decoration: none;
}

.game-form-header-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.5rem;
  flex-shrink: 0;
  padding-top: 0;
}

/* ---- Loading ---- */

.game-form-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
}

/* ---- Body ---- */

.game-form-body {
  padding: var(--xy-space-xs) var(--xy-space-lg) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border-radius: 0 0 8px 8px;
}

.game-form-tabs-panel {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  margin-bottom: var(--xy-space-sm);
  padding-top: 0;
}

.game-form-tabs-shell {
  overflow-x: auto;
  border-bottom: 1px solid var(--xy-border);
}

.game-form-tabs {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  min-width: max-content;
}

.game-form-tab {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 2px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--xy-text-muted);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.game-form-tab:hover {
  color: var(--xy-text-secondary);
}

.game-form-tab--active {
  color: var(--xy-text-primary);
  border-bottom-color: var(--xy-primary);
}

.game-form-tab__label {
  font-size: 0.8rem;
  letter-spacing: 0.04em;
  text-transform: none;
}

.game-form-tab-panel {
  min-width: 0;
}

.game-form-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* ---- Sections ---- */

.form-section {
  padding-top: var(--xy-space-lg);
  border-bottom: 1px solid var(--xy-border);
  padding-bottom: var(--xy-space-lg);
}

.form-section:first-child {
  padding-top: var(--xy-space-sm);
}

.form-section--compact {
  padding-top: var(--xy-space-sm);
  padding-bottom: var(--xy-space-md);
}

.form-section--last {
  border-bottom: none;
  padding-bottom: 0;
}

/* ---- Overview Metadata Strip ---- */

.overview-metadata {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--xy-space-lg);
}

@media (max-width: 900px) {
  .overview-metadata {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-md);
  }
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
}

.section-bar {
  width: 3px;
  height: 16px;
  border-radius: 2px;
  flex-shrink: 0;
}

.section-title {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--xy-text-secondary);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  white-space: nowrap;
}

.section-line {
  flex: 1;
  height: 1px;
  background: var(--xy-border);
  margin-left: var(--xy-space-xs);
}

/* ---- Feature Chips ---- */

.feature-groups {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-lg);
}

.feature-group {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.feature-group-label {
  font-size: 0.62rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.feature-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: stretch;
  gap: var(--xy-space-sm);
}

.feature-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 44px;
  padding: 10px 14px;
  border-radius: 6px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  cursor: pointer;
  transition:
    background var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast),
    opacity var(--xy-transition-fast);
  color: var(--xy-text-muted);
  font-size: 0.8rem;
  font-family: inherit;
  line-height: 1.2;
  opacity: 0.7;
}

.feature-chip:hover {
  border-color: var(--xy-border-hover);
  opacity: 1;
}

.feature-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--xy-text-muted);
  opacity: 0.4;
  transition:
    background var(--xy-transition-fast),
    opacity var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
  flex-shrink: 0;
}

.feature-chip--active {
  background: var(--xy-primary-bg-subtle);
  border-color: var(--xy-primary-border-soft);
  color: var(--xy-text-primary);
  opacity: 1;
}

.feature-chip--active .feature-dot {
  background: var(--xy-primary);
  opacity: 1;
  box-shadow: 0 0 6px var(--xy-primary-glow-soft);
}

/* ---- Platform Tabs ---- */

.platform-empty {
  font-size: 0.82rem;
  padding: var(--xy-space-md);
  text-align: center;
  background: var(--xy-surface-0);
  border-radius: 8px;
  border: 1px solid var(--xy-border);
}

.platform-tabs {
  display: inline-flex;
  background: var(--xy-surface-0);
  border-radius: 8px;
  padding: 3px;
  gap: 2px;
  margin-bottom: var(--xy-space-md);
}

.platform-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 44px;
  padding: 6px 16px;
  border-radius: 6px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  transition:
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
  font-family: inherit;
}

.platform-tab:hover {
  color: var(--xy-text-secondary);
}

.platform-tab--active {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
}

.platform-icon-inactive {
  color: var(--xy-text-muted);
}

.platform-icon-windows-active {
  color: var(--xy-platform-windows);
}

.platform-icon-linux-active {
  color: var(--xy-platform-linux);
}

/* ---- Command Blocks ---- */

.platform-commands {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.cmd-block {
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 0 8px 8px 0;
  overflow: hidden;
  transition: border-left-color var(--xy-transition-fast);
}

.cmd-block--windows {
  border-left: 3px solid var(--xy-platform-windows);
}

.cmd-block--windows:hover {
  border-left-color: var(--xy-platform-windows-hover);
}

.cmd-block--linux {
  border-left: 3px solid var(--xy-platform-linux);
}

.cmd-block--linux:hover {
  border-left-color: var(--xy-platform-linux-hover);
}

.cmd-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding: 6px 12px;
  border-bottom: 1px solid var(--xy-border);
}

.cmd-label {
  font-size: 0.72rem;
  color: var(--xy-text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-weight: 500;
}

.cmd-badge {
  font-size: 0.6rem;
  letter-spacing: 0.04em;
  border: 1px solid var(--xy-border);
  padding: 1px 6px;
  border-radius: 3px;
}

.cmd-type-select {
  appearance: none;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  color: var(--xy-text-secondary);
  font-size: 0.65rem;
  min-height: 32px;
  padding: 2px 24px 2px 8px;
  cursor: pointer;
  outline: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%23979b9e' stroke-width='1.5' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;
  transition:
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
}

.cmd-type-select:hover {
  border-color: var(--xy-border-hover);
}

.cmd-type-select:focus {
  border-color: var(--xy-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--xy-primary) 24%, transparent);
}

.cmd-type-select option {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
}

.cmd-type-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.cmd-type-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.cmd-type-label {
  font-size: 0.6rem;
  color: var(--xy-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-weight: 500;
}

.cmd-input-wrap {
  position: relative;
}

.cmd-highlight {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 10px 12px;
  font-size: 0.82rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  pointer-events: none;
  overflow: hidden;
}

.cmd-textarea {
  display: block;
  width: 100%;
  background: transparent;
  border: none;
  color: transparent;
  caret-color: var(--xy-text-primary);
  font-size: 0.82rem;
  padding: 10px 12px;
  resize: vertical;
  outline: none;
  line-height: 1.5;
  position: relative;
  z-index: 1;
  transition: box-shadow var(--xy-transition-fast);
}

.cmd-textarea:focus-visible {
  box-shadow: inset 0 0 0 1px var(--xy-primary);
}

.cmd-textarea::placeholder {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

/* When textarea is empty, show placeholder text (not the highlight layer) */
.cmd-textarea:placeholder-shown {
  color: transparent;
}

.cmd-internal {
  padding: 10px 12px;
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  font-style: italic;
}

/* Working directory doesn't need highlighting — show text normally */
.cmd-block .cmd-textarea:only-child {
  color: var(--xy-text-primary);
}

/* Syntax highlight token colors */
:deep(.cmd-hl-flag) {
  color: var(--xy-accent);
}

:deep(.cmd-hl-binary) {
  color: var(--xy-warning);
}

/* ---- Section Help Text ---- */

.section-help {
  font-size: 0.78rem;
  margin-top: calc(var(--xy-space-md) * -0.5);
  margin-bottom: var(--xy-space-md);
  line-height: 1.5;
}

/* ---- Mods Layout ---- */

.mods-layout {
  display: grid;
  grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);
  gap: clamp(16px, 2vw, 26px);
  align-items: start;
}

.mods-layout-single {
  display: block;
}

.mods-rail {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.mods-rail-intro,
.mods-rail-signal {
  padding: clamp(14px, 1.6vw, 18px);
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 15%, var(--xy-border) 85%);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-info) 7%, transparent), transparent 64%),
    var(--xy-surface-0);
}

.mods-rail-eyebrow,
.mods-rail-signal-label {
  display: inline-flex;
  align-items: center;
  font-size: 0.7rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--xy-info) 58%, var(--xy-text-secondary) 42%);
}

.mods-rail-title {
  margin-top: 0.5rem;
  color: var(--xy-text-primary);
  font-size: 0.98rem;
  line-height: 1.28;
}

.mods-rail-copy,
.mods-rail-signal-copy {
  margin-top: 0.45rem;
  font-size: 0.8rem;
  line-height: 1.55;
}

.mods-rail-signals {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.mods-workspace {
  min-width: 0;
}

.mods-workspace-card {
  min-height: 100%;
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--xy-info) 12%, var(--xy-border) 88%);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-info) 4%, transparent), transparent 36%),
    var(--xy-surface-0);
}

.mods-workspace-header {
  gap: var(--xy-space-md);
}

.mods-status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.55rem;
}

.mods-status-chip {
  display: inline-flex;
  align-items: center;
  min-height: 1.95rem;
  padding: 0.28rem 0.72rem;
  border-radius: 999px;
  border: 1px solid var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-1) 86%, transparent);
  color: var(--xy-text-secondary);
  font-size: 0.72rem;
  line-height: 1;
}

.mods-status-chip--active {
  border-color: color-mix(in srgb, var(--xy-info) 28%, var(--xy-border) 72%);
  background: color-mix(in srgb, var(--xy-info) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-info) 32%, var(--xy-text-primary) 68%);
}

.mods-quickstart {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
  padding: 0.9rem 1rem;
  border: 1px dashed color-mix(in srgb, var(--xy-info) 22%, var(--xy-border) 78%);
  border-radius: 14px;
  background: color-mix(in srgb, var(--xy-info) 4%, var(--xy-surface-1) 96%);
}

.mods-quickstart-title {
  font-size: 0.8rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--xy-info) 42%, var(--xy-text-primary) 58%);
}

.mods-quickstart-copy,
.mods-next-step {
  font-size: 0.78rem;
  line-height: 1.55;
}

.mods-quickstart-steps {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.mods-quickstart-step {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.7rem;
  align-items: start;
}

.mods-quickstart-step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2.1rem;
  min-height: 2.1rem;
  padding: 0 0.45rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 22%, var(--xy-border) 78%);
  background: color-mix(in srgb, var(--xy-surface-0) 84%, transparent);
  font-size: 0.74rem;
  color: color-mix(in srgb, var(--xy-info) 48%, var(--xy-text-primary) 52%);
}

.mods-quickstart-step-copy {
  padding-top: 0.25rem;
  font-size: 0.78rem;
  line-height: 1.5;
  color: var(--xy-text-secondary);
}

.mods-next-step {
  padding: 0.82rem 0.95rem;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 18%, var(--xy-border) 82%);
  background: color-mix(in srgb, var(--xy-info) 5%, var(--xy-surface-1) 95%);
}

/* ---- Typed Game Config ---- */

.typed-config-grid {
  display: grid;
  gap: var(--xy-space-md);
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}

.typed-config-card {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 10px;
}

.typed-config-header {
  display: flex;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  align-items: flex-start;
}

.typed-config-title {
  font-size: 0.94rem;
  color: var(--xy-text-primary);
}

.typed-config-copy {
  font-size: 0.78rem;
  line-height: 1.45;
  margin-top: 0.15rem;
}

.typed-config-copy--secondary {
  max-width: 34rem;
}

.typed-config-fields {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.typed-config-managed {
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px dashed var(--xy-border);
  border-radius: 10px;
  line-height: 1.55;
}

.typed-config-fields--nested {
  padding: var(--xy-space-sm);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
}

.typed-subtitle {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--xy-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

/* ---- Preset Selector ---- */

.preset-selector {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  margin-bottom: var(--xy-space-md);
}

.preset-select {
  max-width: 400px;
}

.preset-description {
  font-size: 0.78rem;
  line-height: 1.4;
  padding-left: 2px;
}

@media (max-width: 700px) {
  .game-form-header {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm) var(--xy-space-md) calc(var(--xy-space-xs) + 2px);
  }

  .game-form-header-left {
    width: 100%;
  }

  .game-form-header-actions {
    width: 100%;
    padding-top: 0;
  }

  .game-form-title {
    font-size: 1.04rem;
    line-height: 1.15;
  }

  .game-form-body {
    padding: var(--xy-space-xs) var(--xy-space-md) var(--xy-space-md);
  }

  .game-form-tabs-shell {
    margin-inline: calc(var(--xy-space-xs) * -1);
    padding-inline: var(--xy-space-xs);
  }

  .game-form-tabs {
    gap: var(--xy-space-sm);
  }

  .game-form-tab {
    min-height: 44px;
  }

  .game-form-tab__label {
    font-size: 0.76rem;
  }

  .feature-groups {
    flex-direction: column;
    gap: var(--xy-space-md);
  }

  .feature-chips {
    display: grid;
    grid-template-columns: 1fr;
  }

  .feature-chip {
    width: 100%;
    justify-content: flex-start;
  }

  .mods-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .mods-workspace {
    order: -1;
  }

  .platform-tabs {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .platform-tab {
    width: 100%;
  }

  .cmd-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .cmd-type-row {
    width: 100%;
    gap: var(--xy-space-sm);
  }

  .cmd-type-select {
    min-height: 40px;
    font-size: 0.72rem;
    padding-inline: 10px;
  }
}
</style>
