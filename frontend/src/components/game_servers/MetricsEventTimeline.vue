<template>
  <section class="metrics-timeline" aria-labelledby="metrics-timeline-title">
    <header class="metrics-timeline__header">
      <div>
        <h2 id="metrics-timeline-title">Lifecycle & operations</h2>
        <p>Restarts, status changes, backups, and updates recorded in this range.</p>
      </div>
      <span class="font-mono">{{ events.length }} events</span>
    </header>

    <ol v-if="events.length > 0" class="metrics-timeline__list">
      <li v-for="event in visibleEvents" :key="`${event.kind}-${event.id}`">
        <q-icon
          :class="`metrics-timeline__marker--${event.tone}`"
          class="metrics-timeline__marker"
          :name="toneIcons[event.tone]"
          size="16px">
          <span class="metrics-timeline__sr-only">{{ toneLabels[event.tone] }}</span>
        </q-icon>
        <div>
          <div class="metrics-timeline__event-header">
            <strong>{{ event.title }}</strong>
            <time :datetime="new Date(event.timestampMs).toISOString()">
              {{ formatMetricTimestamp(event.timestampMs) }}
            </time>
          </div>
          <p>{{ event.detail || kindLabels[event.kind] }}</p>
        </div>
      </li>
    </ol>
    <div v-else class="metrics-timeline__empty">
      <q-icon aria-hidden="true" name="history" size="24px" />
      <span>No lifecycle or operation events were recorded in this range.</span>
    </div>

    <q-btn
      v-if="events.length > initialEventCount"
      class="q-mt-sm"
      dense
      flat
      no-caps
      :label="expanded ? 'Show fewer events' : `Show all ${events.length} events`"
      @click="expanded = !expanded" />
  </section>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import type { MetricsTimelineEvent } from '@/pages/game_servers/useGameServerMetrics'
import { formatMetricTimestamp } from '@/pages/game_servers/metrics-format'

const props = defineProps<{ events: MetricsTimelineEvent[] }>()
const initialEventCount = 8
const expanded = ref(false)
const visibleEvents = computed(() =>
  expanded.value ? props.events : props.events.slice(0, initialEventCount),
)

const toneIcons: Record<MetricsTimelineEvent['tone'], string> = {
  positive: 'check_circle',
  warning: 'warning',
  negative: 'error',
  neutral: 'radio_button_unchecked',
}

const toneLabels: Record<MetricsTimelineEvent['tone'], string> = {
  positive: 'Success:',
  warning: 'Warning:',
  negative: 'Failure:',
  neutral: 'Event:',
}

const kindLabels: Record<MetricsTimelineEvent['kind'], string> = {
  lifecycle: 'Lifecycle event',
  operation: 'Server operation',
}
</script>

<style scoped>
.metrics-timeline {
  padding: var(--xy-space-md);
  background: var(--xy-surface-raised-soft);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.metrics-timeline__header,
.metrics-timeline__event-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.metrics-timeline__header h2 {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
}

.metrics-timeline__header p,
.metrics-timeline__event-header + p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.metrics-timeline__header > span,
.metrics-timeline time {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  white-space: nowrap;
}

.metrics-timeline__list {
  display: grid;
  gap: 0;
  padding: 0;
  margin: var(--xy-space-md) 0 0;
  list-style: none;
}

.metrics-timeline__list li {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr);
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) 0;
  border-top: 1px solid var(--xy-border);
}

.metrics-timeline__marker {
  margin-top: var(--xy-space-2xs);
  color: var(--xy-text-muted);
}

.metrics-timeline__marker--positive {
  color: var(--xy-success);
}

.metrics-timeline__marker--warning {
  color: var(--xy-warning);
}

.metrics-timeline__marker--negative {
  color: var(--xy-danger);
}

.metrics-timeline__sr-only {
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

.metrics-timeline__event-header strong {
  color: var(--xy-text-primary);
  font-weight: 600;
}

.metrics-timeline__empty {
  display: flex;
  min-height: 110px;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-muted);
  text-align: center;
}

@media (max-width: 600px) {
  .metrics-timeline__event-header {
    display: block;
  }

  .metrics-timeline time {
    display: block;
    margin-top: var(--xy-space-2xs);
  }
}
</style>
