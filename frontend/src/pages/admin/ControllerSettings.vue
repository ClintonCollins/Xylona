<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header controller-settings-header">
      <div>
        <h1 class="xy-page-title">Controller Settings</h1>
        <p class="controller-settings-description">
          Configure services owned by this controller and shared across connected nodes.
        </p>
      </div>
    </div>
    <email-settings />
  </q-page>
</template>

<script lang="ts" setup>
import EmailSettings from '@/components/admin/EmailSettings.vue'
</script>

<style scoped>
.controller-settings-header {
  align-items: flex-start;
}

.controller-settings-description {
  max-width: 68ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
}
</style>
