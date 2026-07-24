<template>
  <header class="xy-page-header">
    <div class="xy-page-header-main">
      <h1 class="xy-page-title">
        <q-icon v-if="icon" aria-hidden="true" class="xy-page-title-icon" :name="icon" />
        {{ title }}
      </h1>
      <div v-if="$slots.default || subtitle" class="xy-page-subtitle">
        <slot>{{ subtitle }}</slot>
      </div>
    </div>
    <div v-if="$slots.actions" class="xy-page-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script lang="ts" setup>
defineProps<{
  title: string
  subtitle?: string
  icon?: string
}>()
</script>

<style scoped>
.xy-page-title-icon {
  margin-right: var(--xy-space-xs);
  color: var(--xy-primary);
  font-size: 0.75em;
  vertical-align: -0.08em;
}

.xy-page-subtitle {
  max-width: 70ch;
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-muted);
}
</style>
