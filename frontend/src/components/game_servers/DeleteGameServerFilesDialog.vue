<template>
  <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
    <q-card>
      <q-card-section>
        <q-card-title>
          <div class="text-h6 text-red">Delete Files</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <p>Are you sure you want to delete the following files/directories? <span class="text-bold">This action cannot be undone.</span>
            </p>
            <q-scroll-area style="height: 30dvh;width: 100%;">
              <div class="q-pl-xl" v-for="file in props.filesToDelete as XylonaFile[]" key="name">
                  <span v-if="file.isDirectory">
                    <q-icon size="xs" color="amber" :name="tabFolderFilled" left></q-icon>
                    <span class="file-name">{{ file.name }}</span>
                  </span>
                <span v-else>
                    <q-icon size="xs" :style="'color:'+ getColorFromFilenameExtension(file.name)"
                            :name="getIconFromFilenameExtension(file.name)" left></q-icon>
                    <span class="file-name">{{ file.name }}</span>
                  </span>
              </div>
            </q-scroll-area>
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="neutral" @click="showDialog = false" flat/>
        <q-btn label="Delete" class="bg-error" @click="deleteFiles"/>
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import {QBtn, QCard, QCardSection, QDialog, useQuasar} from "quasar";
import { File as XylonaFile, GameServerFilesDeleteRequestSchema } from '@/proto/gameserver_files_operations_pb'
import {getColorFromFilenameExtension, getIconFromFilenameExtension, GetXylonaClient} from "@/utils/shared";
import {tabFolderFilled} from "quasar-extras-svg-icons/tabler-icons-v2";
import {GameServerFilesDeleteRequest, GameServerFilesDeleteResponse} from "@/proto/gameserver_files_operations_pb";
import {Ref, ref} from "vue";

const $q = useQuasar()

const props = defineProps({
  gameServerID: {
    type: String,
    required: true
  },
  filesToDelete: {
    type: Array<XylonaFile>,
    required: true
  },
  currentPath: {
    type: String,
    required: true
  },
  pathSeparator: {
    type: String,
    required: true
  }
})


const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const loading: Ref<boolean> = ref(false)
const emit = defineEmits(['filesDeleted'])

async function deleteFiles() {
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
    const response: GameServerFilesDeleteResponse = await GetXylonaClient().gameServerFilesDelete(request)
    void deleteSuccess(response.fullFilePaths)
    emit('filesDeleted', response.fullFilePaths)
  } catch (e) {
    console.error(e)
    void deleteFailure(e)
    emit('filesDeleted', [])
  } finally {
    loading.value = false
    showDialog.value = false
  }
}

async function deleteSuccess(deletedFiles: string[]) {
  $q.notify({
    caption: `Deleted ${deletedFiles.length} files/directories successfully.`,
    type: 'xylona-success',
    position: 'top',
    timeout: 3000
  })
}

async function deleteFailure(err: Error) {
  $q.notify({
    message: 'Failed to delete files/directories.\n' + err.message,
    type: 'xylona-error',
    position: 'top',
    timeout: 3000
  })
}

</script>

<style scoped>
.q-dialog__inner--minimized > div {
  max-width: 80dvw;
}
</style>
