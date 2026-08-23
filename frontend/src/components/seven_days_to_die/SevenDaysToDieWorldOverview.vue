<script lang="ts" setup>
import { computed } from 'vue'

import {
  type SevenDaysToDieGameTime,
  type SevenDaysToDieMapView,
  type SevenDaysToDieWebAPIStatus,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'

type StatusPresentation = {
  icon: string
  label: string
  tone: 'neutral' | 'warning' | 'negative'
}

const props = withDefaults(
  defineProps<{
    configurationPath?: string
    showTactical?: boolean
    status?: SevenDaysToDieWebAPIStatus | null
    statusLoading?: boolean
    statusPresentation?: StatusPresentation | null
    view: SevenDaysToDieMapView | null
  }>(),
  {
    configurationPath: '',
    showTactical: true,
    status: null,
    statusLoading: false,
    statusPresentation: null,
  },
)

const worldTimeState = computed(() => props.status?.worldTimeState ?? props.view?.bloodMoonState)
const worldTime = computed(() => props.status?.worldTime ?? props.view?.bloodMoon?.gameTime)
const bloodMoonState = computed(() => props.status?.bloodMoonState ?? props.view?.bloodMoonState)
const bloodMoonActive = computed(
  () => props.status?.bloodMoonActive ?? props.view?.bloodMoon?.active,
)
const nextBloodMoon = computed(
  () => props.status?.nextBloodMoon ?? props.view?.bloodMoon?.nextBloodMoon,
)
const nextBloodMoonEnd = computed(
  () => props.status?.nextBloodMoonEnd ?? props.view?.bloodMoon?.nextBloodMoonEnd,
)
const worldTimeLabel = computed(() => valueLabel(worldTimeState.value, worldTime.value))
const bloodMoonLabel = computed(() => {
  if (
    bloodMoonState.value ===
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
  ) {
    if (bloodMoonActive.value === true) return 'Active'
    if (bloodMoonActive.value === false) return 'Inactive'
  }
  return valueStateLabel(bloodMoonState.value)
})
const nextBloodMoonLabel = computed(() =>
  bloodMoonTimeLabel(bloodMoonState.value, nextBloodMoon.value),
)
const nextBloodMoonEndLabel = computed(() =>
  bloodMoonTimeLabel(bloodMoonState.value, nextBloodMoonEnd.value),
)
const playerCountLabel = computed(() => {
  const view = props.view
  if (view === null || !view.enabled) return 'Unavailable'
  return `${view.players.filter((player) => player.online).length} online · ${view.players.length} known`
})
const mapSizeLabel = computed(() => {
  const mapSize = props.view?.mapSize
  return mapSize === undefined
    ? 'Unavailable'
    : `${mapSize.x.toLocaleString()} × ${mapSize.z.toLocaleString()}`
})
const mapNoteCountLabel = computed(() =>
  props.view?.enabled ? String(props.view.markers.length) : 'Unavailable',
)
const claimCountLabel = computed(() =>
  tacticalCountLabel(props.view?.claimsState, props.view?.claims.length),
)
const hostileCountLabel = computed(() =>
  tacticalCountLabel(props.view?.hostileState, props.view?.hostiles.length),
)
const animalCountLabel = computed(() =>
  tacticalCountLabel(props.view?.animalState, props.view?.animals.length),
)

function formatGameTime(gameTime: SevenDaysToDieGameTime): string {
  return `Day ${gameTime.day}, ${String(gameTime.hour).padStart(2, '0')}:${String(gameTime.minute).padStart(2, '0')}`
}

function valueStateLabel(state: SevenDaysToDieWebAPIValueState | undefined): string {
  switch (state) {
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED:
      return 'Not supported by this WebAPI'
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED:
      return 'Access denied by the game server'
    default:
      return 'Unavailable'
  }
}

function valueLabel(
  state: SevenDaysToDieWebAPIValueState | undefined,
  gameTime: SevenDaysToDieGameTime | undefined,
): string {
  return state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    gameTime
    ? formatGameTime(gameTime)
    : valueStateLabel(state)
}

function bloodMoonTimeLabel(
  state: SevenDaysToDieWebAPIValueState | undefined,
  gameTime: SevenDaysToDieGameTime | undefined,
): string {
  return state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    gameTime
    ? formatGameTime(gameTime)
    : state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
      ? 'Not reported'
      : valueStateLabel(state)
}

function tacticalCountLabel(
  state: SevenDaysToDieWebAPIValueState | undefined,
  count: number | undefined,
): string {
  return state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
    ? String(count ?? 0)
    : valueStateLabel(state)
}
</script>

<template>
  <section
    class="world-overview"
    data-testid="world-overview"
    aria-atomic="true"
    aria-labelledby="world-overview-title"
    aria-live="polite">
    <header class="world-overview__header">
      <h2 id="world-overview-title">World overview</h2>
      <div class="world-overview__actions">
        <span v-if="statusLoading" class="world-overview__status" role="status">
          <q-spinner size="1rem" />
          Loading world data…
        </span>
        <span
          v-else-if="statusPresentation"
          class="world-overview__status"
          :class="`is-${statusPresentation.tone}`">
          <q-icon :name="statusPresentation.icon" />
          {{ statusPresentation.label }}
        </span>
        <q-btn
          v-if="statusPresentation && configurationPath"
          :to="configurationPath"
          aria-label="Open game server configuration"
          dense
          flat
          icon="tune"
          label="Configure"
          no-caps />
      </div>
    </header>

    <dl class="world-overview__facts">
      <div>
        <dt>World time</dt>
        <dd>{{ worldTimeLabel }}</dd>
      </div>
      <template v-if="showTactical">
        <div data-testid="blood-moon-state" :class="{ 'is-active': bloodMoonActive === true }">
          <dt>Blood Moon</dt>
          <dd>{{ bloodMoonLabel }}</dd>
        </div>
        <div>
          <dt>Next Blood Moon</dt>
          <dd>{{ nextBloodMoonLabel }}</dd>
        </div>
        <div>
          <dt>Blood Moon ends</dt>
          <dd>{{ nextBloodMoonEndLabel }}</dd>
        </div>
      </template>
      <div class="world-overview__fact--group-start">
        <dt>Players</dt>
        <dd>{{ playerCountLabel }}</dd>
      </div>
      <div>
        <dt>World size</dt>
        <dd>{{ mapSizeLabel }}</dd>
      </div>
      <div class="world-overview__fact--group-start">
        <dt>Map notes</dt>
        <dd>{{ mapNoteCountLabel }}</dd>
      </div>
      <template v-if="showTactical">
        <div>
          <dt>Land claims</dt>
          <dd>{{ claimCountLabel }}</dd>
        </div>
        <div>
          <dt>Hostiles</dt>
          <dd>{{ hostileCountLabel }}</dd>
        </div>
        <div>
          <dt>Animals</dt>
          <dd>{{ animalCountLabel }}</dd>
        </div>
      </template>
    </dl>
  </section>
</template>

<style scoped>
.world-overview {
  flex-shrink: 0;
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.world-overview__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-base);
  min-height: 48px;
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.world-overview__header h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  font-weight: 500;
}

.world-overview__actions,
.world-overview__status {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.world-overview__status {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  white-space: nowrap;
}

.world-overview__status.is-warning {
  color: var(--xy-warning-hover);
}

.world-overview__status.is-negative {
  color: var(--xy-danger-hover);
}

.world-overview__facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--xy-space-base) var(--xy-space-lg);
  margin: 0;
  padding: var(--xy-space-md);
}

.world-overview__facts > div {
  display: grid;
  min-width: 0;
  gap: var(--xy-space-xs);
}

.world-overview__facts dt {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.world-overview__facts dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-variant-numeric: tabular-nums;
}

.world-overview__facts .is-active dd {
  color: var(--xy-danger-hover);
}

@media (min-width: 1200px) {
  .world-overview__facts {
    grid-template-columns: 1fr;
    gap: var(--xy-space-base);
  }

  .world-overview__fact--group-start {
    margin-top: var(--xy-space-xs);
    padding-top: var(--xy-space-base);
    border-top: 1px solid var(--xy-border);
  }
}

@media (max-width: 599px) {
  .world-overview {
    border-inline: 0;
    border-radius: 0;
  }

  .world-overview__header {
    align-items: flex-start;
  }

  .world-overview__status {
    white-space: normal;
  }
}
</style>
