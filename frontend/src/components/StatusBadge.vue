<template>
  <span class="server-status-badge" :class="badgeClass">
    <span class="status-dot" :class="dotClass"></span>
    {{ label }}
  </span>
</template>

<script setup lang="ts">
import { Status } from '@/proto/shared_pb'
import { computed, PropType } from 'vue'

const props = defineProps({
  status: {
    type: Number as PropType<Status>,
    required: true,
    default: Status.UNKNOWN,
  },
})

const label = computed(() => {
  switch (props.status) {
    case Status.ONLINE:
      return 'ONLINE'
    case Status.OFFLINE:
      return 'OFFLINE'
    case Status.UPDATING:
      return 'UPDATING'
    case Status.INSTALLING:
      return 'INSTALLING'
    default:
      return 'UNKNOWN'
  }
})

const badgeClass = computed(() => {
  switch (props.status) {
    case Status.ONLINE:
      return 'badge-success'
    case Status.OFFLINE:
      return 'badge-danger'
    case Status.UPDATING:
    case Status.INSTALLING:
      return 'badge-warning'
    default:
      return 'badge-neutral'
  }
})

const dotClass = computed(() => {
  switch (props.status) {
    case Status.ONLINE:
      return 'dot-success'
    case Status.OFFLINE:
      return 'dot-danger'
    case Status.UPDATING:
    case Status.INSTALLING:
      return 'dot-warning'
    default:
      return 'dot-neutral'
  }
})
</script>

<style scoped>
.server-status-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  font-family: var(--xy-font-display);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-radius: 999px;
  border: 1px solid;
}

.status-dot {
  width: 8px;
  height: 8px;
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
  box-shadow: 0 0 6px var(--xy-success);
  animation: pulse-success 2s ease-in-out infinite;
}

@keyframes pulse-success {
  0%,
  100% {
    opacity: 1;
    box-shadow: 0 0 6px var(--xy-success);
  }
  50% {
    opacity: 0.7;
    box-shadow: 0 0 2px var(--xy-success);
  }
}

@media (prefers-reduced-motion: reduce) {
  .dot-success {
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
