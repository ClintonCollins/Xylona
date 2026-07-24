<template>
  <q-dialog :model-value="modelValue" @update:model-value="emit('update:modelValue', $event)">
    <q-card class="pm-dialog">
      <q-card-section class="pm-dialog__head">
        <div>
          <div class="pm-dialog__title">Player management</div>
          <div class="pm-dialog__subtitle">
            Roster actions and identifier-based administration for this server.
          </div>
        </div>
        <q-btn v-close-popup aria-label="Close player management" dense flat icon="close" round />
      </q-card-section>
      <q-separator />
      <q-card-section class="pm-dialog__body">
        <player-management-panel :game-server-id="gameServerId" />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import PlayerManagementPanel from './PlayerManagementPanel.vue'

defineProps<{
  modelValue: boolean
  gameServerId: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<style scoped>
.pm-dialog {
  width: min(720px, calc(100vw - 32px));
  max-width: none;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
}

.pm-dialog__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.pm-dialog__title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
}

.pm-dialog__subtitle {
  margin-top: var(--xy-space-2xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}

.pm-dialog__body {
  max-height: min(70vh, 640px);
  overflow-y: auto;
}
</style>
