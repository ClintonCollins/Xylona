<script lang="ts" setup>
import { computed, ref, watch } from 'vue'

import type { GameServerBackup } from '@/proto/shared_pb'
import { BackupRestoreMode } from '@/proto/shared_pb'
import { useGameServerName } from '@/pages/game_servers/game-server-context'
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

const serverName = useGameServerName()

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
        <h2 id="backup-restore-dialog-title" class="backup-restore-dialog__title">
          Restore Backup
        </h2>
        <div class="backup-restore-dialog__subtitle">
          Choose how {{ getArchiveName(backup?.archivePath ?? '') }} should be applied to
          {{ serverName ? `${serverName}'s` : 'the current' }} server directory.
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

        <!-- Divided rows, not cards: the dialog is already the card. -->
        <div class="backup-restore-dialog__option-list">
          <div class="backup-restore-dialog__option">
            <div>
              <h3 class="backup-restore-dialog__option-title">Overlay Restore</h3>
              <div class="backup-restore-dialog__option-copy">
                Restore the archived files over the current server directory and keep any extra
                files that already exist on disk.
              </div>
            </div>
            <q-btn
              :disable="blockedReason !== ''"
              :loading="loading"
              class="backup-restore-dialog__option-action"
              color="primary"
              label="Restore As Overlay"
              no-caps
              @click="emitRestore(BackupRestoreMode.OVERLAY)" />
          </div>

          <div class="backup-restore-dialog__option">
            <div>
              <h3 class="backup-restore-dialog__option-title">Exact Restore</h3>
              <div class="backup-restore-dialog__option-copy">
                Make the server directory match the backup exactly by removing files that are not in
                the archive before applying the restore.
              </div>
            </div>
            <q-btn
              :disable="blockedReason !== ''"
              :loading="loading"
              class="backup-restore-dialog__option-action"
              color="warning"
              label="Restore Exactly"
              no-caps
              @click="emitRestore(BackupRestoreMode.EXACT)" />
          </div>
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
  margin: 0;
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-lg);
  font-weight: 400;
  line-height: var(--xy-line-height-tight);
  letter-spacing: normal;
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
}

.backup-restore-dialog__option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md) 0;
  border-top: 1px solid var(--xy-border);
}

.backup-restore-dialog__option-action {
  flex-shrink: 0;
}

.backup-restore-dialog__option-title {
  margin: 0;
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-base);
  font-weight: 400;
  line-height: var(--xy-line-height-tight);
  letter-spacing: normal;
  color: var(--xy-text-primary);
}

.backup-restore-dialog__option-copy {
  margin-top: 0.35rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}

@media (max-width: 599px) {
  .backup-restore-dialog__option {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
