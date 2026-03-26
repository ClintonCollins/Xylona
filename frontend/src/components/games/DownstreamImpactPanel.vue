<template>
  <div class="downstream-impact">
    <div class="downstream-impact__header">
      <div class="font-display downstream-impact__title">Downstream impact</div>
      <div class="text-xy-secondary downstream-impact__copy">
        {{ servers.length }} servers currently use this template.
      </div>
    </div>

    <div v-if="servers.length === 0" class="downstream-impact__empty text-xy-muted">
      No game servers are currently using this game definition.
    </div>

    <div v-else class="downstream-impact__list">
      <article v-for="server in servers" :key="server.name" class="downstream-impact__row">
        <div class="downstream-impact__name">{{ server.name }}</div>
        <div class="downstream-impact__detail text-xy-secondary">
          {{ server.patchCount === 0 ? 'all defaults' : `${server.patchCount} args customized` }}
        </div>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  servers: Array<{
    name: string
    patchCount: number
  }>
}>()
</script>

<style scoped>
.downstream-impact {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.downstream-impact__header {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.downstream-impact__title {
  font-size: 0.95rem;
  color: var(--xy-text-primary);
}

.downstream-impact__copy {
  font-size: 0.82rem;
}

.downstream-impact__empty {
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px dashed var(--xy-border);
  background: var(--xy-surface-0);
}

.downstream-impact__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.downstream-impact__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-radius: 10px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.downstream-impact__name {
  color: var(--xy-text-primary);
}

.downstream-impact__detail {
  font-size: 0.82rem;
}

@media (max-width: 720px) {
  .downstream-impact__row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
