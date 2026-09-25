<template>
  <div ref="mainArea" :class="{ 'main-area-expanded': consoleExpanded }" class="main-area">
    <!-- The layout's identity bar shows the name; this keeps the page's heading. -->
    <h1 class="xy-visually-hidden">{{ serverName ? `${serverName} console` : 'Console' }}</h1>
    <q-resize-observer @resize="onMainAreaResize" />
    <div
      :class="{ 'sidebar-backdrop-visible': !sidebarCollapsed }"
      aria-hidden="true"
      class="sidebar-backdrop"
      @click="sidebarDrawerOpen = false"></div>

    <!-- Sidebar -->
    <aside
      :aria-hidden="sidebarCollapsed"
      :class="{ collapsed: sidebarCollapsed }"
      :inert="sidebarCollapsed"
      aria-label="Server details"
      class="sidebar"
      @keydown="trapDrawerFocus">
      <div class="sidebar-mobile-header">
        <span>Server details</span>
        <q-btn
          ref="drawerCloseButton"
          aria-label="Hide server details"
          class="sidebar-mobile-close"
          dense
          flat
          icon="close"
          round
          @click="sidebarDrawerOpen = false" />
      </div>
      <div class="sidebar-header">
        <span class="sidebar-header__label">Server details</span>
        <q-btn
          aria-label="Collapse server details"
          class="console-toolbar-btn"
          dense
          flat
          icon="first_page"
          square
          @click="setSidePanelOpen('sidebar', false)">
          <q-tooltip>Collapse server details</q-tooltip>
        </q-btn>
      </div>
      <div class="sidebar-content">
        <!-- Controls -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Controls</div>
          <!-- Hints sit on a wrapper: a disabled button gets no hover, so its own tooltip never shows. -->
          <div class="server-controls">
            <span v-for="control in lifecycleControls" :key="control.label" class="server-control">
              <!-- Outline, like the topbar copies: "Start again" stays the one filled action. -->
              <q-btn
                :aria-label="control.ariaLabel"
                :color="control.color"
                :disable="control.disable"
                :label="control.label"
                :loading="control.loading"
                outline
                @click="control.run" />
              <q-tooltip v-if="control.hint">{{ control.hint }}</q-tooltip>
            </span>
            <span v-if="showUpdateButton" class="server-control">
              <q-btn
                :aria-label="lifecycleAriaLabel('Update', 'game_server.settings')"
                :disable="disableUpdateButton || !hasPermission('game_server.settings')"
                :loading="updatingServer"
                color="primary"
                label="Update"
                outline
                @click="updateGameServer" />
              <q-tooltip v-if="lifecycleHint('game_server.settings')">
                {{ lifecycleHint('game_server.settings') }}
              </q-tooltip>
            </span>
          </div>
          <div v-if="!serverStateAuthoritative" class="controls-hint" role="status">
            Waiting for server status — controls are paused until it is confirmed.
          </div>
        </div>

        <div v-if="readinessVisible" class="sidebar-section">
          <div class="sidebar-section-label">Readiness</div>
          <div class="readiness-list">
            <div
              v-for="item in visibleReadinessItems"
              :key="item.kind"
              :class="{ 'readiness-item--complete': item.complete }"
              class="readiness-item">
              <div class="readiness-item-icon">
                <q-icon :name="item.complete ? 'verified' : 'report_problem'" />
              </div>
              <div class="readiness-item-body">
                <div class="readiness-item-title">{{ readinessLabel(item.kind) }}</div>
                <div class="readiness-item-message">{{ item.message }}</div>
                <q-btn
                  v-if="
                    !item.complete &&
                    isConfigReadinessItem(item) &&
                    hasPermission('game_server.config')
                  "
                  :to="`/game-servers/${gameServerId}/configuration`"
                  class="readiness-action"
                  color="primary"
                  dense
                  icon="tune"
                  label="Open Configuration"
                  no-caps
                  outline />
                <q-btn
                  v-if="item.kind === 'minecraft_eula' && hasPermission('game_server.settings')"
                  :loading="acceptingMinecraftEula"
                  class="readiness-action"
                  color="primary"
                  dense
                  label="Accept EULA"
                  outline
                  @click="acceptMinecraftEula" />
                <div
                  v-if="item.kind === 'steam_gslt' && hasPermission('game_server.settings')"
                  class="readiness-secret-form">
                  <q-input
                    v-model="steamGSLT"
                    autocomplete="off"
                    class="readiness-secret-input"
                    dense
                    label="Steam GSLT"
                    outlined
                    type="password" />
                  <div class="readiness-secret-actions">
                    <q-btn
                      :disable="steamGSLT.trim() === ''"
                      :loading="savingSteamGSLT"
                      color="primary"
                      dense
                      label="Save Token"
                      outline
                      @click="saveSteamGSLT" />
                    <q-btn
                      v-if="item.complete"
                      :loading="clearingSteamGSLT"
                      color="negative"
                      dense
                      flat
                      label="Clear"
                      @click="clearSteamGSLT" />
                  </div>
                </div>
                <div
                  v-if="item.kind === 'hytale_account' && hasPermission('game_server.settings')"
                  class="readiness-secret-form">
                  <q-btn
                    v-if="!item.complete && hytaleFlowId === '' && hytaleProfiles.length === 0"
                    :loading="startingHytaleAuth"
                    color="primary"
                    dense
                    label="Link Account"
                    outline
                    @click="startHytaleDeviceAuth" />
                  <div
                    v-if="hytaleFlowId !== '' && hytaleProfiles.length === 0"
                    class="readiness-device-flow">
                    <div v-if="hytaleUserCode !== ''" class="readiness-device-code">
                      {{ hytaleUserCode }}
                    </div>
                    <a
                      v-if="hytaleVerificationLink !== ''"
                      :href="hytaleVerificationLink"
                      class="readiness-link"
                      rel="noopener noreferrer"
                      target="_blank">
                      Open Hytale authorization
                    </a>
                    <q-btn
                      :loading="pollingHytaleAuth"
                      color="primary"
                      dense
                      label="Check Status"
                      outline
                      @click="pollHytaleDeviceAuth" />
                  </div>
                  <div v-if="hytaleProfiles.length > 0" class="readiness-device-flow">
                    <q-select
                      v-model="selectedHytaleProfile"
                      :options="hytaleProfileOptions"
                      class="readiness-profile-select"
                      dense
                      emit-value
                      label="Profile"
                      map-options
                      outlined />
                    <q-btn
                      :disable="selectedHytaleProfile === ''"
                      :loading="selectingHytaleProfile"
                      color="primary"
                      dense
                      label="Use Profile"
                      outline
                      @click="selectHytaleProfile" />
                  </div>
                  <q-btn
                    v-if="item.complete"
                    :loading="clearingHytaleAccount"
                    color="negative"
                    dense
                    flat
                    label="Clear Link"
                    @click="clearHytaleAccount" />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Version -->
        <div v-if="showVersionSection || hasSoftwareOptions" class="sidebar-section">
          <div class="sidebar-section-label">Version</div>
          <div class="version-list">
            <div v-if="hasSoftwareOptions" class="version-item">
              <span class="cl-label">Software</span>
              <span class="version-software">
                <span class="cl-value-plain">{{ softwareDisplayName }}</span>
                <button
                  v-if="showChangeButton"
                  class="change-btn"
                  @click="softwareSelector?.openChangeDialog()">
                  Change
                  <span class="change-arrow">&rsaquo;</span>
                </button>
              </span>
            </div>
            <div class="version-item">
              <span class="cl-label">Installed</span>
              <span class="cl-value-plain">{{ versionSection.installedVersion || '—' }}</span>
            </div>
            <div v-if="versionSection.latestVersion" class="version-item">
              <span class="cl-label">Latest</span>
              <span class="cl-value-plain">{{ versionSection.latestVersion }}</span>
            </div>
            <div v-if="versionStatusBadge || versionMetaText" class="version-item version-footer">
              <span
                v-if="versionStatusBadge"
                :class="versionStatusBadge.cssClass"
                class="version-status">
                <span v-if="versionSection.state === 'update-available'" class="update-dot"></span>
                <q-icon v-else :name="versionStatusBadge.icon" size="13px" />
                {{ versionStatusBadge.label }}
              </span>
              <span v-if="versionMetaText" class="version-meta">{{ versionMetaText }}</span>
            </div>
          </div>
        </div>

        <!-- Connection -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Connection</div>
          <div class="connection-list">
            <div class="connection-item">
              <span class="cl-label">Address</span>
              <span class="cl-value">
                <clip-board-copy
                  :clip-board-value="connectionAddress"
                  :display-text="connectionAddress" />
              </span>
            </div>
          </div>
        </div>

        <!-- Resource Usage -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Resource Usage</div>
          <div :class="{ 'metrics-offline': !showLiveMetrics }" class="metrics-preview">
            <p v-if="!showLiveMetrics" class="metrics-offline-note">
              {{
                isServerProcessRunning
                  ? 'Waiting for live metrics…'
                  : 'No live metrics while the server is offline'
              }}
            </p>
            <!-- Compute -->
            <div class="metrics-group">
              <div class="metrics-group-label">Compute</div>
              <div>
                <div class="metric-row">
                  <span class="ml"
                    >CPU
                    <span class="metric-detail"
                      >({{ showLiveMetrics ? metricsCpuCores : '--' }} cores)</span
                    ></span
                  >
                  <span class="mv">{{ showLiveMetrics ? metricsCpu.toFixed(1) + '%' : '--' }}</span>
                </div>
                <div class="metric-bar">
                  <div
                    :class="cpuBarClass"
                    :style="{
                      transform: `scaleX(${showLiveMetrics ? Math.min(Math.max(metricsCpu / 100, 0), 1) : 0})`,
                    }"
                    class="metric-bar-fill"></div>
                </div>
              </div>
              <div class="metric-row">
                <span class="ml">Threads</span>
                <span class="mv">{{ showLiveMetrics ? metricsThreads : '--' }}</span>
              </div>
            </div>

            <!-- Memory -->
            <div class="metrics-group">
              <div class="metrics-group-label">Memory</div>
              <div>
                <div class="metric-row">
                  <span class="ml">Memory</span>
                  <span class="mv">{{ showLiveMetrics ? bytesToSize(metricsMemory) : '--' }}</span>
                </div>
                <div
                  v-if="metricsMaxMemory > 0"
                  class="metric-bar metric-bar--marked"
                  :title="`Java heap limit ${bytesToSize(metricsMaxMemory)}`">
                  <div
                    :class="memoryBarClass"
                    :style="{
                      transform: `scaleX(${showLiveMetrics ? metricsMemoryBarRatio : 0})`,
                    }"
                    class="metric-bar-fill"></div>
                  <span
                    aria-hidden="true"
                    class="metric-bar-marker"
                    :style="{ left: `${metricsHeapMarkerRatio * 100}%` }"></span>
                </div>
              </div>
              <div v-if="metricsMaxMemory > 0" class="metric-row">
                <span class="ml">Java heap limit</span>
                <span class="mv">{{ bytesToSize(metricsMaxMemory) }}</span>
              </div>
              <div v-if="showLiveMetrics && metricsMemoryPercent > 0" class="metric-row">
                <span class="ml">System RAM</span>
                <span class="mv">{{ metricsMemoryPercent.toFixed(1) }}%</span>
              </div>
            </div>

            <!-- Storage -->
            <div class="metrics-group">
              <div class="metrics-group-label">Storage</div>
              <div class="metric-row">
                <span class="ml">Disk Usage</span>
                <span class="mv">{{
                  showLiveMetrics && metricsDiskValid ? bytesToSize(metricsDisk) : '--'
                }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">I/O Read</span>
                <span class="mv">{{ showLiveMetrics ? formatRate(metricsIoReadRate) : '--' }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">I/O Write</span>
                <span class="mv">{{
                  showLiveMetrics ? formatRate(metricsIoWriteRate) : '--'
                }}</span>
              </div>
            </div>

            <!-- Network -->
            <div class="metrics-group">
              <div class="metrics-group-label">Network</div>
              <div class="metric-row">
                <span class="ml">Connections</span>
                <span class="mv">{{ showLiveMetrics ? metricsConnections : '--' }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">Uptime</span>
                <span class="mv">{{ showLiveMetrics ? formattedUptime : '--' }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <!-- Collapsed sidebar strip (desktop) -->
    <div v-if="sidebarCollapsed" class="sidebar-mini">
      <q-btn
        aria-label="Expand server details"
        class="console-toolbar-btn"
        dense
        flat
        icon="last_page"
        square
        @click="setSidePanelOpen('sidebar', true)">
        <q-tooltip>Expand server details</q-tooltip>
      </q-btn>
      <span class="sidebar-mini__label">Details</span>
    </div>

    <!-- Console wrapper -->
    <div :class="{ expanded: consoleExpanded }" class="console-wrapper">
      <div v-if="lastStartFailure" class="start-failure" role="alert">
        <q-icon aria-hidden="true" class="start-failure__icon" name="report_problem" />
        <div class="start-failure__body">
          <span class="start-failure__title">
            Start failed at {{ formatFailureTime(lastStartFailure.at) }}
          </span>
          <span class="start-failure__message">{{ lastStartFailure.message }}</span>
        </div>
        <div class="start-failure__actions">
          <q-btn
            :disable="disableStartButton || !hasPermission('game_server.start')"
            :loading="startingServer"
            color="primary"
            dense
            label="Start again"
            no-caps
            unelevated
            @click="startGameServer" />
          <q-btn
            aria-label="Dismiss start failure"
            dense
            flat
            icon="close"
            round
            @click="dismissStartFailure" />
        </div>
      </div>

      <!-- Console toolbar -->
      <div class="console-topbar">
        <div
          v-if="consoleFeedFilterOptions.length > 1 && !showConsolePlaceholder"
          aria-label="Console output filter"
          class="console-feed-filters"
          role="group">
          <span class="console-feed-filters__label">Filter</span>
          <button
            v-for="option in consoleFeedFilterOptions"
            :key="option.value"
            :aria-pressed="consoleFeedFilter === option.value"
            :class="{ 'console-feed-filter--active': consoleFeedFilter === option.value }"
            class="console-feed-filter"
            type="button"
            @click="consoleFeedFilter = option.value">
            {{ option.label }}
          </button>
          <span v-if="consoleFeedFilter !== 'all'" class="console-feed-filters__note">
            {{ visibleConsoleLines.length }} of {{ consoleLines.length }} lines
          </span>
        </div>
        <span v-else class="console-topbar__label">Console</span>
        <!-- Whenever the details sidebar is out of view, lifecycle controls sit here instead. -->
        <div
          v-if="showTopbarControls"
          aria-label="Server controls"
          class="console-lifecycle"
          role="group">
          <span v-for="control in lifecycleControls" :key="control.label" class="server-control">
            <q-btn
              :aria-label="control.ariaLabel ?? control.label"
              :color="control.color"
              :disable="control.disable"
              :icon="control.icon"
              :label="control.label"
              :loading="control.loading"
              dense
              no-caps
              no-wrap
              outline
              @click="control.run" />
            <q-tooltip>{{ control.hint || control.label }}</q-tooltip>
          </span>
        </div>
        <span class="console-topbar__spacer"></span>
        <q-btn
          v-if="isNarrow"
          ref="detailsButton"
          :aria-expanded="sidebarDrawerOpen ? 'true' : 'false'"
          class="console-toolbar-btn console-details-btn"
          dense
          flat
          icon="info_outline"
          label="Details"
          no-caps
          square
          @click="sidebarDrawerOpen ? (sidebarDrawerOpen = false) : openDetails()" />
        <q-btn
          :aria-pressed="consoleAutoScroll ? 'true' : 'false'"
          :class="{ 'console-toolbar-btn-off': !consoleAutoScroll }"
          :icon="consoleAutoScroll ? 'keyboard_double_arrow_down' : 'pause'"
          :text-color="consoleAutoScroll ? 'info' : undefined"
          aria-label="Auto scroll"
          class="console-toolbar-btn console-autoscroll-btn"
          dense
          flat
          label="Auto Scroll"
          no-caps
          square
          @click="toggleAutoScroll">
          <q-tooltip>
            {{
              consoleAutoScroll
                ? 'Auto scroll is on — click to stop sticking to new output'
                : 'Auto scroll is off — click to stick to the latest output'
            }}
          </q-tooltip>
        </q-btn>
        <q-btn
          :aria-label="consoleExpanded ? 'Exit fullscreen console' : 'Fullscreen console'"
          :icon="tabMaximize"
          class="console-toolbar-btn"
          dense
          flat
          square
          text-color="info"
          @click="consoleExpanded = !consoleExpanded">
          <q-tooltip>{{ consoleExpanded ? 'Exit fullscreen' : 'Fullscreen console' }}</q-tooltip>
        </q-btn>
      </div>
      <div
        v-if="showTopbarControls && !serverStateAuthoritative"
        class="controls-hint controls-hint--topbar"
        role="status">
        Waiting for server status — controls are paused until it is confirmed.
      </div>

      <div
        v-if="consoleStreamState !== 'ready' || consoleLoadError"
        :class="{
          'console-stream-state--error': consoleStreamState === 'error' || consoleLoadError,
        }"
        class="console-stream-state"
        :role="consoleStreamState === 'error' || consoleLoadError ? 'alert' : 'status'"
        aria-live="polite">
        <q-spinner
          v-if="consoleStreamState === 'loading' || consoleStreamState === 'reconnecting'"
          color="info"
          size="1rem" />
        <q-icon v-else name="sync_problem" size="sm" />
        <span v-if="consoleStreamState === 'loading'">Loading console output…</span>
        <span v-else-if="consoleStreamState === 'reconnecting'">
          Console connection interrupted. Reconnecting…
        </span>
        <span v-else>{{ consoleLoadError || 'Console output is unavailable.' }}</span>
        <q-btn
          v-if="consoleStreamState === 'error' || consoleLoadError"
          dense
          flat
          icon="refresh"
          label="Retry"
          @click="retryConsoleOutput" />
      </div>

      <!-- Console output -->
      <template v-if="showConsolePlaceholder">
        <div class="console-output console-output-offline">
          <div class="offline-placeholder">
            <div class="offline-icon">
              <q-icon :name="isServerStatusUnknown ? 'sync_problem' : 'power_settings_new'" />
            </div>
            <div class="offline-text">
              {{ isServerStatusUnknown ? 'Server status unavailable' : 'Server is offline' }}
            </div>
            <div class="offline-hint">{{ offlineHint }}</div>
            <q-btn
              v-if="!isServerStatusUnknown && setupBlocksStart"
              class="offline-details-btn"
              dense
              flat
              icon="info_outline"
              label="Show details"
              no-caps
              @click="openDetails" />
          </div>
        </div>
      </template>
      <template v-else>
        <div class="console-output-area" @scroll.capture="onConsoleScroll">
          <q-scroll-area id="consoleContainer" ref="consoleScrollArea" class="console-scroll-area">
            <div
              v-if="
                (isServerOffline || isServerStatusUnknown) &&
                !updateInProgress &&
                !softwareOperationInProgress
              "
              class="console-status-banner">
              {{ isServerStatusUnknown ? 'Server status unavailable.' : 'Server offline.' }}
            </div>
            <div v-if="consoleTruncated" class="console-truncated-notice">
              Earlier output truncated
            </div>
            <div v-if="filteredConsoleEmpty" class="console-filter-empty" role="status">
              <span>
                No {{ activeFilterLabel.toLowerCase() }} lines in the current buffer — showing 0 of
                {{ consoleLines.length }}.
              </span>
              <button
                class="console-filter-empty__reset"
                type="button"
                @click="consoleFeedFilter = 'all'">
                Show all
              </button>
            </div>
            <!-- A silent log: the throttled summary below speaks for it. -->
            <!-- eslint-disable vue/no-v-html -- authenticated game-server output is an accepted trust boundary -->
            <code
              id="consoleCodeEl"
              aria-label="Game server console output"
              aria-live="off"
              class="q-pb-md"
              role="log">
              <span v-for="line in visibleConsoleLines" :key="line.id" v-html="line.html"></span>
            </code>
            <!-- eslint-enable vue/no-v-html -->
          </q-scroll-area>
          <button
            v-if="unseenConsoleOutput"
            class="console-jump"
            type="button"
            @click="jumpToLatestOutput">
            New output
            <q-icon aria-hidden="true" name="arrow_downward" />
            Jump to latest
          </button>
        </div>
      </template>
      <span class="xy-visually-hidden" role="status">{{ consoleAnnouncement }}</span>

      <console-command-input
        v-model="serverInput"
        :commands="gameServer.game?.consoleCommands ?? []"
        :disabled="consoleInputDisabled"
        :disabled-reason="consoleInputDisabledReason"
        :game-name="gameServer.gameName"
        :loading="sendingConsoleInput"
        :permission-denied="!hasPermission('game_server.console')"
        @history="navigateConsoleInputHistory"
        @submit="sendGameServerInput" />
    </div>

    <!-- Player rail -->
    <aside
      :class="{ collapsed: playerRailCollapsed }"
      aria-label="Online players"
      class="player-rail">
      <template v-if="!playerRailCollapsed">
        <div class="player-rail__head">
          <span class="player-rail__title">Players</span>
          <span class="player-rail__head-actions">
            <q-btn
              v-if="hasPermission('game_server.players.manage')"
              :aria-label="
                gameServer.gameId === '7_days_to_die'
                  ? 'Open Player operations'
                  : 'Open player management'
              "
              class="console-toolbar-btn"
              dense
              flat
              icon="manage_accounts"
              square
              :to="
                gameServer.gameId === '7_days_to_die'
                  ? `/game-servers/${gameServerId}/operations`
                  : undefined
              "
              @click="openPlayerManagement">
              <q-tooltip>
                {{
                  gameServer.gameId === '7_days_to_die' ? 'Player operations' : 'Player management'
                }}
              </q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Collapse player panel"
              class="console-toolbar-btn"
              dense
              flat
              icon="last_page"
              square
              @click="setSidePanelOpen('players', false)">
              <q-tooltip>Collapse player panel</q-tooltip>
            </q-btn>
          </span>
        </div>
        <div class="player-rail__body">
          <p v-if="playerCount === null" role="status" class="q-ma-none text-caption text-xy-muted">
            {{ unknownPlayersMessage }}
          </p>
          <game-server-player-list
            v-else
            :can-manage-players="
              gameServer.gameId !== 'valheim' && hasPermission('game_server.players.manage')
            "
            :current-player-count="currentPlayerCount"
            :game-server-id="gameServerId"
            :is-online="isServerOnline"
            :max-player-count="displayedMaxPlayerCount"
            :native-identifiers-required="gameServer.gameId === '7_days_to_die'"
            :player-list-supported="playerListSupported"
            :player-names="onlinePlayers"
            :unlisted-player-count="unlistedPlayerCount" />
        </div>
      </template>
      <template v-else>
        <q-btn
          aria-label="Expand player panel"
          class="console-toolbar-btn"
          dense
          flat
          icon="first_page"
          square
          @click="setSidePanelOpen('players', true)">
          <q-tooltip>Expand player panel</q-tooltip>
        </q-btn>
        <span class="player-rail__mini-count font-mono">
          {{
            playerCount === null ? 'Unknown' : `${currentPlayerCount}/${displayedMaxPlayerCount}`
          }}
        </span>
      </template>
    </aside>
  </div>

  <game-server-player-management-dialog
    v-if="gameServer.gameId !== '7_days_to_die'"
    v-model="playerManagementOpen"
    :game-server-id="gameServerId" />

  <!-- ServerSoftwareSelector (renders its own dialog, kept mounted for ref access) -->
  <server-software-selector
    v-if="gameServer.gameId !== ''"
    ref="softwareSelector"
    :current-installed-version="gameServer.versionInfo?.installedVersion || gameServer.version"
    :current-software="gameServer.selectedVariantId"
    :current-target="gameServer.selectedTarget"
    :current-target-pinned="gameServer.selectedTargetPinned"
    :current-version="displayVersion"
    :game-name="gameServer.gameName"
    :game-server-id="gameServerId"
    :variants="gameServer.game?.variants ?? []"
    @software-changed="handleSoftwareChanged"
    @software-operation-state="onSoftwareOperationState" />
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import ClipBoardCopy from '@/components/ClipBoardCopy.vue'
import ConsoleCommandInput from '@/components/game_servers/ConsoleCommandInput.vue'
import GameServerPlayerManagementDialog from '@/components/game_servers/GameServerPlayerManagementDialog.vue'
import GameServerPlayerList from '@/components/game_servers/GameServerPlayerList.vue'
import ServerSoftwareSelector from '@/components/game_servers/ServerSoftwareSelector.vue'
import type { StepState } from '@/components/game_servers/UpdateProgressPanel.types'
import type { ServerSoftwareOperationEvent } from '@/components/game_servers/ServerSoftwareSelector.types'
import { playerLimit } from '@/components/game_servers/start-args'
import { type QBtn, QScrollArea, useQuasar } from 'quasar'
import { tabMaximize } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifyConnectError, notifyError, notifyInfo, notifySuccess } from '@/api/notifications'
import {
  GameServer,
  GameServerSchema,
  ReadGameServerOutputRequest,
  ReadGameServerOutputRequestSchema,
  ReadGameServerOutputResponse,
  RestartGameServerRequestSchema,
  SendGameServerInputRequest,
  SendGameServerInputRequestSchema,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequestSchema,
} from '@/proto/shared_pb'
import type { HytaleProfile, UpdateProgress } from '@/proto/xylona_pb'
import {
  AcceptMinecraftEulaRequestSchema,
  ClearHytaleAccountRequestSchema,
  ClearSteamGSLTRequestSchema,
  GetGameServerRequest,
  GetGameServerRequestSchema,
  GetUpdateTargetsRequestSchema,
  PollHytaleDeviceAuthRequestSchema,
  SelectHytaleProfileRequestSchema,
  SetSteamGSLTRequestSchema,
  StartHytaleDeviceAuthRequestSchema,
  StepStatus,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema,
  UpdateStep,
} from '@/proto/xylona_pb'
import { canShowUpdateButton } from './game-server-update-capability'
import {
  applyUpdateProgress,
  buildUpdateStepLabels,
  buildUpdateSteps,
  isUpdateProgressTerminal,
} from './update-progress'
import {
  bytesToSize,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { recordLifecycleIntent } from '@/utils/game-server-notifications'
import { computed, nextTick, onBeforeUnmount, onMounted, Ref, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  resolveCanonicalVersionDisplay,
  resolveVariantTrackingLabel,
  resolveVersionSection,
} from './version-display'
import { canSelectSteamBranch, chooseSteamBranchForUpdate } from './steam-branch-update'
import {
  consoleLineMatchesFilter,
  diffPlayers,
  getConsoleFeedClassifier,
  getConsoleFeedFilterOptions,
  type ConsoleFeedFilter,
} from './console-feed'
import { useGameServerConsoleState } from './useGameServerConsoleState'
import { useGameServerMetricsPreview } from './useGameServerMetricsPreview'
import { useGameServerQueryStatusVersion } from './useGameServerQueryStatusVersion'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'
import { resolveConsoleStreamChunk } from './console-stream-sequence'
import { fitConsoleSidePanels, type ConsoleSidePanel } from './console-side-panels'
import { useGameServerName } from './game-server-context'
import {
  findStartBlocker,
  isConfigReadinessItem,
  readinessLabel,
  useGameServerReadiness,
} from './game-server-readiness'
import {
  askLifecycleConfirmation,
  buildLifecycleConfirmation,
  isServerRunning,
  type LifecycleConfirmAction,
} from './server-list-actions'
import { formatFailureTime, useGameServerLifecycle } from './start-failure'
import { isServerStopping } from '@/utils/game-server-stopping'

const $q = useQuasar()
const route = useRoute()
const serverName = useGameServerName()
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const gameServerId: Ref<string> = ref(
  route.params.id instanceof Array ? route.params.id[0] : route.params.id,
)
const consoleScrollArea = ref<QScrollArea | null>(null)
const softwareSelector = ref<InstanceType<typeof ServerSoftwareSelector> | null>(null)
const mainArea = ref<HTMLElement | null>(null)
const consoleExpanded = ref(false)

// Fullscreen covers the whole app, so everything outside the view leaves the
// tab order. Body-level portals (dialogs, menus, toasts) stay reachable.
let inertBehindConsole: Element[] = []
function setAppInertBehindConsole(inert: boolean): void {
  for (const element of inertBehindConsole) element.removeAttribute('inert')
  inertBehindConsole = []
  if (!inert) return
  for (let element = mainArea.value; element !== null; element = element.parentElement) {
    const parent = element.parentElement
    if (parent === null || parent === document.body) break
    for (const sibling of Array.from(parent.children)) {
      if (sibling === element || sibling.hasAttribute('inert')) continue
      sibling.setAttribute('inert', '')
      inertBehindConsole.push(sibling)
    }
  }
}
watch(consoleExpanded, setAppInertBehindConsole)

// Below Quasar's md breakpoint server details is an overlay drawer and the
// players live in the identity bar, so neither squeezes the console.
const isNarrow = computed(() => $q.screen.lt.md)
const sidebarDrawerOpen = ref(false)
const detailsButton = ref<QBtn | null>(null)
const drawerCloseButton = ref<QBtn | null>(null)

// Tab wraps inside the open drawer. The page behind is not made inert, so the
// start-failure alert there is still announced.
function trapDrawerFocus(event: KeyboardEvent): void {
  if (event.key !== 'Tab' || !isNarrow.value || !sidebarDrawerOpen.value) return
  const focusable = Array.from(
    (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => element.offsetParent !== null)
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (first === undefined || last === undefined) return
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

// The drawer takes focus while open and hands it back to Details on close.
watch(sidebarDrawerOpen, async (open) => {
  // The drawer stays inert until it renders open.
  if (open) await nextTick()
  const button = (open ? drawerCloseButton : detailsButton).value?.$el as HTMLElement | undefined
  button?.focus()
})
const sidePanelStorageKeys: Record<ConsoleSidePanel, string> = {
  sidebar: 'xylona_console_sidebar',
  players: 'xylona_console_player_rail',
}
// What the user asked for; only their own clicks are saved.
const wantedSidePanels = ref({
  sidebar: readSidePanelPreference('sidebar'),
  players: readSidePanelPreference('players'),
})
const lastOpenedSidePanel = ref<ConsoleSidePanel | null>(null)
const mainAreaWidth = ref(Number.POSITIVE_INFINITY)
const openSidePanels = computed(() =>
  fitConsoleSidePanels(mainAreaWidth.value, wantedSidePanels.value, lastOpenedSidePanel.value),
)
const sidebarCollapsed = computed(() =>
  isNarrow.value ? !sidebarDrawerOpen.value : !openSidePanels.value.sidebar,
)
const playerRailCollapsed = computed(() => !openSidePanels.value.players)
const playerManagementOpen = ref(false)

function openPlayerManagement(): void {
  if (gameServer.value.gameId !== '7_days_to_die') playerManagementOpen.value = true
}

function readSidePanelPreference(panel: ConsoleSidePanel): boolean {
  try {
    return window.localStorage.getItem(sidePanelStorageKeys[panel]) !== 'collapsed'
  } catch {
    return true
  }
}

function setSidePanelOpen(panel: ConsoleSidePanel, open: boolean): void {
  wantedSidePanels.value = { ...wantedSidePanels.value, [panel]: open }
  lastOpenedSidePanel.value = open ? panel : null
  try {
    window.localStorage.setItem(sidePanelStorageKeys[panel], open ? 'open' : 'collapsed')
  } catch {
    // Persisting the preference is best-effort.
  }
}

function onMainAreaResize({ width }: { width: number }): void {
  mainAreaWidth.value = width
}

function openDetails(): void {
  // Details can't show over the fullscreen console, so leave fullscreen first.
  consoleExpanded.value = false
  if (isNarrow.value) {
    sidebarDrawerOpen.value = true
    return
  }
  setSidePanelOpen('sidebar', true)
}

function scrollConsoleToBottom() {
  const el = consoleScrollArea.value?.$el as HTMLElement | undefined
  const container = el?.querySelector('.q-scrollarea__container') as HTMLElement | null
  if (container) {
    container.scrollTop = container.scrollHeight
  }
}

// Following pauses once the reader scrolls this far above the newest line.
const followThresholdPx = 40

function onConsoleScroll(event: Event) {
  const container = event.target
  if (
    !(container instanceof HTMLElement) ||
    !container.classList.contains('q-scrollarea__container')
  ) {
    return
  }
  setConsoleScrolledAway(
    container.scrollHeight - container.clientHeight - container.scrollTop > followThresholdPx,
  )
}

const {
  appendConsoleOutput,
  appendPlayerEvent,
  cancelPendingConsoleFlush,
  consoleAutoScroll,
  consoleLines,
  consoleTruncated,
  jumpToLatestOutput,
  navigateConsoleInputHistory,
  recordConsoleInput,
  replaceConsoleOutput,
  serverInput,
  setConsoleScrolledAway,
  toggleConsoleAutoScroll: toggleAutoScroll,
  unseenConsoleOutput,
} = useGameServerConsoleState({
  gameID: computed(() => gameServer.value.gameId),
  scrollToBottom: scrollConsoleToBottom,
})

// The log is silent so a busy server never reads every line aloud; this
// summary speaks for it at most once per interval. Line ids only grow.
const consoleAnnounceIntervalMs = 5000
const consoleAnnouncement = ref('')
let announcedConsoleLineId = -1
let consoleAnnounceTimer: ReturnType<typeof setTimeout> | undefined

watch(
  () => consoleLines.value.at(-1)?.id,
  () => {
    consoleAnnounceTimer ??= setTimeout(announceNewConsoleLines, consoleAnnounceIntervalMs)
  },
)

function announceNewConsoleLines(): void {
  consoleAnnounceTimer = undefined
  const lastLineId = consoleLines.value.at(-1)?.id ?? announcedConsoleLineId
  const newLines = lastLineId - announcedConsoleLineId
  announcedConsoleLineId = lastLineId
  if (newLines <= 0) return
  // Clear, then set after a flush: the same count twice must still change the region.
  consoleAnnouncement.value = ''
  void nextTick(() => {
    consoleAnnouncement.value = `${newLines} new console ${newLines === 1 ? 'line' : 'lines'}`
  })
}

const startingServer = ref(false)
const stoppingServer = ref(false)
// Shared with the identity bar, so "Start failed" and "Restarting" show on every tab.
const {
  lastStartFailure,
  intents: lifecycleIntents,
  restarting: restartingServer,
} = useGameServerLifecycle(gameServerId)

function dismissStartFailure(): void {
  lastStartFailure.value = null
}
const serverStatusFresh = ref(false)
const sendingConsoleInput = ref(false)
const consoleStreamState = ref<'loading' | 'ready' | 'reconnecting' | 'error'>('loading')
const consoleLoadError = ref('')
const lastConsoleSequence = ref(0n)
const receivedConsoleReset = ref(false)
const updatingServer = ref(false)
const updateInProgress = ref(false)
const updateSteps = ref<StepState[]>([])
const softwareOperationInProgress = ref(false)
const { items: readinessItems, reload: reloadReadiness } = useGameServerReadiness(gameServerId)
const acceptingMinecraftEula = ref(false)
const steamGSLT = ref('')
const savingSteamGSLT = ref(false)
const clearingSteamGSLT = ref(false)
const hytaleFlowId = ref('')
const hytaleUserCode = ref('')
const hytaleVerificationUri = ref('')
const hytaleVerificationUriComplete = ref('')
const hytaleProfiles = ref<HytaleProfile[]>([])
const selectedHytaleProfile = ref('')
const startingHytaleAuth = ref(false)
const pollingHytaleAuth = ref(false)
const selectingHytaleProfile = ref(false)
const clearingHytaleAccount = ref(false)
let gameServerDetailsRequestSequence = 0
let liveStatusSequence = 0

const {
  cpuBarClass,
  formatRate,
  formattedUptime,
  memoryBarClass,
  metricsConnections,
  metricsCpu,
  metricsCpuCores,
  metricsDisk,
  metricsDiskValid,
  metricsHeapMarkerRatio,
  metricsIoReadRate,
  metricsIoWriteRate,
  metricsMaxMemory,
  metricsMemory,
  metricsMemoryBarRatio,
  metricsMemoryPercent,
  metricsReceived,
  metricsThreads,
  startMetricsPreviewLifecycle,
} = useGameServerMetricsPreview({
  gameServer,
  gameServerId,
})
const {
  playerCount,
  currentPlayerCount,
  maxPlayerCount,
  onlinePlayers,
  playerListSupported,
  unknownPlayersMessage,
  queryGameServer,
  startQueryStatusVersionLifecycle,
} = useGameServerQueryStatusVersion({
  gameServer,
  gameServerId,
})

// Without a query reply (e.g. offline Valheim) show the configured limit, as the server list does.
const displayedMaxPlayerCount = computed(
  () => maxPlayerCount.value || Number(playerLimit(gameServer.value)),
)
const isServerOnline = computed(() => gameServer.value.status === Status.ONLINE)
// Started but not ready yet still has a live process: metrics and stdin work.
const isServerProcessRunning = computed(() => isServerRunning(gameServer.value.status))
const serverStopping = computed(() => isServerStopping(gameServerId.value, gameServer.value.status))
const showLiveMetrics = computed(() => isServerProcessRunning.value && metricsReceived.value)
const isServerOffline = computed(() => gameServer.value.status === Status.OFFLINE)
const isServerStatusUnknown = computed(() => gameServer.value.status === Status.UNKNOWN)
const unlistedPlayerCount = computed(() =>
  Math.max(currentPlayerCount.value - onlinePlayers.value.length, 0),
)
const serverStateAuthoritative = computed(
  () =>
    websocketStateAuthoritative.value && serverStatusFresh.value && !isServerStatusUnknown.value,
)
const consoleInputDisabled = computed(
  () =>
    !hasPermission('game_server.console') ||
    !isServerProcessRunning.value ||
    serverStopping.value ||
    !serverStateAuthoritative.value ||
    sendingConsoleInput.value,
)
const consoleInputDisabledReason = computed(() => {
  if (!hasPermission('game_server.console')) {
    return 'Sending commands requires console permission'
  }
  if (isServerOffline.value && serverStateAuthoritative.value) {
    return 'Server offline — start it to send commands'
  }
  if (serverStopping.value) {
    return 'Server is stopping — commands are closed'
  }
  if (!serverStateAuthoritative.value || isServerStatusUnknown.value) {
    return 'Waiting for server status — commands are paused'
  }
  return ''
})
const hasConsoleOutput = computed(() => consoleLines.value.length > 0)

// --- Console feed filters + panel-generated player events ---
const consoleFeedFilter = ref<ConsoleFeedFilter>('all')
const consoleFeedClassifier = computed(() => getConsoleFeedClassifier(gameServer.value.gameId))
const consoleFeedFilterOptions = computed(() =>
  getConsoleFeedFilterOptions({
    classifier: consoleFeedClassifier.value,
    playerEventsAvailable: playerListSupported.value,
  }),
)
const visibleConsoleLines = computed(() => {
  if (consoleFeedFilter.value === 'all') return consoleLines.value
  return consoleLines.value.filter((line) =>
    consoleLineMatchesFilter(line.kind, consoleFeedFilter.value),
  )
})
const filteredConsoleEmpty = computed(
  () =>
    consoleFeedFilter.value !== 'all' &&
    visibleConsoleLines.value.length === 0 &&
    consoleLines.value.length > 0,
)
const activeFilterLabel = computed(
  () =>
    consoleFeedFilterOptions.value.find((option) => option.value === consoleFeedFilter.value)
      ?.label ?? 'matching',
)

watch(consoleFeedFilterOptions, (options) => {
  if (!options.some((option) => option.value === consoleFeedFilter.value)) {
    consoleFeedFilter.value = 'all'
  }
})

// Player list snapshots are diffed into join/leave markers in the console stream.
// The first snapshot after (re)connecting is the baseline, not a mass join.
let playersBaseline: string[] | null = null
watch(onlinePlayers, (next) => {
  if (!isServerOnline.value || !playerListSupported.value) {
    playersBaseline = null
    return
  }
  if (playersBaseline === null) {
    playersBaseline = [...next]
    return
  }
  const playerChanges = diffPlayers(playersBaseline, next)
  for (const name of playerChanges.joined) {
    appendPlayerEvent({
      type: 'join',
      name,
      playerCount: currentPlayerCount.value,
      playerCapacity: maxPlayerCount.value,
    })
  }
  for (const name of playerChanges.left) {
    appendPlayerEvent({
      type: 'leave',
      name,
      playerCount: currentPlayerCount.value,
      playerCapacity: maxPlayerCount.value,
    })
  }
  playersBaseline = [...next]
})

watch(isServerOnline, (online) => {
  if (!online) playersBaseline = null
})
const showConsolePlaceholder = computed(
  () =>
    (isServerOffline.value || isServerStatusUnknown.value) &&
    !hasConsoleOutput.value &&
    !updateInProgress.value &&
    !softwareOperationInProgress.value,
)
const visibleReadinessItems = computed(() =>
  readinessItems.value.filter(
    (item) =>
      item.required &&
      (item.blocking ||
        ((item.kind === 'steam_gslt' || item.kind === 'hytale_account') &&
          item.complete &&
          hasPermission('game_server.settings'))),
  ),
)
const startBlocker = computed(() => findStartBlocker(readinessItems.value))
const setupBlocksStart = computed(() => startBlocker.value !== undefined)
const offlineHint = computed(() => {
  if (isServerStatusUnknown.value) return 'Lifecycle controls are paused until status is confirmed'
  if (startBlocker.value !== undefined) {
    return `Start is blocked until ${readinessLabel(startBlocker.value.kind)} is finished — see Details`
  }
  return 'Press Start to launch the server'
})
// Readiness re-reads on focus and navigation, so a load alone must not flash an empty section.
const readinessVisible = computed(() => visibleReadinessItems.value.length > 0)
const hytaleVerificationLink = computed(
  () => hytaleVerificationUriComplete.value || hytaleVerificationUri.value,
)
const hytaleProfileOptions = computed(() =>
  hytaleProfiles.value.map((profile) => ({
    label: profile.username || profile.uuid,
    value: profile.uuid,
  })),
)

const connectionAddress = computed(() => {
  const ip = gameServer.value.ip?.address ?? ''
  const port = gameServer.value.port.toString()
  if (!ip) return port
  return `${ip}:${port}`
})

function onEscapeKey(e: KeyboardEvent) {
  // A prevented Escape already closed something, such as the command menu.
  if (e.key !== 'Escape' || e.defaultPrevented) return
  if (consoleExpanded.value) {
    consoleExpanded.value = false
    return
  }
  // Dialogs opened from the drawer are portaled out and close on their own Escape.
  if (
    isNarrow.value &&
    sidebarDrawerOpen.value &&
    e.target instanceof Node &&
    mainArea.value?.contains(e.target)
  ) {
    sidebarDrawerOpen.value = false
  }
}

const disableStartButton = computed(
  () =>
    !serverStateAuthoritative.value || !isServerOffline.value || startBlocker.value !== undefined,
)

// Starting servers can be stopped or restarted too; a stop from any view closes both.
const disableStopButton = computed(
  () => !serverStateAuthoritative.value || !isServerProcessRunning.value || serverStopping.value,
)

const startHint = computed(() => {
  const hint = lifecycleHint('game_server.start')
  const blocker = startBlocker.value
  if (hint !== '' || !isServerOffline.value || blocker === undefined) return hint
  return `Finish setup first — ${readinessLabel(blocker.kind)}: ${blocker.message}`
})

const startAriaLabel = computed(() => {
  const blocker = startBlocker.value
  if (!hasPermission('game_server.start') || !isServerOffline.value || blocker === undefined) {
    return lifecycleAriaLabel('Start', 'game_server.start')
  }
  return `Start (blocked until ${readinessLabel(blocker.kind)} is finished)`
})

// Rendered in the sidebar, and in the console topbar whenever the sidebar is out of view.
const lifecycleControls = computed(() => [
  {
    label: 'Start',
    icon: 'play_arrow',
    color: 'positive',
    ariaLabel: startAriaLabel.value,
    disable: disableStartButton.value || !hasPermission('game_server.start'),
    loading: startingServer.value,
    hint: startHint.value,
    run: startGameServer,
  },
  {
    label: 'Restart',
    icon: 'restart_alt',
    color: 'warning',
    ariaLabel: lifecycleAriaLabel('Restart', 'game_server.restart'),
    disable:
      disableStopButton.value || stoppingServer.value || !hasPermission('game_server.restart'),
    loading: restartingServer.value,
    hint: lifecycleHint('game_server.restart'),
    run: restartGameServer,
  },
  {
    label: 'Stop',
    icon: 'stop',
    color: 'negative',
    ariaLabel: lifecycleAriaLabel('Stop', 'game_server.stop'),
    disable:
      disableStopButton.value || restartingServer.value || !hasPermission('game_server.stop'),
    loading: stoppingServer.value,
    hint: lifecycleHint('game_server.stop'),
    run: stopGameServer,
  },
])
const showTopbarControls = computed(
  () => isNarrow.value || sidebarCollapsed.value || consoleExpanded.value,
)

const disableUpdateButton = computed(() => {
  return (
    !serverStateAuthoritative.value ||
    gameServer.value.status === Status.INSTALLING ||
    gameServer.value.status === Status.UPDATING ||
    updateInProgress.value
  )
})

const showUpdateButton = computed(() => {
  return canShowUpdateButton(gameServer.value)
})

const softwareDisplayName = computed(() => {
  return softwareSelector.value?.currentSoftwareDisplayName ?? ''
})

const hasSoftwareOptions = computed(() => {
  return (gameServer.value.game?.variants?.length ?? 0) > 0
})

const showChangeButton = computed(() => {
  if (!hasPermission('game_server.settings')) return false
  return (gameServer.value.game?.variants?.length ?? 0) > 1
})

const versionDisplay = computed(() => {
  return resolveCanonicalVersionDisplay(gameServer.value.version, gameServer.value.versionInfo)
})

const displayVersion = computed(() => {
  return versionDisplay.value.installedVersion
})

const variantTrackingLabel = computed(() => {
  return resolveVariantTrackingLabel(
    gameServer.value.resolvedUpdateProvider?.kind,
    gameServer.value.selectedTarget,
    gameServer.value.selectedTargetPinned,
  )
})

const versionClockMs = ref(Date.now())
let versionClockTimer: ReturnType<typeof setInterval> | undefined

const versionSection = computed(() => {
  return resolveVersionSection({
    version: gameServer.value.version,
    versionInfo: gameServer.value.versionInfo,
    providerKind: gameServer.value.resolvedUpdateProvider?.kind,
    selectedTarget: gameServer.value.selectedTarget,
    selectedTargetPinned: gameServer.value.selectedTargetPinned,
    nowMs: versionClockMs.value,
  })
})

const showVersionSection = computed(() => {
  return versionSection.value.installedVersion !== '' || versionSection.value.state !== 'unknown'
})

const versionStatusBadge = computed(() => {
  switch (versionSection.value.state) {
    case 'up-to-date':
      return { icon: 'check_circle', label: 'Up to date', cssClass: 'version-status--ok' }
    case 'update-available':
      return {
        icon: 'arrow_circle_up',
        label: 'Update available',
        cssClass: 'version-status--update',
      }
    case 'checking':
      return { icon: 'sync', label: 'Checking…', cssClass: 'version-status--muted' }
    default:
      return null
  }
})

const versionMetaText = computed(() => {
  const parts: string[] = []
  if (variantTrackingLabel.value !== '') {
    parts.push(variantTrackingLabel.value)
  }
  if (versionSection.value.lastCheckedLabel !== '') {
    parts.push(`checked ${versionSection.value.lastCheckedLabel}`)
  }
  return parts.join(' · ')
})

function hasPermission(perm: string): boolean {
  const perms = gameServer.value?.effectivePermissions ?? []
  // Empty permissions = unknown (cache fallback) — allow everything, backend enforces.
  return perms.length === 0 || perms.includes(perm)
}

function lifecycleHint(perm: string): string {
  if (!hasPermission(perm)) return `Requires ${perm.split('.').pop()} permission`
  if (!serverStateAuthoritative.value) return 'Waiting for authoritative server status'
  return ''
}

function lifecycleAriaLabel(label: string, perm: string): string | undefined {
  return hasPermission(perm) ? undefined : `${label} (requires ${perm.split('.').pop()} permission)`
}

onMounted(async () => {
  document.addEventListener('keydown', onEscapeKey)
  versionClockTimer = setInterval(() => {
    versionClockMs.value = Date.now()
  }, 60_000)
  streamGameServerOutput()

  void getGameServerDetails()
    .then(() => {
      void retryConsoleOutput()
      startQueryStatusVersionLifecycle()
      startMetricsPreviewLifecycle()
    })
    .then(queryGameServer)
})

onBeforeUnmount(() => {
  cancelPendingConsoleFlush()
  clearTimeout(consoleAnnounceTimer)
  setAppInertBehindConsole(false)
  document.removeEventListener('keydown', onEscapeKey)
  if (versionClockTimer !== undefined) {
    clearInterval(versionClockTimer)
  }
  unsubscribeConsoleOutputStream()

  XylonaEventBus.off('gameServerUpdateProgress', onUpdateProgress)
  XylonaEventBus.off('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', onWebsocketDisconnect)
  XylonaEventBus.off('gameServerConsoleOutput', onServerConsoleOutput)
  XylonaEventBus.off('gameServerStatus', onServerStatus)
})

function unsubscribeConsoleOutputStream() {
  const ws = GetOrCreateXylonaWebsocketClient()
  if (!ws.isOpen()) {
    return
  }
  XylonaEventBus.emit('gameServerConsoleOutputRemoveRequest', gameServerId.value)
}

async function requestConsoleOutputStream() {
  const ws = GetOrCreateXylonaWebsocketClient()
  await ws.waitForOpen(10_000)
  XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)
}

async function getGameServerDetails() {
  const requestSequence = ++gameServerDetailsRequestSequence
  const statusSequenceAtStart = liveStatusSequence
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (requestSequence !== gameServerDetailsRequestSequence) {
      return false
    }
    if (response.gameServer === undefined) {
      serverStatusFresh.value = false
      gameServer.value = create(GameServerSchema, {
        ...gameServer.value,
        status: Status.UNKNOWN,
      })
      return false
    }
    gameServer.value =
      statusSequenceAtStart === liveStatusSequence
        ? response.gameServer
        : create(GameServerSchema, {
            ...response.gameServer,
            status: gameServer.value.status,
          })
    serverStatusFresh.value = websocketStateAuthoritative.value
    return true
  } catch (e) {
    if (requestSequence !== gameServerDetailsRequestSequence) {
      return false
    }
    serverStatusFresh.value = false
    gameServer.value = create(GameServerSchema, {
      ...gameServer.value,
      status: Status.UNKNOWN,
    })
    console.error(e)
    notifyConnectError(e, 'Failed to load game server details')
    return false
  }
}

async function acceptMinecraftEula() {
  acceptingMinecraftEula.value = true
  try {
    const request = create(AcceptMinecraftEulaRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().acceptMinecraftEula(request)
    readinessItems.value = response.items
    notifySuccess('Minecraft EULA accepted.')
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to accept Minecraft EULA')
  } finally {
    acceptingMinecraftEula.value = false
  }
}

async function saveSteamGSLT() {
  savingSteamGSLT.value = true
  try {
    const request = create(SetSteamGSLTRequestSchema, {
      serverId: gameServerId.value,
      token: steamGSLT.value,
    })
    const response = await GetXylonaClient().setSteamGSLT(request)
    readinessItems.value = response.items
    steamGSLT.value = ''
    notifySuccess('Steam GSLT saved.')
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to save Steam GSLT')
  } finally {
    savingSteamGSLT.value = false
  }
}

async function clearSteamGSLT() {
  clearingSteamGSLT.value = true
  try {
    const request = create(ClearSteamGSLTRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().clearSteamGSLT(request)
    readinessItems.value = response.items
    steamGSLT.value = ''
    notifySuccess('Steam GSLT cleared.')
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to clear Steam GSLT')
  } finally {
    clearingSteamGSLT.value = false
  }
}

function resetHytaleFlow() {
  hytaleFlowId.value = ''
  hytaleUserCode.value = ''
  hytaleVerificationUri.value = ''
  hytaleVerificationUriComplete.value = ''
  hytaleProfiles.value = []
  selectedHytaleProfile.value = ''
}

async function startHytaleDeviceAuth() {
  startingHytaleAuth.value = true
  try {
    const request = create(StartHytaleDeviceAuthRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().startHytaleDeviceAuth(request)
    hytaleFlowId.value = response.flowId
    hytaleUserCode.value = response.userCode
    hytaleVerificationUri.value = response.verificationUri
    hytaleVerificationUriComplete.value = response.verificationUriComplete
    hytaleProfiles.value = []
    selectedHytaleProfile.value = ''
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to start Hytale authorization')
  } finally {
    startingHytaleAuth.value = false
  }
}

async function pollHytaleDeviceAuth() {
  if (hytaleFlowId.value === '') {
    return
  }
  pollingHytaleAuth.value = true
  try {
    const request = create(PollHytaleDeviceAuthRequestSchema, {
      flowId: hytaleFlowId.value,
    })
    const response = await GetXylonaClient().pollHytaleDeviceAuth(request)
    if (response.status === 'ready') {
      hytaleProfiles.value = response.profiles
      selectedHytaleProfile.value = response.profiles[0]?.uuid ?? ''
      return
    }
    if (response.status === 'denied' || response.status === 'expired') {
      resetHytaleFlow()
      notifyError(response.message || 'Hytale authorization was not completed.')
    }
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to check Hytale authorization')
  } finally {
    pollingHytaleAuth.value = false
  }
}

async function selectHytaleProfile() {
  selectingHytaleProfile.value = true
  try {
    const request = create(SelectHytaleProfileRequestSchema, {
      serverId: gameServerId.value,
      flowId: hytaleFlowId.value,
      profileUuid: selectedHytaleProfile.value,
    })
    const response = await GetXylonaClient().selectHytaleProfile(request)
    readinessItems.value = response.items
    resetHytaleFlow()
    notifySuccess('Hytale account linked.')
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to link Hytale account')
  } finally {
    selectingHytaleProfile.value = false
  }
}

async function clearHytaleAccount() {
  clearingHytaleAccount.value = true
  try {
    const request = create(ClearHytaleAccountRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().clearHytaleAccount(request)
    readinessItems.value = response.items
    resetHytaleFlow()
    notifySuccess('Hytale account link cleared.')
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to clear Hytale account')
  } finally {
    clearingHytaleAccount.value = false
  }
}

function resetUpdateSteps() {
  updateSteps.value = buildUpdateSteps(
    gameServer.value.status,
    buildUpdateStepLabels({
      usesSteamcmd: Boolean(gameServer.value.game?.usesSteamcmd),
    }),
  )
}

function buildSoftwareOperationLabel(event: ServerSoftwareOperationEvent): string {
  if (event.versionLabel) {
    return `${event.softwareName} ${event.versionLabel}`
  }
  return event.softwareName
}

function onSoftwareOperationState(event: ServerSoftwareOperationEvent) {
  if (event.status === 'installing') {
    softwareOperationInProgress.value = true
    return
  }

  softwareOperationInProgress.value = false
  if (event.status === 'failed') {
    notifyError(
      `Variant change to ${buildSoftwareOperationLabel(event)} failed: ${event.error || 'unknown error'}`,
    )
  }
}

async function handleSoftwareChanged() {
  XylonaEventBus.emit('gameServerEdited', gameServerId.value)
  await getGameServerDetails()
  if (hasConsoleOutput.value) {
    return
  }
  await getGameServerOutput()
}

function onUpdateProgress(progress: UpdateProgress) {
  if (progress.gameServerId !== gameServerId.value) return

  updateSteps.value = applyUpdateProgress(updateSteps.value, progress)

  if (isUpdateProgressTerminal(progress, updateSteps.value)) {
    updateInProgress.value = false
    if (
      progress.step === UpdateStep.RESTARTING ||
      (progress.step === UpdateStep.INSTALLING && progress.stepStatus === StepStatus.COMPLETED)
    ) {
      void getGameServerDetails()
    }
  }
}

async function startGameServer(): Promise<void> {
  if (disableStartButton.value || !hasPermission('game_server.start')) return
  startingServer.value = true
  lastStartFailure.value = null
  lifecycleIntents.startRequestedAt = Date.now()
  try {
    await GetXylonaClient().startGameServer(
      create(StartGameServerRequestSchema, { serverId: gameServerId.value }),
    )
  } catch (error) {
    console.error(error)
    lifecycleIntents.startRequestedAt = 0
    lastStartFailure.value = { at: Date.now(), message: connectErrorMessage(error) }
    notifyConnectError(error, 'Failed to start game server')
    // A rejected start often means a setup blocker; re-reading it disables Start.
    void reloadReadiness()
  } finally {
    startingServer.value = false
  }
}

function confirmLifecycleAction(action: LifecycleConfirmAction): Promise<boolean> {
  return askLifecycleConfirmation(
    $q,
    buildLifecycleConfirmation(action, [
      { displayName: gameServer.value.name, playerCount: playerCount.value },
    ]),
  )
}

async function stopGameServer(): Promise<void> {
  if (disableStopButton.value || !hasPermission('game_server.stop')) return
  if (!(await confirmLifecycleAction('stop'))) return
  stoppingServer.value = true
  lifecycleIntents.stopRequestedAt = Date.now()
  try {
    await GetXylonaClient().stopGameServer(
      create(StopGameServerRequestSchema, { serverId: gameServerId.value }),
    )
  } catch (error) {
    console.error(error)
    notifyConnectError(error, 'Failed to stop game server')
  } finally {
    stoppingServer.value = false
  }
}

async function restartGameServer(): Promise<void> {
  if (disableStopButton.value || !hasPermission('game_server.restart')) return
  if (!(await confirmLifecycleAction('restart'))) return
  restartingServer.value = true
  // The restart passes through Offline; that is not a failed start.
  lifecycleIntents.stopRequestedAt = Date.now()
  try {
    await GetXylonaClient().restartGameServer(
      create(RestartGameServerRequestSchema, { serverId: gameServerId.value }),
    )
  } catch (error) {
    console.error(error)
    notifyConnectError(error, 'Failed to restart game server')
  } finally {
    restartingServer.value = false
  }
}

async function updateGameServer() {
  if (!serverStateAuthoritative.value) {
    return
  }

  if (isServerProcessRunning.value) {
    const confirmed = await new Promise<boolean>((resolve) => {
      let settled = false
      $q.dialog({
        title: 'Update running server?',
        message: 'Xylona will stop the server, install the update, and start it again.',
        cancel: true,
        persistent: true,
        ok: {
          label: 'Update server',
          color: 'primary',
          unelevated: true,
        },
      })
        .onOk(() => {
          settled = true
          resolve(true)
        })
        .onDismiss(() => {
          if (!settled) {
            resolve(false)
          }
        })
    })
    if (!confirmed) {
      return
    }
  }

  const steamBranchSelection = await chooseSteamBranchForUpdate({
    gameServerId: gameServerId.value,
    gameServer: gameServer.value,
    getBranches: async (serverId: string) => {
      const request = create(GetUpdateTargetsRequestSchema, { gameServerId: serverId })
      return GetXylonaClient().getUpdateTargets(request)
    },
    openDialog: ({ currentBranch, items, onOk, onDismiss }) => {
      let settled = false
      $q.dialog({
        title: 'Choose Update Target',
        message: 'Select which release target this server should update to.',
        cancel: true,
        persistent: true,
        ok: {
          label: 'Update server',
          color: 'primary',
          unelevated: true,
        },
        options: {
          type: 'radio',
          model: currentBranch,
          items,
        },
      })
        .onOk((value) => {
          settled = true
          onOk(typeof value === 'string' && value.trim() !== '' ? value : currentBranch)
        })
        .onDismiss(() => {
          if (!settled) {
            onDismiss()
          }
        })
    },
  })
  if (steamBranchSelection.cancelled) {
    return
  }
  if (canSelectSteamBranch(gameServer.value) && !steamBranchSelection.metadataAvailable) {
    notifyInfo('Update target metadata is unavailable. Updating with the current target.')
  }

  const request: UpdateGameServerRequest = create(UpdateGameServerRequestSchema, {})
  updatingServer.value = true
  resetUpdateSteps()
  updateInProgress.value = true
  try {
    request.serverId = gameServerId.value
    request.target = steamBranchSelection.steamBranch
    await GetXylonaClient().updateGameServer(request)
    recordLifecycleIntent(gameServerId.value, 'update')
  } catch (e) {
    updateInProgress.value = false
    console.error(e)
    notifyConnectError(e, 'Failed to update game server')
  } finally {
    updatingServer.value = false
  }
}

async function getGameServerOutput(fallbackOnly = false) {
  const request: ReadGameServerOutputRequest = create(ReadGameServerOutputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const response: ReadGameServerOutputResponse =
      await GetXylonaClient().readGameServerOutput(request)
    if (fallbackOnly && receivedConsoleReset.value) {
      return
    }
    appendConsoleOutput(response.output)
    consoleLoadError.value = ''
  } catch (e) {
    console.error(e)
    consoleLoadError.value =
      'Earlier console output could not be loaded. Live output may still appear.'
    if (consoleStreamState.value !== 'ready') {
      consoleStreamState.value = 'error'
    }
  }
}

async function onWebsocketReconnect() {
  serverStatusFresh.value = false
  receivedConsoleReset.value = false
  lastConsoleSequence.value = 0n
  consoleStreamState.value = 'reconnecting'
  const detailsLoaded = await getGameServerDetails()
  if (!detailsLoaded) {
    console.error('Failed to refresh game server status after websocket reconnect')
  }
  try {
    await requestConsoleOutputStream()
  } catch (error) {
    console.error('Failed to resubscribe to game server console output', error)
    consoleLoadError.value = 'Console reconnection failed.'
    consoleStreamState.value = 'error'
  }
}

function onWebsocketDisconnect() {
  serverStatusFresh.value = false
  consoleStreamState.value = 'reconnecting'
}

function onServerStatus(serverID: string, _serverName: string, status: Status) {
  if (serverID !== gameServerId.value) {
    return
  }

  liveStatusSequence++
  if (status === Status.OFFLINE) {
    lastConsoleSequence.value = 0n
    receivedConsoleReset.value = false
  }
  gameServer.value = create(GameServerSchema, {
    ...gameServer.value,
    status,
  })
  serverStatusFresh.value = websocketStateAuthoritative.value
}

function onServerConsoleOutput(
  serverID: string,
  output: string,
  sequence: bigint = 0n,
  resetBuffer: boolean = false,
  reconnecting: boolean | undefined = undefined,
) {
  if (serverID !== gameServerId.value) {
    return
  }

  if (reconnecting !== undefined) {
    consoleLoadError.value = ''
    consoleStreamState.value = reconnecting ? 'reconnecting' : 'ready'
    if (reconnecting && output !== '') {
      appendConsoleOutput(output)
    }
    return
  }

  const decision = resolveConsoleStreamChunk(lastConsoleSequence.value, {
    sequence,
    reset: resetBuffer,
  })
  if (decision.action === 'ignore') {
    return
  }
  lastConsoleSequence.value = decision.nextSequence

  if (decision.action === 'replace') {
    receivedConsoleReset.value = true
    consoleLoadError.value = ''
    consoleStreamState.value = 'ready'
    replaceConsoleOutput(output)
    // A replay (first load, reconnect) re-sends history, not new output: count it as heard.
    announcedConsoleLineId = consoleLines.value.at(-1)?.id ?? announcedConsoleLineId
    return
  }

  appendConsoleOutput(output)
  consoleStreamState.value = 'ready'
}

function streamGameServerOutput() {
  // Stream game server output.
  XylonaEventBus.on('gameServerConsoleOutput', onServerConsoleOutput)
  XylonaEventBus.on('gameServerStatus', onServerStatus)

  // Listen for update progress events before any initial websocket request so
  // an early send failure cannot skip the listener registration.
  XylonaEventBus.on('gameServerUpdateProgress', onUpdateProgress)

  // Handle socket reconnection.
  XylonaEventBus.on('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.on('websocketDisconnected', onWebsocketDisconnect)
}

async function retryConsoleOutput() {
  receivedConsoleReset.value = false
  consoleStreamState.value = 'loading'
  consoleLoadError.value = ''
  try {
    await requestConsoleOutputStream()
  } catch (error) {
    console.error('Failed to subscribe to game server console output', error)
    consoleLoadError.value = 'Could not connect to live console output.'
    consoleStreamState.value = 'error'
  }
  await getGameServerOutput(true)
}

async function sendGameServerInput() {
  if (consoleInputDisabled.value || serverInput.value === '') {
    return
  }

  const request: SendGameServerInputRequest = create(SendGameServerInputRequestSchema, {})
  sendingConsoleInput.value = true
  try {
    request.serverId = gameServerId.value
    request.input = serverInput.value
    await GetXylonaClient().sendGameServerInput(request)
    recordConsoleInput()
  } catch (e) {
    console.error(e)
    notifyConnectError(e, 'Failed to send command')
  } finally {
    sendingConsoleInput.value = false
  }
}
</script>

<style scoped>
/* ===== Main Area ===== */
.main-area {
  flex: 1;
  display: flex;
  min-height: 0;
  position: relative;
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 var(--xy-radius-md) var(--xy-radius-md);
  overflow: hidden;
}

/* ===== Sidebar ===== */
.sidebar-backdrop,
.sidebar-mobile-header {
  display: none;
}

.sidebar {
  width: 290px;
  display: flex;
  flex-direction: column;
  background: var(--xy-surface-0);
  border-right: 1px solid var(--xy-border);
  flex-shrink: 0;
  /* Animates layout width on purpose, like the player rail: the console must reflow with it. */
  transition:
    width 0.25s cubic-bezier(0.25, 1, 0.5, 1),
    opacity 0.2s ease;
  overflow: hidden;
}

.sidebar.collapsed {
  width: 0;
  border-right: none;
  opacity: 0;
}

.sidebar-content {
  padding: var(--xy-space-md);
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
  overflow-y: auto;
  flex: 1;
  min-width: 290px;
}

.sidebar-section-label {
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  margin-bottom: var(--xy-space-xs);
}

/* ===== Controls ===== */
.server-controls {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.server-control {
  display: inline-flex;
}

/* Browsers send no hover to a disabled button; let it reach the wrapper's hint. */
.server-control:has(.q-btn.disabled) {
  cursor: not-allowed;
}

.server-control .q-btn.disabled {
  pointer-events: none;
}

.controls-hint {
  margin-top: var(--xy-space-sm);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  line-height: 1.35;
}

.controls-hint--topbar {
  flex-shrink: 0;
  margin-top: 0;
  padding: var(--xy-space-xs) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.readiness-list {
  display: grid;
  gap: var(--xy-space-sm);
}

.readiness-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm);
  border: 1px solid color-mix(in srgb, var(--xy-warning) 35%, var(--xy-border));
  border-radius: var(--xy-radius-md);
  background: color-mix(in srgb, var(--xy-warning) 9%, var(--xy-surface-1));
}

.readiness-item--complete {
  border-color: color-mix(in srgb, var(--xy-success) 35%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-success) 8%, var(--xy-surface-1));
}

.readiness-item-icon {
  color: var(--xy-warning);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 2px;
}

.readiness-item--complete .readiness-item-icon {
  color: var(--xy-success);
}

.readiness-item-body {
  min-width: 0;
}

.readiness-item-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.readiness-item-message {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  line-height: 1.35;
  margin-top: 2px;
}

.readiness-action {
  margin-top: var(--xy-space-sm);
}

.readiness-secret-form {
  display: grid;
  gap: var(--xy-space-xs);
  margin-top: var(--xy-space-sm);
}

.readiness-secret-input {
  min-width: 0;
}

.readiness-secret-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
}

.readiness-device-flow {
  display: grid;
  gap: var(--xy-space-xs);
}

.readiness-device-code {
  display: inline-flex;
  justify-content: center;
  width: fit-content;
  max-width: 100%;
  padding: 0.2rem 0.45rem;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  overflow-wrap: anywhere;
}

.readiness-link {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
  text-decoration: none;
}

.readiness-link:hover {
  text-decoration: underline;
}

.readiness-profile-select {
  min-width: 0;
}

/* Version section */
.version-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: var(--xy-border);
  border-radius: var(--xy-radius-sm);
  overflow: hidden;
}

.version-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
}

.version-status {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: var(--xy-font-size-2xs);
}

.version-status--ok {
  color: var(--xy-success);
}

.version-status--update {
  color: var(--xy-warning);
}

.version-status--muted {
  color: var(--xy-text-muted);
}

.version-meta {
  font-size: var(--xy-font-size-2xs);
  color: var(--xy-text-muted);
  text-align: right;
}

.update-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--xy-warning);
  flex-shrink: 0;
  animation: update-pulse 2.5s ease-in-out infinite;
}

/* Opacity only, so the pulse stays on the compositor; reduced motion stops it globally. */
@keyframes update-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}

.version-software {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

/* Change button */
.change-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-2xs);
  font-weight: 500;
  cursor: pointer;
  min-height: 24px;
  padding: 0.25rem 0.6rem;
  transition: all var(--xy-transition-fast);
}

.change-btn:hover {
  border-color: var(--xy-primary);
  color: var(--xy-primary-text);
  background: var(--xy-primary-muted);
}

.change-arrow {
  font-size: var(--xy-font-size-2xs);
  transition: transform var(--xy-transition-fast);
}

.change-btn:hover .change-arrow {
  transform: translateX(2px);
}

/* ===== Connection ===== */
.connection-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: var(--xy-border);
  border-radius: var(--xy-radius-sm);
  overflow: hidden;
}

.connection-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
}

.cl-label {
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-muted);
}

.cl-value {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-secondary);
}

.cl-value :deep(.copy-clipboard) {
  cursor: pointer;
  transition: color var(--xy-transition-fast);
}

.cl-value :deep(.copy-clipboard:hover) {
  color: var(--xy-accent);
}

.cl-value-plain {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-secondary);
}

/* ===== Console feed filters (inside the topbar) ===== */
.console-feed-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.console-feed-filters__label {
  margin-right: var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.console-feed-filter {
  min-height: 24px;
  padding: 1px var(--xy-space-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-pill);
  background: transparent;
  color: var(--xy-text-secondary);
  cursor: pointer;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.console-feed-filter:hover {
  border-color: var(--xy-border-hover);
  color: var(--xy-text-primary);
}

.console-feed-filter:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 1px;
}

.console-feed-filter--active {
  background: var(--xy-primary-darker);
  border-color: var(--xy-primary-darker);
  color: var(--xy-text-on-dark);
}

.console-feed-filters__note {
  margin-left: var(--xy-space-sm);
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
}

/* Panel-generated player join/leave markers injected into the console stream */
.console-scroll-area :deep(.console-player-event) {
  display: block;
  padding: var(--xy-space-2xs) 0;
  color: var(--xy-text-secondary);
}

.console-scroll-area :deep(.console-player-event b) {
  font-weight: 600;
}

.console-scroll-area :deep(.console-player-event--join b) {
  color: var(--xy-success-text-soft);
}

.console-scroll-area :deep(.console-player-event--leave b) {
  color: var(--xy-danger-hover);
}

/* ===== Metrics Preview ===== */
.metrics-preview {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.metrics-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding-bottom: var(--xy-space-sm);
  border-bottom: 1px solid var(--xy-border);
}

.metrics-group:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.metrics-group-label {
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  margin-bottom: 0.1rem;
}

.metrics-offline-note {
  margin: 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metrics-offline .metric-bar-fill {
  transform: scaleX(0) !important;
}

.metric-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: var(--xy-font-size-xs);
}

.metric-row .ml {
  color: var(--xy-text-muted);
  font-weight: 500;
}

.metric-detail {
  font-size: var(--xy-font-size-2xs);
  color: var(--xy-text-muted);
}

.metric-row .mv {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.metric-bar {
  width: 100%;
  height: 3px;
  background: var(--xy-surface-3);
  border-radius: var(--xy-radius-sm);
  margin-top: 3px;
  overflow: hidden;
}

.metric-bar--marked {
  position: relative;
  overflow: visible;
}

.metric-bar-marker {
  position: absolute;
  top: -2px;
  bottom: -2px;
  width: 2px;
  transform: translateX(-50%);
  background: var(--xy-text-secondary);
  border-radius: var(--xy-radius-sm);
}

.metric-bar-fill {
  width: 100%;
  height: 100%;
  border-radius: var(--xy-radius-sm);
  transform-origin: left center;
  transition: transform 0.8s var(--xy-ease-standard);
}

.fill-low {
  background: var(--xy-success);
}

.fill-mid {
  background: var(--xy-warning);
}

.fill-high {
  background: var(--xy-danger);
}

/* ===== Console Wrapper ===== */
.console-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  position: relative;
}

/* Console toolbar */
.start-failure {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-danger-bg);
  border-bottom: 1px solid var(--xy-danger-border);
  flex-shrink: 0;
}

.start-failure__icon {
  flex-shrink: 0;
  font-size: var(--xy-font-size-lg);
  color: var(--xy-danger);
}

.start-failure__body {
  display: flex;
  flex-wrap: wrap;
  column-gap: var(--xy-space-sm);
  row-gap: var(--xy-space-2xs);
  min-width: 0;
  flex: 1;
  font-size: var(--xy-font-size-sm);
}

.start-failure__title {
  font-weight: 600;
  color: var(--xy-text-primary);
}

.start-failure__message {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.start-failure__actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  flex-shrink: 0;
}

.console-topbar {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: var(--xy-space-xs);
  min-height: 36px;
  padding: var(--xy-space-2xs) var(--xy-space-xs) var(--xy-space-2xs) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.console-topbar__label {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.console-topbar__spacer {
  flex: 1;
}

/* With lifecycle controls the filters take their own row instead of squeezing them. */
.console-topbar:has(.console-lifecycle) {
  flex-wrap: wrap;
}

.console-lifecycle {
  display: flex;
  gap: var(--xy-space-xs);
}

.console-toolbar-btn {
  padding: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  transition: color var(--xy-transition-fast);
}

.console-autoscroll-btn,
.console-details-btn {
  padding-inline: var(--xy-space-sm);
}

.console-autoscroll-btn :deep(.q-btn__content) {
  flex-wrap: nowrap;
  white-space: nowrap;
}

.console-toolbar-btn:hover {
  color: var(--xy-text-primary);
}

/* Off reads through the pause icon and aria-pressed; the text stays readable. */
.console-toolbar-btn-off {
  color: var(--xy-text-secondary);
}

.console-stream-state {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 2.5rem;
  padding: var(--xy-space-xs) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-warning-bg-faint);
  border-bottom: 1px solid var(--xy-warning-border);
  font-family: var(--xy-font-body);
}

.console-stream-state--error {
  background: var(--xy-danger-bg-faint);
  border-color: var(--xy-danger-border);
}

.console-stream-state .q-btn {
  margin-inline-start: auto;
}

/* Console output scroll area */
.console-output-area {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.console-jump {
  position: absolute;
  bottom: var(--xy-space-md);
  left: 50%;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-base);
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-3);
  box-shadow: var(--xy-shadow-md);
  color: var(--xy-text-primary);
  cursor: pointer;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  transform: translateX(-50%);
}

.console-jump:hover {
  background: var(--xy-surface-4);
}

.console-jump:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.console-scroll-area {
  flex: 1;
  min-height: 0;
  padding-left: var(--xy-space-md);
  padding-right: var(--xy-space-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-weight: 400;
  font-style: normal;
  overflow-x: hidden;
  white-space: pre-wrap;
  max-width: 100%;
  background-color: var(--xy-base);
  position: relative;
}

/* Top fade gradient */
.console-scroll-area::before {
  content: '';
  position: sticky;
  top: 0;
  display: block;
  height: 16px;
  margin-bottom: -16px;
  background: linear-gradient(to bottom, var(--xy-base) 0%, transparent 100%);
  pointer-events: none;
  z-index: 1;
}

.console-truncated-notice {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-muted);
  text-align: center;
  padding: var(--xy-space-xs) 0;
  border-bottom: 1px solid var(--xy-border);
}

.console-status-banner {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-muted);
  padding: var(--xy-space-xs) 0;
  border-bottom: 1px solid var(--xy-border);
  margin-bottom: var(--xy-space-xs);
}

.console-filter-empty {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md) 0;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.console-filter-empty__reset {
  min-height: 24px;
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  background: transparent;
  color: var(--xy-text-secondary);
  cursor: pointer;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.console-filter-empty__reset:hover {
  border-color: var(--xy-border-hover);
  color: var(--xy-text-primary);
}

.console-filter-empty__reset:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 1px;
}

.console-scroll-area :deep(#consoleCodeEl) {
  white-space: pre-wrap;
  /* Break long paths/URLs so they can't widen the content past the clipped edge. */
  overflow-wrap: anywhere;
}

.console-scroll-area :deep(.q-scrollarea__content) {
  padding-top: var(--xy-space-md);
}

/* Offline console output */
.console-output-offline {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--xy-base);
  min-height: 0;
}

.offline-placeholder {
  text-align: center;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
}

.offline-icon {
  width: 56px;
  height: 56px;
  margin: 0 auto var(--xy-space-md);
  border-radius: 50%;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--xy-font-size-xl);
  opacity: 0.5;
  position: relative;
}

.offline-icon::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  border: 1px solid var(--xy-border);
  animation: offline-ring 4s ease-in-out infinite;
}

@keyframes offline-ring {
  0%,
  100% {
    opacity: 0;
    transform: scale(0.95);
  }
  50% {
    opacity: 0.4;
    transform: scale(1);
  }
}

.offline-text {
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.offline-hint {
  font-size: var(--xy-font-size-xs);
}

.offline-details-btn {
  margin-top: var(--xy-space-sm);
}

/* ===== Fullscreen Console ===== */
.console-wrapper.expanded {
  position: fixed;
  inset: 0;
  z-index: var(--xy-z-fullscreen);
  width: 100%;
  height: 100dvh;
  background-color: var(--xy-base);
  animation: console-expand 200ms cubic-bezier(0.25, 1, 0.5, 1) both;
}

.console-wrapper.expanded .console-scroll-area {
  padding-left: var(--xy-space-sm);
}

@keyframes console-expand {
  from {
    opacity: 0;
    transform: scale(0.97);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

/* Expanded state also hides sidebar when fullscreen is active */
.main-area-expanded .sidebar {
  display: none;
}

/* ===== Sidebar header + collapsed strip ===== */
.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  padding: var(--xy-space-xs) var(--xy-space-xs) var(--xy-space-xs) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.sidebar-header__label {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.sidebar-mini {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex-shrink: 0;
  gap: var(--xy-space-sm);
  width: 44px;
  padding-top: var(--xy-space-xs);
  background: var(--xy-surface-0);
  border-right: 1px solid var(--xy-border);
}

.sidebar-mini__label {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  writing-mode: vertical-rl;
}

.main-area-expanded .sidebar-mini {
  display: none;
}

@media (max-width: 1023px) {
  .sidebar-header,
  .sidebar-mini {
    display: none;
  }
}

/* ===== Player rail ===== */
.player-rail {
  display: flex;
  flex-direction: column;
  width: 264px;
  flex-shrink: 0;
  background: var(--xy-surface-0);
  border-left: 1px solid var(--xy-border);
  overflow: hidden;
  /* Animates layout width on purpose: the rail collapses to 44px and the console must reflow with it. */
  transition: width 0.25s cubic-bezier(0.25, 1, 0.5, 1);
}

.player-rail.collapsed {
  align-items: center;
  width: 44px;
  padding-top: var(--xy-space-xs);
  gap: var(--xy-space-sm);
}

.player-rail__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  padding: var(--xy-space-xs) var(--xy-space-xs) var(--xy-space-xs) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.player-rail__title {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.player-rail__head-actions {
  display: inline-flex;
  gap: 2px;
}

.player-rail__body {
  flex: 1;
  min-height: 0;
  padding: var(--xy-space-sm) var(--xy-space-md);
  overflow-y: auto;
}

.player-rail__mini-count {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  writing-mode: vertical-rl;
}

.main-area-expanded .player-rail {
  display: none;
}

@media (max-width: 1023px) {
  .player-rail {
    display: none;
  }
}

/* Native buttons miss the global coarse-pointer rule for q-btn. */
@media (pointer: coarse), (any-pointer: coarse) {
  .change-btn,
  .console-feed-filter,
  .console-filter-empty__reset,
  .console-jump {
    min-height: 44px;
  }
}

/* ===== Focus rings ===== */
.change-btn:focus-visible,
.console-toolbar-btn:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: 2px;
}

/* ===== Mobile ===== */
@media (max-width: 1023px) {
  .sidebar-backdrop {
    display: block;
    position: absolute;
    inset: 0;
    z-index: calc(var(--xy-z-sticky) + 1);
    background: color-mix(in srgb, var(--xy-base) 72%, transparent);
    opacity: 0;
    pointer-events: none;
    transition: opacity var(--xy-transition-base);
  }

  .sidebar-backdrop-visible {
    opacity: 1;
    pointer-events: auto;
  }

  .sidebar {
    position: absolute;
    inset: 0 auto 0 0;
    z-index: var(--xy-z-drawer);
    width: min(20rem, calc(100% - 3rem));
    max-width: 100%;
    border-right: 1px solid var(--xy-border-active);
    opacity: 1;
    transform: translateX(0);
    transition:
      transform var(--xy-transition-base),
      opacity var(--xy-transition-fast);
  }

  .sidebar.collapsed {
    width: min(20rem, calc(100% - 3rem));
    border-right: 1px solid var(--xy-border-active);
    opacity: 0;
    pointer-events: none;
    transform: translateX(-100%);
  }

  .sidebar-mobile-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 3rem;
    padding: 0 var(--xy-space-sm) 0 var(--xy-space-md);
    border-bottom: 1px solid var(--xy-border);
    color: var(--xy-text-primary);
    font-family: var(--xy-font-control);
    font-size: var(--xy-font-size-sm);
    font-weight: 600;
  }

  .sidebar-mobile-close {
    min-width: 44px;
    min-height: 44px;
  }

  .sidebar-content {
    width: 100%;
    min-width: 0;
    padding: var(--xy-space-base);
    gap: var(--xy-space-md);
  }
}

@media (max-width: 599px) {
  .start-failure {
    flex-wrap: wrap;
  }

  .start-failure__actions {
    width: 100%;
    justify-content: flex-end;
  }

  .console-topbar {
    min-height: 3rem;
  }

  .console-toolbar-btn {
    min-width: 44px;
    min-height: 44px;
  }

  /* Icon-only below tablet; the aria-label and tooltip still name the action.
     !important counters Quasar's `.block { display: block !important }` utility. */
  .console-autoscroll-btn :deep(.q-btn__content > span.block),
  .console-lifecycle :deep(.q-btn__content > span.block) {
    display: none !important;
  }

  .console-autoscroll-btn :deep(.q-icon.on-left),
  .console-lifecycle :deep(.q-icon.on-left) {
    margin-right: 0;
  }

  /* The section select already names the page; the controls need the room. */
  .console-topbar:has(.console-lifecycle) .console-topbar__label {
    display: none;
  }

  .console-stream-state {
    flex-wrap: wrap;
    padding: var(--xy-space-sm) var(--xy-space-base);
  }

  .console-scroll-area {
    padding-inline: var(--xy-space-base) var(--xy-space-sm);
  }
}
</style>
