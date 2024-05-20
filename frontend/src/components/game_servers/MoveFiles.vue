<template>
    <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
        <q-card class="full-width">
            <q-card-section>
                <q-card-title>
                    <div class="text-h6">Move files</div>
                </q-card-title>
            </q-card-section>
            <q-card-section>
                <q-form class="q-pa-lg">
                    <div class="row wrap q-col-gutter-md justify-between">
                        <q-select class="col-12" outlined v-model="destinationDirectory" emit-value map-options
                                  use-input new-value-mode="add-unique" clearable hide-selected fill-input
                                  hint="Press enter after typing a new directory name to create it and move files to it."
                                  input-debounce="0" :options="moveOptions" label="Destination directory">
                            <template v-slot:prepend>
                                <q-icon name="folder"/>
                            </template>
                        </q-select>
                    </div>
                </q-form>
            </q-card-section>
            <q-card-actions align="right">
                <q-btn label="Cancel" color="primary" @click="showDialog = false" flat/>
                <q-btn label="Submit" color="primary" @click="moveFiles"/>
            </q-card-actions>
        </q-card>
    </q-dialog>
</template>

<script setup lang="ts">
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import { GameServerFilesMoveRequest } from 'src/proto/gameserver_files_operations_pb'
import { File as xylonaFile } from 'src/proto/xylona_pb'
import {
    GetPathSeparator,
    GetRelativeFilePath,
    GetXylonaClient
} from 'src/utils/shared'
import { computed, onMounted, ref, Ref } from 'vue'

const props = defineProps({
    gameServerId: {
        type: String,
        required: true
    },
    gameServerPath: {
        type: String,
        required: true
    },
    path: {
        type: String,
        required: true
    },
    selectedFiles: {
        type: Array<xylonaFile>,
        default: []
    },
    neighboringDirectoriesInPath: {
        type: Array<string>,
        default: []
    }
})

const moveOptions = computed(() => {
    let options = props.neighboringDirectoriesInPath.filter((neighbor: string) => {
        if (props.path === '') {
            return neighbor !== '..'
        }
        return neighbor
    })
    options = options.sort((a: string, b: string) => {
        return a.localeCompare(b)
    })
    return options
})

const destinationDirectory: Ref<string> = ref('')

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
    type: Boolean,
    default: false
})

const emit = defineEmits(['submit'])

function getDestinationDirectory() {
    if (destinationDirectory.value === '..') {
        const pathSeparator = GetPathSeparator(props.gameServerPath)
        let pathSplit = props.path.split(pathSeparator)
        pathSplit.pop()
        return pathSplit.join(pathSeparator)
    }
    return GetRelativeFilePath(props.gameServerPath, props.path, destinationDirectory.value)
}

async function moveFiles() {
    const request = new GameServerFilesMoveRequest()
    request.gameServerId = props.gameServerId
    request.destinationBasePath = getDestinationDirectory()
    request.fullFilePaths = props.selectedFiles.map((file: xylonaFile) => {
        console.log(GetRelativeFilePath(props.gameServerPath, props.path, file.name))
        return GetRelativeFilePath(props.gameServerPath, props.path, file.name)
    })
    try {
        console.log(request.fullFilePaths)
        await GetXylonaClient().gameServerFilesMove(request)
        emit('submit')
        $q.notify({
            caption: `Files moved to ${destinationDirectory.value} successfully.`,
            type: 'positive',
            position: 'top',
            timeout: 3000
        })
    } catch (err) {
        console.error(err)
        emit('submit')
        $q.notify({
            caption: `Error moving files ${err}`,
            type: 'xylona-error',
            position: 'top',
            timeout: 5000
        })
    } finally {
        showDialog.value = false
        destinationDirectory.value = ''
    }
}

</script>

<style scoped>

</style>
