type MonacoModule = typeof import('monaco-editor')
type MonacoWorkerConstructor = new () => Worker

type MonacoWorkerKind = 'editor' | 'json' | 'css' | 'html' | 'ts'

type MonacoEnvironment = {
  getWorker: (_moduleId: string, label: string) => Worker
}

const workerLoaders: Record<MonacoWorkerKind, () => Promise<{ default: MonacoWorkerConstructor }>> =
  {
    editor: () => import('monaco-editor/esm/vs/editor/editor.worker?worker'),
    json: () => import('monaco-editor/esm/vs/language/json/json.worker?worker'),
    css: () => import('monaco-editor/esm/vs/language/css/css.worker?worker'),
    html: () => import('monaco-editor/esm/vs/language/html/html.worker?worker'),
    ts: () => import('monaco-editor/esm/vs/language/typescript/ts.worker?worker'),
  }

const loadedWorkerConstructors: Partial<Record<MonacoWorkerKind, MonacoWorkerConstructor>> = {}
const loadingWorkers = new Map<MonacoWorkerKind, Promise<MonacoWorkerConstructor>>()

let monacoModulePromise: Promise<MonacoModule> | null = null
let environmentInstalled = false

export function getMonacoWorkerKinds(language: string): MonacoWorkerKind[] {
  const workerKinds: MonacoWorkerKind[] = ['editor']

  if (language === 'json') {
    workerKinds.push('json')
    return workerKinds
  }

  if (language === 'css' || language === 'scss' || language === 'less') {
    workerKinds.push('css')
    return workerKinds
  }

  if (language === 'html' || language === 'handlebars' || language === 'razor') {
    workerKinds.push('html')
    return workerKinds
  }

  if (language === 'typescript' || language === 'javascript') {
    workerKinds.push('ts')
  }

  return workerKinds
}

export async function loadMonacoRuntime(language: string): Promise<MonacoModule> {
  installMonacoEnvironment()
  await Promise.all([ensureMonacoWorkers(language), loadMonacoModule()])

  if (!monacoModulePromise) {
    monacoModulePromise = import('monaco-editor')
  }

  return monacoModulePromise
}

export async function ensureMonacoWorkers(language: string): Promise<void> {
  const workerKinds = getMonacoWorkerKinds(language)
  await Promise.all(workerKinds.map((workerKind) => loadMonacoWorker(workerKind)))
}

function installMonacoEnvironment(): void {
  if (environmentInstalled) {
    return
  }

  const globalScope = globalThis as typeof globalThis & {
    MonacoEnvironment?: MonacoEnvironment
  }

  globalScope.MonacoEnvironment = {
    getWorker(_moduleId: string, label: string): Worker {
      const workerKind = resolveWorkerKind(label)
      const workerConstructor =
        loadedWorkerConstructors[workerKind] ?? loadedWorkerConstructors.editor

      if (!workerConstructor) {
        throw new Error(`Monaco worker for "${label}" was requested before it was loaded.`)
      }

      return new workerConstructor()
    },
  }

  environmentInstalled = true
}

async function loadMonacoModule(): Promise<MonacoModule> {
  if (!monacoModulePromise) {
    monacoModulePromise = import('monaco-editor')
  }

  return monacoModulePromise
}

async function loadMonacoWorker(workerKind: MonacoWorkerKind): Promise<MonacoWorkerConstructor> {
  const loadedWorker = loadedWorkerConstructors[workerKind]
  if (loadedWorker) {
    return loadedWorker
  }

  const inFlightWorker = loadingWorkers.get(workerKind)
  if (inFlightWorker) {
    return inFlightWorker
  }

  const workerPromise = workerLoaders[workerKind]()
    .then((workerModule) => {
      loadedWorkerConstructors[workerKind] = workerModule.default
      loadingWorkers.delete(workerKind)
      return workerModule.default
    })
    .catch((error) => {
      loadingWorkers.delete(workerKind)
      throw error
    })

  loadingWorkers.set(workerKind, workerPromise)
  return workerPromise
}

function resolveWorkerKind(label: string): MonacoWorkerKind {
  if (label === 'json') {
    return 'json'
  }

  if (label === 'css' || label === 'scss' || label === 'less') {
    return 'css'
  }

  if (label === 'html' || label === 'handlebars' || label === 'razor') {
    return 'html'
  }

  if (label === 'typescript' || label === 'javascript') {
    return 'ts'
  }

  return 'editor'
}
