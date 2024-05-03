<template>
  <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
    <q-card class="full-width">
      <q-card-section>
        <q-card-title>
          <div class="text-h6">Extract Files</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-input class="col-12" outlined v-model="extractToFolder" hint="Leave this blank to extract to the current directory." label="Folder to extract files to" :autofocus="true"/>
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="primary" @click="showDialog = false" flat/>
        <q-btn label="Extract" color="primary" @click="submit"/>
      </q-card-actions>
      <q-inner-loading
          :showing="props.loading"
          label="Extracting files..."
          label-class="text-teal"
          label-style="font-size: 1.1em"
      />
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import {QBtn, QCard, QCardSection, QDialog, QInput} from "quasar";

const props = defineProps({
  loading: {
    type: Boolean,
    default: false
  }
})


const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})
const extractToFolder = defineModel('extractToFolder', {
  type: String,
  default: '',
  required: true
})

const emit = defineEmits(['submit'])

function submit() {
  emit('submit')
}

</script>

<style scoped>

</style>
