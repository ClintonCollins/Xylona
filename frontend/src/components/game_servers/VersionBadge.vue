<template>
  <span v-if="updateAvailable" class="update-badge">
    <span class="update-badge-arrow">⬆</span>
    Update
  </span>
</template>

<script setup lang="ts">
defineProps<{
  updateAvailable: boolean
}>()
</script>

<style scoped>
.update-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.15rem 0.55rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  color: var(--xy-warning);
  box-shadow: 0 0 8px var(--xy-warning-bg-soft);
  animation: pulse-glow 2.5s ease-in-out infinite;
  letter-spacing: 0.05em;
  cursor: default;
  user-select: none;
}

@keyframes pulse-glow {
  0%,
  100% {
    box-shadow: 0 0 6px var(--xy-warning-bg);
  }
  50% {
    box-shadow: 0 0 14px var(--xy-warning-border);
  }
}

.update-badge-arrow {
  font-size: 0.8rem;
  line-height: 1;
}
</style>
