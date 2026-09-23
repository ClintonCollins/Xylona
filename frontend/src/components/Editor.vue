<template>
  <q-card class="xylona-editor">
    <q-card-section>
      <div class="editor-header">
        <div class="editor-file">
          <q-icon aria-hidden="true" name="description" size="sm" />
          <span class="editor-path" :title="fullFilePath">{{ fullFilePath }}</span>
          <span v-if="dirty" class="editor-dirty">Unsaved changes</span>
        </div>
        <div class="editor-options">
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
      <q-btn :disable="saving" flat label="Cancel" no-caps @click="requestClose" />
      <q-btn
        :disable="saving"
        :loading="saving"
        color="primary"
        label="Save"
        no-caps
        @click="saveFile({ close: true })">
        <q-tooltip>Save ({{ $q.platform.is.mac ? '⌘S' : 'Ctrl+S' }})</q-tooltip>
      </q-btn>
    </q-card-actions>
  </q-card>
</template>

<script lang="ts" setup>
import { QCard, useQuasar } from 'quasar'
import type { editor as MonacoEditor } from 'monaco-editor'

import loadCustomEditorSettings, {
  getLanguageFromFileName,
  LanguageOptions,
} from '@/components/editor/editor'
import { loadMonacoRuntime } from '@/components/editor/monaco-runtime'
import { uploadFormData } from '@/utils/upload'
import { computed, onMounted, onUnmounted, ref } from 'vue'

type IStandaloneCodeEditor = MonacoEditor.IStandaloneCodeEditor

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

// submit: saved from the Save button, close the editor. saved: saved in place with Ctrl/Cmd+S.
const emit = defineEmits(['submit', 'saved', 'close'])

const codeInput = defineModel('codeInput', {
  type: String,
  default: '',
})
const savedContent = ref(codeInput.value)
const dirty = computed(() => codeInput.value !== savedContent.value)

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
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      void saveFile({ close: false })
    })
  } catch (error) {
    console.error(error)
  }
}

function requestClose() {
  if (!dirty.value) {
    emit('close')
    return
  }
  $q.dialog({
    title: 'Unsaved Changes',
    message: `You have unsaved changes to ${props.fileName}. Discard them and close the editor?`,
    cancel: { flat: true, label: 'Keep editing' },
    ok: { color: 'negative', label: 'Discard Changes' },
    persistent: true,
  }).onOk(() => emit('close'))
}

async function saveFile({ close }: { close: boolean }) {
  if (saving.value) {
    return
  }

  saving.value = true
  saveError.value = ''
  try {
    const directory = props.fullFilePath.replaceAll('\\', '/').split('/').slice(0, -1).join('/')
    const formData = new FormData()
    formData.append('gameServerId', props.gameServerId)
    formData.append('path', directory)
    formData.append('file', new File([codeInput.value], props.fileName))
    await uploadFormData('/api/file/upload', formData)
    savedContent.value = codeInput.value
    $q.notify({
      caption: `File ${props.fileName} saved successfully.`,
      type: 'xylona-success',
      position: 'top',
      timeout: 3000,
    })
    emit(close ? 'submit' : 'saved')
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
  border-radius: var(--xy-radius-md);
}

.editor-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm) var(--xy-space-md);
  margin-bottom: var(--xy-space-md);
}

.editor-file {
  display: flex;
  flex: 1 1 16rem;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
  color: var(--xy-text-secondary);
}

.editor-path {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.editor-dirty {
  flex-shrink: 0;
  color: var(--xy-warning);
  font-size: var(--xy-font-size-xs);
}

.editor-options {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.editor-select {
  width: 15rem;
  max-width: 100%;
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
  border-radius: var(--xy-radius-md);
  overflow-wrap: anywhere;
}

.xylona-editor {
  min-width: min(60vw, 100%) !important;
  min-height: min(70vh, 80dvh) !important;
}
</style>
