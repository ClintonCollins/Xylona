<script lang="ts" setup>
import { computed, ref, watch } from 'vue'

import type { GameServerBackup } from '@/proto/shared_pb'
import { BackupRestoreMode } from '@/proto/shared_pb'
import { bytesToSize } from '@/utils/shared'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    backup: GameServerBackup | null
    loading?: boolean
    /** Size of the current server directory, when known, to estimate the safety backup. */
    currentSizeBytes?: bigint
    /** Why a restore cannot start right now, such as the server running. */
    blockedReason?: string
    /** Why this server cannot take the safety backup, such as backups being disabled. */
    backupUnavailableReason?: string
  }>(),
  { loading: false, currentSizeBytes: undefined, blockedReason: '', backupUnavailableReason: '' },
)

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  restore: [mode: BackupRestoreMode, backupCurrentFilesFirst: boolean]
}>()

// On by default every time the dialog opens: the user decided the safety
// backup is opt-out, not opt-in. When the server cannot take a backup the
// option starts off, so the default path does not fail after the dialog closes.
const backupFirst = ref(props.backupUnavailableReason === '')
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      backupFirst.value = props.backupUnavailableReason === ''
    }
  },
)

const backupFirstLabel = computed(() =>
  props.currentSizeBytes !== undefined && props.currentSizeBytes > 0n
    ? `Back up current files first (about ${bytesToSize(Number(props.currentSizeBytes))})`
    : 'Back up current files first',
)

function closeDialog(): void {
  if (props.loading) {
    return
  }

  emit('update:modelValue', false)
}

function emitRestore(mode: BackupRestoreMode): void {
  if (props.loading || props.blockedReason !== '') {
    return
  }

  emit('restore', mode, backupFirst.value)
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
  <q-dialog
    :model-value="modelValue"
    aria-labelledby="backup-restore-dialog-title"
    persistent
    @hide="closeDialog">
    <q-card class="backup-restore-dialog">
      <q-card-section class="backup-restore-dialog__header">
        <div id="backup-restore-dialog-title" class="backup-restore-dialog__title">
          Restore Backup
        </div>
        <div class="backup-restore-dialog__subtitle">
          Choose how {{ getArchiveName(backup?.archivePath ?? '') }} should be applied to the
          current server directory.
        </div>
      </q-card-section>

      <q-card-section class="backup-restore-dialog__body">
        <q-banner v-if="blockedReason" class="xy-banner-warning" dense rounded>
          <template #avatar>
            <q-icon name="warning" />
          </template>
          {{ blockedReason }}. Restore replaces live files on disk, so it only runs while the server
          is offline.
        </q-banner>

        <div class="backup-restore-dialog__safety">
          <q-checkbox
            v-model="backupFirst"
            data-testid="restore-backup-first"
            :disable="loading || backupUnavailableReason !== ''"
            :label="backupFirstLabel" />
          <div class="backup-restore-dialog__option-copy">
            <template v-if="backupUnavailableReason">
              Unavailable: {{ backupUnavailableReason }}
            </template>
            <template v-else>
              Saved as a manual backup that is never auto-pruned, so this restore can be undone. The
              restore starts when it finishes.
            </template>
          </div>
          <q-banner v-if="!backupFirst" class="xy-banner-warning" dense rounded>
            <template #avatar>
              <q-icon name="warning" />
            </template>
            Without a backup, the current worlds, configs and mods are overwritten, and Exact
            Restore also deletes files that are not in the archive. There is no way back.
          </q-banner>
        </div>

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
                :disable="blockedReason !== ''"
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
                :disable="blockedReason !== ''"
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
  border-radius: var(--xy-radius-xl);
}

.backup-restore-dialog__header {
  border-bottom: 1px solid var(--xy-border);
}

.backup-restore-dialog__title {
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-lg);
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

.backup-restore-dialog__safety {
  display: grid;
  gap: var(--xy-space-sm);
}

.backup-restore-dialog__option-list {
  display: grid;
  gap: 1rem;
}

.backup-restore-dialog__option {
  background: var(--xy-surface-2);
  border-color: var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.backup-restore-dialog__option-title {
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-base);
  color: var(--xy-text-primary);
}

.backup-restore-dialog__option-copy {
  margin-top: 0.35rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}
</style>
