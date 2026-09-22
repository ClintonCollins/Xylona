<template>
  <div class="xy-empty-state" role="status">
    <q-icon aria-hidden="true" class="xy-empty-state__icon" :name="icon" size="3rem" />
    <p class="xy-empty-state__title">{{ title }}</p>
    <p v-if="description || $slots.default" class="xy-empty-state__copy">
      <slot>{{ description }}</slot>
    </p>
    <div v-if="$slots.actions" class="xy-empty-state__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script lang="ts" setup>
defineProps<{
  /** Material icon name that names the missing object, e.g. `dns`, `search_off`. */
  icon: string
  title: string
  description?: string
}>()
</script>

<style scoped>
.xy-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  padding: var(--xy-space-xl) var(--xy-space-lg);
  text-align: center;
  color: var(--xy-text-secondary);
}

.xy-empty-state__icon {
  margin-bottom: var(--xy-space-sm);
  color: var(--xy-text-muted);
}

.xy-empty-state__title {
  margin: 0;
  font-size: var(--xy-font-size-base);
  font-weight: 600;
  color: var(--xy-text-primary);
}

.xy-empty-state__copy {
  max-width: 48ch;
  margin: var(--xy-space-xs) 0 0;
  font-size: var(--xy-font-size-sm);
  color: var(--xy-text-muted);
}

.xy-empty-state__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-md);
}
</style>
