<template>
    <q-card class="xylona-editor">
        <q-card-section>
            <div class="q-pa-md">
                <div class="row justify-end q-gutter-md">
                    <q-select class="editor-select" dense outlined v-model="editorTheme" map-options emit-value
                              autocomplete="false"
                              :options="editorOptions" @update:model-value="editorThemeChanged" label="Theme"/>
                    <q-select class="editor-select" dense outlined v-model="selectedLanguage" map-options emit-value
                              autocomplete="false"
                              :options="LanguageOptions" @update:model-value="editorLanguageChanged" label="Language"/>
                </div>
            </div>
            <div id="editor" ref="editorContainer" class="editor-container"></div>
        </q-card-section>

        <q-card-actions align="right">
            <q-btn flat label="Cancel" color="neutral" v-close-popup/>
            <q-btn label="Save" class="q-btn bg-main" @click="saveFile" v-close-popup/>
        </q-card-actions>
    </q-card>
</template>

<script setup lang="ts">
import { QCard, useQuasar } from 'quasar'
import loadCustomEditorSettings, { getLanguageFromFileName, LanguageOptions } from 'components/editor/editor'
import { GameServersFileEditRequest, GameServersFileEditResponse } from 'src/proto/gameserver_files_operations_pb'
import { GetXylonaClient } from 'src/utils/shared'
import { onMounted, ref } from 'vue'
import * as monaco from 'monaco-editor'
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import jsonWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker'
import cssWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker'
import htmlWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker'
import tsWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker'
import IStandaloneCodeEditor = monaco.editor.IStandaloneCodeEditor

const $q = useQuasar()

const props = defineProps({
    filePath: {
        type: String,
        required: true
    },
    fileName: {
        type: String,
        required: true
    },
    gameServerId: {
        type: String,
        required: true
    }
})

const editorTheme = ref('vs-dark')
const editorOptions = ref([
    {label: 'Visual Studio', value: 'vs'},
    {label: 'Visual Studio Dark', value: 'vs-dark'},
    {label: 'High Contrast Black', value: 'hc-black'}
])

const codeInput = defineModel('codeInput', {
    type: String,
    default: ''
})

let editor: IStandaloneCodeEditor
const editorContainer = ref(null)
const selectedLanguage = ref(getLanguageFromFileName(props.fileName))

function editorThemeChanged() {
    editor.updateOptions({theme: editorTheme.value})
}

function editorLanguageChanged() {
    const model = editor.getModel()
    if (model) {
        monaco.editor.setModelLanguage(model,
            getLanguageFromFileName(selectedLanguage.value))
    }

}

self.MonacoEnvironment = {
    getWorker(_: any, label: string) {
        if (label === 'json') {
            // noinspection JSPotentiallyInvalidConstructorUsage
            return new jsonWorker()
        }
        if (label === 'css' || label === 'scss' || label === 'less') {
            // noinspection JSPotentiallyInvalidConstructorUsage
            return new cssWorker()
        }
        if (label === 'html' || label === 'handlebars' || label === 'razor') {
            // noinspection JSPotentiallyInvalidConstructorUsage
            return new htmlWorker()
        }
        if (label === 'typescript' || label === 'javascript') {
            // noinspection JSPotentiallyInvalidConstructorUsage
            return new tsWorker()
        }
        // noinspection JSPotentiallyInvalidConstructorUsage
        return new editorWorker()
    }
}

onMounted(() => {
    loadCustomEditorSettings()
    if (editorContainer.value) {
        editor = monaco.editor.create(editorContainer.value, {
            value: codeInput.value,
            language: selectedLanguage.value,
            scrollBeyondLastLine: false,
            theme: 'vs-dark',
            automaticLayout: true,
            suggest: {
                showWords: true,
                showClasses: true,
                showColors: true,
                showFiles: true,
                snippetsPreventQuickSuggestions: false
            }
        })
        editor.onDidChangeModelContent(() => {
            codeInput.value = editor.getValue()
        })
    } else {
        console.error('editorContainer is null')
    }
})

async function saveFile() {
    try {
        const request = new GameServersFileEditRequest()
        request.content = codeInput.value
        request.filePath = props.fileName
        request.gameServerId = props.gameServerId
       await GetXylonaClient().gameServersFileEdit(request)
        $q.notify({
            caption: `File <span class="text-bold">${props.fileName}</span> saved successfully.`,
            type: 'xylona-success',
            html: true,
            position: 'top-right',
            timeout: 3000,
        })
    } catch (err) {
        console.error(err)
        $q.notify({
            caption: `Error saving file ${props.fileName}.`,
            type: 'xylona-error',
            position: 'top-right',
            timeout: 5000,
        })
    } finally {
    }
}

</script>

<style scoped>
.editor-container {
    height: 60dvh;
    border: .1rem solid var(--bg-neutral);
    border-radius: .3rem;
}

.editor-select {
    width: 15rem;
}

.xylona-editor {
    min-width: 60vw !important;
    min-height: 70vh !important;
    font-family: "Oxygen Mono", monospace !important;
}
</style>
