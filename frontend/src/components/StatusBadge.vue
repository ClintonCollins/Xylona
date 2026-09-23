<template>
  <span :class="badgeClass" class="server-status-badge">
    <span :class="dotClass" class="status-dot"></span>
    {{ label }}
  </span>
</template>

<script lang="ts" setup>
import { Status } from '@/proto/shared_pb'
import { computed, PropType } from 'vue'

/**
 * A lifecycle phase the backend status cannot express: a stop or restart that
 * is in flight, or a start that failed and left the server offline.
 */
export type StatusBadgePhase = 'stopping' | 'restarting' | 'failed'

const props = defineProps({
  status: {
    type: Number as PropType<Status>,
    default: Status.UNKNOWN,
  },
  phase: {
    type: String as PropType<StatusBadgePhase>,
    default: undefined,
  },
})

const tone = computed<'success' | 'danger' | 'warning' | 'neutral'>(() => {
  if (props.phase === 'stopping' || props.phase === 'restarting') return 'warning'
  if (props.phase === 'failed') return 'danger'
  switch (props.status) {
    case Status.ONLINE:
      return 'success'
    case Status.OFFLINE:
      return 'danger'
    case Status.PRE_START:
    case Status.UPDATING:
    case Status.INSTALLING:
      return 'warning'
    default:
      return 'neutral'
  }
})

const label = computed(() => {
  if (props.phase === 'stopping') return 'Stopping'
  if (props.phase === 'restarting') return 'Restarting'
  if (props.phase === 'failed') return 'Start failed'
  switch (props.status) {
    case Status.ONLINE:
      return 'Online'
    case Status.OFFLINE:
      return 'Offline'
    case Status.PRE_START:
      return 'Starting'
    case Status.UPDATING:
      return 'Updating'
    case Status.INSTALLING:
      return 'Installing'
    default:
      return 'Unknown'
  }
})

const badgeClass = computed(() => `badge-${tone.value}`)
const dotClass = computed(() => `dot-${tone.value}`)
</script>

<style scoped>
.server-status-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  font-family: var(--xy-font-control);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-radius: var(--xy-radius-pill);
  border: 1px solid;
}

.status-dot {
  position: relative;
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  flex-shrink: 0;
}

.badge-success {
  background-color: var(--xy-success-bg);
  border-color: var(--xy-success-border);
  color: var(--xy-success);
}

.dot-success {
  background-color: var(--xy-success);
}

.dot-success::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: 50%;
  border: 3px solid var(--xy-success);
  opacity: 0.5;
  animation: pulse-success 2s ease-in-out infinite;
}

@keyframes pulse-success {
  0%,
  100% {
    opacity: 0.5;
    transform: scale(1);
  }
  50% {
    opacity: 0.15;
    transform: scale(0.6);
  }
}

@media (prefers-reduced-motion: reduce) {
  .dot-success::after {
    animation: none;
  }
}

.badge-danger {
  background-color: var(--xy-danger-bg);
  border-color: var(--xy-danger-border);
  color: var(--xy-danger);
}

.dot-danger {
  background-color: var(--xy-danger);
}

.badge-warning {
  background-color: var(--xy-warning-bg);
  border-color: var(--xy-warning-border);
  color: var(--xy-warning);
}

.dot-warning {
  background-color: var(--xy-warning);
}

.badge-neutral {
  background-color: var(--xy-surface-3);
  border-color: var(--xy-border);
  color: var(--xy-text-secondary);
}

.dot-neutral {
  background-color: var(--xy-text-muted);
}
</style>
