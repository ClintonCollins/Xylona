<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="delete-files-dialog">
      <q-card-section>
        <div id="dialog-title" class="text-h6 text-error">Delete Files</div>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <p>
              Are you sure you want to delete the following files/directories?
              <span class="text-bold">This action cannot be undone.</span>
            </p>
            <q-scroll-area class="delete-files-list">
              <div
                v-for="file in props.filesToDelete as XylonaFile[]"
                :key="file.name"
                class="delete-file-entry">
                <q-icon
                  :class="file.isDirectory ? 'text-warning' : undefined"
                  :name="
                    file.isDirectory ? tabFolderFilled : getIconFromFilenameExtension(file.name)
                  "
                  :style="
                    file.isDirectory
                      ? undefined
                      : `color:${getColorFromFilenameExtension(file.name)}`
                  "
                  size="xs" />
                <span class="file-name">{{ file.name }}</span>
              </div>
            </q-scroll-area>
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="neutral" :disable="loading" flat label="Cancel" @click="showDialog = false" />
        <q-btn class="bg-error" label="Delete" :loading="loading" @click="deleteFiles" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import {
  File as XylonaFile,
  GameServerFilesDeleteRequest,
  GameServerFilesDeleteRequestSchema,
  GameServerFilesDeleteResponse,
} from '@/proto/gameserver_files_operations_pb'
import {
  getColorFromFilenameExtension,
  getIconFromFilenameExtension,
  GetXylonaClient,
} from '@/utils/shared'
import { tabFolderFilled } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { Ref, ref } from 'vue'

const $q = useQuasar()

const props = defineProps({
  gameServerID: {
    type: String,
    required: true,
  },
  filesToDelete: {
    type: Array<XylonaFile>,
    required: true,
  },
  currentPath: {
    type: String,
    required: true,
  },
  pathSeparator: {
    type: String,
    required: true,
  },
})

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const loading: Ref<boolean> = ref(false)
const emit = defineEmits(['filesDeleted'])

async function deleteFiles() {
  if (loading.value) {
    return
  }
  loading.value = true
  const request: GameServerFilesDeleteRequest = create(GameServerFilesDeleteRequestSchema, {})
  request.gameServerId = props.gameServerID
  request.fullFilePaths = props.filesToDelete.map((file) => {
    if (props.currentPath === '') {
      return file.name
    }
    return props.currentPath + props.pathSeparator + file.name
  })
  try {
    const response: GameServerFilesDeleteResponse =
      await GetXylonaClient().gameServerFilesDelete(request)
    deleteSuccess(response.fullFilePaths)
    emit('filesDeleted', response.fullFilePaths)
    showDialog.value = false
  } catch (e: unknown) {
    console.error(e)
    deleteFailure(e)
  } finally {
    loading.value = false
  }
}

function deleteSuccess(deletedFiles: string[]) {
  $q.notify({
    caption: `Deleted ${deletedFiles.length} files/directories successfully.`,
    type: 'xylona-success',
    position: 'top',
    timeout: 3000,
  })
}

function deleteFailure(err: unknown) {
  $q.notify({
    message:
      err instanceof Error
        ? `Could not delete the selected items. ${err.message}`
        : 'Could not delete the selected items. Try again.',
    type: 'xylona-error',
    position: 'top',
    timeout: 3000,
  })
}
</script>

<style scoped>
.delete-files-dialog {
  width: min(44rem, calc(100dvw - var(--xy-space-xl)));
}

.delete-files-list {
  width: 100%;
  height: 30dvh;
}

.delete-file-entry {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  padding-inline: var(--xy-space-lg);
  font-family: var(--xy-font-mono);
}

.file-name {
  min-width: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 599px) {
  .delete-files-dialog {
    width: calc(100dvw - var(--xy-space-lg));
  }

  .delete-file-entry {
    padding-inline: 0;
  }
}
</style>
