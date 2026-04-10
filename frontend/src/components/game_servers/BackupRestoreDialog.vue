<script lang="ts" setup>
import type { GameServerBackup } from '@/proto/shared_pb'
import { BackupRestoreMode } from '@/proto/shared_pb'

const props = defineProps<{
  modelValue: boolean
  backup: GameServerBackup | null
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  restore: [mode: BackupRestoreMode]
}>()

function closeDialog(): void {
  if (props.loading) {
    return
  }

  emit('update:modelValue', false)
}

function emitRestore(mode: BackupRestoreMode): void {
  if (props.loading) {
    return
  }

  emit('restore', mode)
}

function getArchiveName(archivePath: string): string {
  if (!archivePath) {
    return 'Selected backup'
  }

  const segments = archivePath.split(/[\\/]/)
  const archiveName = segments[segments.length - 1]
  if (!archiveName) {
    return archivePath
  }

  return archiveName
}
</script>

<template>
  <q-dialog :model-value="modelValue" persistent @hide="closeDialog">
    <q-card class="backup-restore-dialog">
      <q-card-section class="backup-restore-dialog__header">
        <div class="backup-restore-dialog__title">Restore Backup</div>
        <div class="backup-restore-dialog__subtitle">
          Choose how {{ getArchiveName(backup?.archivePath ?? '') }} should be applied to the
          current server directory.
        </div>
      </q-card-section>

      <q-card-section class="backup-restore-dialog__body">
        <q-banner class="backup-restore-dialog__banner" dense rounded>
          <template #avatar>
            <q-icon color="warning" name="warning" />
          </template>
          Restore replaces live files on disk. The server must already be offline before restore can
          start.
        </q-banner>

        <div class="backup-restore-dialog__option-list">
          <q-card bordered class="backup-restore-dialog__option" flat>
            <q-card-section>
              <div class="backup-restore-dialog__option-title">Overlay Restore</div>
              <div class="backup-restore-dialog__option-copy">
                Restore the archived files over the current server directory and keep any extra
                files that already exist on disk.
              </div>
            </q-card-section>
            <q-card-actions align="right">
              <q-btn
                :loading="loading"
                color="primary"
                label="Restore As Overlay"
                no-caps
                @click="emitRestore(BackupRestoreMode.OVERLAY)" />
            </q-card-actions>
          </q-card>

          <q-card bordered class="backup-restore-dialog__option" flat>
            <q-card-section>
              <div class="backup-restore-dialog__option-title">Exact Restore</div>
              <div class="backup-restore-dialog__option-copy">
                Make the server directory match the backup exactly by removing files that are not in
                the archive before applying the restore.
              </div>
            </q-card-section>
            <q-card-actions align="right">
              <q-btn
                :loading="loading"
                color="warning"
                label="Restore Exactly"
                no-caps
                @click="emitRestore(BackupRestoreMode.EXACT)" />
            </q-card-actions>
          </q-card>
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn
          :disable="loading"
          color="primary"
          flat
          label="Cancel"
          no-caps
          @click="closeDialog" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.backup-restore-dialog {
  width: min(720px, calc(100vw - 2rem));
  max-width: 100%;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-accent) 8%, transparent), transparent 40%),
    var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 1rem;
}

.backup-restore-dialog__header {
  border-bottom: 1px solid var(--xy-border);
}

.backup-restore-dialog__title {
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
  color: var(--xy-text-primary);
}

.backup-restore-dialog__subtitle {
  margin-top: 0.4rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}

.backup-restore-dialog__body {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.backup-restore-dialog__banner {
  background: color-mix(in srgb, var(--xy-warning) 12%, var(--xy-surface-2));
  color: var(--xy-text-primary);
}

.backup-restore-dialog__option-list {
  display: grid;
  gap: 1rem;
}

.backup-restore-dialog__option {
  background: var(--xy-surface-2);
  border-color: var(--xy-border);
  border-radius: 0.875rem;
}

.backup-restore-dialog__option-title {
  font-family: var(--xy-font-display);
  font-size: 0.95rem;
  color: var(--xy-text-primary);
}

.backup-restore-dialog__option-copy {
  margin-top: 0.35rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}
</style>
