<template>
  <q-card class="xylona-editor">
    <q-card-section>
      <div class="q-pa-md">
        <div class="row justify-end q-gutter-md">
          <q-select
            v-model="editorTheme"
            :options="editorOptions"
            autocomplete="false"
            class="editor-select"
            dense
            emit-value
            label="Theme"
            map-options
            outlined
            @update:model-value="editorThemeChanged" />
          <q-select
            v-model="selectedLanguage"
            :options="LanguageOptions"
            autocomplete="false"
            class="editor-select"
            dense
            emit-value
            label="Language"
            map-options
            outlined
            @update:model-value="editorLanguageChanged" />
        </div>
      </div>
      <div id="editor" ref="editorContainer" class="editor-container"></div>
      <div v-if="saveError" class="editor-save-error" role="alert" aria-live="assertive">
        <q-icon name="error" size="sm" />
        <span>{{ saveError }}</span>
      </div>
    </q-card-section>

    <q-card-actions align="right">
      <q-btn v-close-popup :disable="saving" color="neutral" flat label="Cancel" />
      <q-btn
        :disable="saving"
        :loading="saving"
        class="q-btn bg-main"
        label="Save"
        @click="saveFile" />
    </q-card-actions>
  </q-card>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QCard, useQuasar } from 'quasar'
import loadCustomEditorSettings, {
  getLanguageFromFileName,
  LanguageOptions,
} from '@/components/editor/editor'
import { loadMonacoRuntime } from '@/components/editor/monaco-runtime'
import {
  GameServersFileEditRequest,
  GameServersFileEditRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { GetXylonaClient } from '@/utils/shared'
import { onMounted, onUnmounted, ref } from 'vue'

type IStandaloneCodeEditor = import('monaco-editor').editor.IStandaloneCodeEditor

const $q = useQuasar()

const props = defineProps({
  fileName: {
    type: String,
    required: true,
  },
  gameServerId: {
    type: String,
    required: true,
  },
  fullFilePath: {
    type: String,
    required: true,
  },
  editorTitle: {
    type: String,
    default: 'Editor',
  },
})

const editorTheme = ref('vs-dark')
const saving = ref(false)
const saveError = ref('')
const editorOptions = ref([
  { label: 'Visual Studio', value: 'vs' },
  { label: 'Visual Studio Dark', value: 'vs-dark' },
  { label: 'High Contrast Black', value: 'hc-black' },
])

const emit = defineEmits(['submit'])

const codeInput = defineModel('codeInput', {
  type: String,
  default: '',
})

let editor: IStandaloneCodeEditor | null = null
const editorContainer = ref(null)
const selectedLanguage = ref(getLanguageFromFileName(props.fileName))
let editorStartupTimeout: ReturnType<typeof setTimeout> | null = null
let editorDisposed = false

function editorThemeChanged() {
  if (!editor) {
    return
  }
  editor.updateOptions({ theme: editorTheme.value })
}

async function editorLanguageChanged() {
  if (!editor) {
    return
  }
  const model = editor.getModel()
  if (model) {
    try {
      const monaco = await loadMonacoRuntime(selectedLanguage.value)
      await loadCustomEditorSettings(monaco, selectedLanguage.value)
      monaco.editor.setModelLanguage(model, getLanguageFromFileName(selectedLanguage.value))
    } catch (error) {
      console.error(error)
    }
  }
}

onMounted(() => {
  editorDisposed = false
  // Without this timeout, the entire page will lock up in Chrome and begin leaking memory...
  editorStartupTimeout = setTimeout(() => {
    editorStartupTimeout = null
    void initializeEditor()
  }, 10)
})

onUnmounted(() => {
  editorDisposed = true
  if (editorStartupTimeout) {
    clearTimeout(editorStartupTimeout)
    editorStartupTimeout = null
  }
  if (editor) {
    editor.dispose()
    editor = null
  }
})

async function initializeEditor() {
  if (editorDisposed || editor) {
    return
  }
  if (!editorContainer.value) {
    if (!editorDisposed) {
      console.error('editorContainer is null')
    }
    return
  }

  try {
    const monaco = await loadMonacoRuntime(selectedLanguage.value)
    if (editorDisposed || editor || !editorContainer.value) {
      return
    }
    await loadCustomEditorSettings(monaco, selectedLanguage.value)
    if (editorDisposed || editor || !editorContainer.value) {
      return
    }
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
        snippetsPreventQuickSuggestions: false,
      },
    })
    editor.onDidChangeModelContent(() => {
      if (!editor) {
        return
      }
      codeInput.value = editor.getValue()
    })
  } catch (error) {
    console.error(error)
  }
}

async function saveFile() {
  if (saving.value) {
    return
  }

  saving.value = true
  saveError.value = ''
  try {
    const request: GameServersFileEditRequest = create(GameServersFileEditRequestSchema, {})
    request.content = codeInput.value
    request.fullFilePath = props.fullFilePath
    request.gameServerId = props.gameServerId
    await GetXylonaClient().gameServersFileEdit(request)
    $q.notify({
      caption: `File ${props.fileName} saved successfully.`,
      type: 'xylona-success',
      position: 'top',
      timeout: 3000,
    })
    emit('submit')
  } catch (err) {
    console.error(err)
    saveError.value =
      err instanceof Error
        ? `The file was not saved. ${err.message}`
        : 'The file was not saved. Try again.'
    $q.notify({
      caption: `Error saving file ${props.fileName}.`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.editor-container {
  height: clamp(200px, 55dvh, 70dvh);
  border: 0.1rem solid var(--xy-surface-3);
  border-radius: 0.3rem;
}

.editor-select {
  width: 15rem;
}

.editor-save-error {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: 6px;
  overflow-wrap: anywhere;
}

.xylona-editor {
  min-width: min(60vw, 100%) !important;
  min-height: min(70vh, 80dvh) !important;
  font-family: var(--xy-font-mono) !important;
}
</style>
