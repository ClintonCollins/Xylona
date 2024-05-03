<template>
  <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
    <q-card class="full-width">
      <q-card-section>
        <q-card-title>
          <div class="text-h6">Archive Files</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-select class="col-12" outlined v-model="archiveType" emit-value map-options :options="props.archiveTypeOptions"
                      label="Archive Type">
              <template v-slot:prepend>
                <q-icon name="event"/>
              </template>
            </q-select>
            <q-input class="col-12" outlined v-model="archiveName" label="Name" :autofocus="true"/>
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="primary" @click="showDialog = false" flat/>
        <q-btn label="Archive" color="primary" @click="submit"/>
      </q-card-actions>
      <q-inner-loading
          :showing="props.loading"
          label="Archiving files..."
          label-class="text-teal"
          label-style="font-size: 1.1em"
      />
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import {QBtn, QCard, QCardSection, QDialog, QInput} from "quasar";
import {GameServerFilesCompressionType} from "src/proto/gameserver_files_operations_pb";
import {ArchiveTypeToString} from "src/utils/shared";

const props = defineProps({
  archiveTypeOptions: {
    type: Array<string>,
    default: [
      {label: ArchiveTypeToString(GameServerFilesCompressionType.ZIP), value: GameServerFilesCompressionType.ZIP},
      {label: ArchiveTypeToString(GameServerFilesCompressionType.GZIP), value: GameServerFilesCompressionType.GZIP},
      {label: ArchiveTypeToString(GameServerFilesCompressionType.BZIP2), value: GameServerFilesCompressionType.BZIP2},
      {label: ArchiveTypeToString(GameServerFilesCompressionType.ZST), value: GameServerFilesCompressionType.ZST},
      {label: ArchiveTypeToString(GameServerFilesCompressionType.XZ), value: GameServerFilesCompressionType.XZ},
    ]
  },
  loading: {
    type: Boolean,
    default: false
  }
})


const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})
const archiveName = defineModel('archiveName', {
  type: String,
  default: '',
  required: true
})
const archiveType = defineModel('archiveType', {
  type: Number,
  default: GameServerFilesCompressionType.ZIP,
  required: true
})

const emit = defineEmits(['submit'])

function submit() {
  emit('submit')
}


</script>

<style scoped>

</style>
