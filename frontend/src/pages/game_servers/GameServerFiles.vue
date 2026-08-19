<template>
  <q-card-section class="files-page-section">
    <page-header class="files-page-header" title="Files" />
    <file-uploader-drop
      v-model:file-uploader-dialog="fileUploaderDialog"
      :game-server-id="gameServerId"
      :path="loadedPath"
      :path-separator="pathSeparator"
      :target-element="canMutateFiles ? fileListContainer : null"
      :upload-u-r-l="uploadURL"
      @uploaded-files="listDirectoryFiles(loadedPath)">
      <div ref="fileListContainer" class="file-list-container">
        <div class="file-toolbar">
          <div class="file-toolbar-primary">
            <q-btn
              v-if="canEditFiles"
              :disable="!createButtonEnabled"
              :dense="$q.screen.gt.xs"
              color="primary"
              icon="add"
              label="Create"
              @click="createFilesDialog = true" />
            <q-btn
              v-if="canEditFiles"
              :disable="!canMutateFiles"
              :dense="$q.screen.gt.xs"
              color="positive"
              icon="upload"
              label="Upload"
              @click="fileUploaderDialog = true" />
            <q-btn
              v-if="canEditFiles"
              :aria-label="$q.screen.gt.xs ? undefined : 'Upload from URL'"
              :disable="!canMutateFiles"
              :dense="$q.screen.gt.xs"
              flat
              icon="link"
              :label="$q.screen.gt.xs ? 'URL Upload' : undefined"
              data-testid="open-url-upload-dialog"
              @click="openURLUploadDialog">
              <q-tooltip v-if="!$q.screen.gt.xs">Upload from URL</q-tooltip>
            </q-btn>
            <q-badge v-else class="file-read-only-badge" label="Read only" outline />
          </div>
          <div v-if="selectedFiles.length > 0" class="file-toolbar-selection">
            <span class="file-selection-count"> {{ selectedFiles.length }} selected </span>
            <q-btn
              v-if="downloadButtonEnabled"
              aria-label="Download selected files"
              dense
              flat
              icon="download"
              :loading="downloadingSelected"
              round
              @click="downloadSelectedFiles">
              <q-tooltip>Download</q-tooltip>
            </q-btn>
            <q-btn
              v-if="renameButtonEnabled"
              aria-label="Rename selected item"
              dense
              flat
              icon="edit"
              round
              @click="renameFilesDialog = true">
              <q-tooltip>Rename</q-tooltip>
            </q-btn>
            <q-btn
              v-if="moveButtonEnabled"
              aria-label="Move selected items"
              dense
              flat
              icon="drive_file_move"
              round
              @click="moveFilesDialog = true">
              <q-tooltip>Move</q-tooltip>
            </q-btn>
            <q-btn
              v-if="zipButtonEnabled"
              aria-label="Archive selected items"
              dense
              flat
              icon="archive"
              round
              @click="archiveFilesDialog = true">
              <q-tooltip>Archive</q-tooltip>
            </q-btn>
            <q-btn
              v-if="extractButtonEnabled"
              aria-label="Extract selected archive"
              dense
              flat
              icon="unarchive"
              round
              @click="extractFilesDialog = true">
              <q-tooltip>Extract</q-tooltip>
            </q-btn>
            <q-btn
              v-if="deleteButtonEnabled"
              aria-label="Delete selected items"
              color="negative"
              dense
              flat
              icon="delete"
              round
              @click="deleteFilesDialog = true">
              <q-tooltip>Delete</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Clear selection"
              dense
              flat
              icon="close"
              round
              @click="clearSelection">
              <q-tooltip>Clear selection</q-tooltip>
            </q-btn>
          </div>
          <q-space />
        </div>

        <div class="file-navigation">
          <div class="file-navigation-buttons" aria-label="Directory navigation">
            <q-btn
              aria-label="Back"
              :disable="!canNavigateBack || directoryLoading"
              dense
              flat
              icon="arrow_back"
              round
              @click="navigateHistory(-1)">
              <q-tooltip>Back</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Forward"
              :disable="!canNavigateForward || directoryLoading"
              dense
              flat
              icon="arrow_forward"
              round
              @click="navigateHistory(1)">
              <q-tooltip>Forward</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Up one directory"
              :disable="loadedPath === '' || directoryLoading"
              dense
              flat
              icon="arrow_upward"
              round
              @click="navigateUp">
              <q-tooltip>Up one directory</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Refresh directory"
              :disable="directoryLoading"
              dense
              flat
              icon="refresh"
              round
              @click="refreshFileList">
              <q-tooltip>Refresh</q-tooltip>
            </q-btn>
          </div>

          <div class="file-path-shell">
            <q-input
              v-if="pathEditing"
              v-model="path"
              :prefix="gameServer.directory + pathSeparator"
              aria-label="File path"
              autofocus
              dense
              :loading="directoryLoading"
              outlined
              @update:model-value="clearSelection"
              @keydown.esc.prevent="cancelPathEditing"
              @keydown.enter.prevent="updatePathFromInput">
              <template #append>
                <q-btn
                  aria-label="Open path"
                  dense
                  flat
                  icon="arrow_forward"
                  round
                  @click="updatePathFromInput" />
              </template>
            </q-input>
            <div v-else class="file-path-display">
              <nav aria-label="Current directory" class="file-breadcrumbs">
                <q-btn
                  class="file-breadcrumb-button"
                  dense
                  flat
                  label="Server root"
                  no-caps
                  @click="navigateToPath('')" />
                <template v-for="segment in breadcrumbSegments" :key="segment.path">
                  <q-icon
                    aria-hidden="true"
                    class="file-breadcrumb-separator"
                    name="chevron_right" />
                  <q-btn
                    class="file-breadcrumb-button"
                    dense
                    flat
                    :label="segment.label"
                    no-caps
                    @click="navigateToPath(segment.path)" />
                </template>
              </nav>
              <q-btn aria-label="Edit path" dense flat icon="edit" round @click="startPathEditing">
                <q-tooltip>Edit path</q-tooltip>
              </q-btn>
            </div>
          </div>

          <q-input
            v-model="filterQuery"
            aria-label="Filter files"
            class="file-filter"
            clearable
            debounce="150"
            dense
            outlined
            placeholder="Filter this folder"
            @clear="filterQuery = ''">
            <template #prepend>
              <q-icon name="search" />
            </template>
          </q-input>
        </div>

        <div
          aria-label="Directory contents"
          class="file-list-surface"
          role="table"
          @contextmenu.prevent="openBackgroundContextMenu">
          <div class="file-list-header" role="row">
            <div class="file-list-select-cell" role="columnheader">
              <q-checkbox
                v-model="selectAllFiles"
                aria-label="Select all visible items"
                :disable="!directoryActionsEnabled || selectableEntries.length === 0"
                :indeterminate="someVisibleFilesSelected"
                size="sm" />
            </div>
            <div :aria-sort="sortAria('name')" class="file-list-name-cell" role="columnheader">
              <button class="file-sort-button" type="button" @click="setSort('name')">
                Name
                <q-icon :name="sortIcon('name')" size="xs" />
              </button>
            </div>
            <div :aria-sort="sortAria('size')" class="file-list-size-cell" role="columnheader">
              <button class="file-sort-button" type="button" @click="setSort('size')">
                Size
                <q-icon :name="sortIcon('size')" size="xs" />
              </button>
            </div>
            <div
              :aria-sort="sortAria('modified')"
              class="file-list-modified-cell"
              role="columnheader">
              <button class="file-sort-button" type="button" @click="setSort('modified')">
                Modified
                <q-icon :name="sortIcon('modified')" size="xs" />
              </button>
            </div>
            <div aria-label="Actions" class="file-list-menu-cell" role="columnheader" />
          </div>

          <div
            v-if="directoryLoading"
            aria-live="polite"
            class="file-directory-state"
            role="status">
            <q-spinner color="primary" size="2rem" />
            <div class="text-subtitle1 text-xy-secondary">Loading directory…</div>
          </div>
          <div
            v-else-if="directoryError"
            aria-live="assertive"
            class="file-directory-state"
            role="alert">
            <q-icon class="text-error" name="folder_off" size="2.5rem" />
            <div class="text-subtitle1 text-xy-primary">Could not load this directory</div>
            <div class="file-directory-error text-caption text-xy-secondary">
              {{ directoryError }}
            </div>
            <div class="row q-gutter-sm q-mt-sm">
              <q-btn color="primary" icon="refresh" label="Retry" @click="retryDirectoryLoad" />
              <q-btn
                v-if="path !== loadedPath"
                flat
                icon="undo"
                label="Return to loaded directory"
                @click="returnToLoadedDirectory" />
            </div>
          </div>
          <div
            v-else-if="loadedPath === '' && directories.length === 0 && files.length === 0"
            class="file-empty-state">
            <q-icon class="text-xy-muted q-mb-sm" name="folder_open" size="3rem" />
            <div class="text-subtitle1 text-xy-secondary">This directory is empty</div>
            <div class="text-caption text-xy-muted q-mt-xs">
              {{
                canEditFiles
                  ? 'Upload files or create something new.'
                  : 'There are no files to view.'
              }}
            </div>
          </div>
          <div v-else-if="displayedEntries.length === 0" class="file-empty-state">
            <q-icon class="text-xy-muted q-mb-sm" name="search_off" size="3rem" />
            <div class="text-subtitle1 text-xy-secondary">No matching files</div>
            <div class="text-caption text-xy-muted q-mt-xs">
              Nothing in this folder matches “{{ filterQuery }}”.
            </div>
            <q-btn
              class="q-mt-md"
              flat
              icon="close"
              label="Clear filter"
              @click="filterQuery = ''" />
          </div>
          <q-virtual-scroll
            v-else
            id="file-list"
            ref="fileVirtualScroll"
            class="file-list-scroll"
            :items="displayedEntries"
            role="rowgroup"
            :virtual-scroll-item-size="$q.screen.lt.sm ? 52 : 32">
            <template #default="{ item: entry, index: entryIndex }">
              <div
                :key="entry.name"
                :aria-label="entryAriaLabel(entry)"
                :class="fileIsSelectedClass(entry)"
                class="file-list-body-row"
                :data-file-index="entryIndex"
                data-file-row
                role="row"
                :tabindex="entryTabIndex(entry, entryIndex)"
                @click="selectEntryFromPointer(entry, $event)"
                @contextmenu.prevent.stop="openItemContextMenu($event, entry)"
                @dblclick.prevent="openEntryFromDoubleClick(entry, $event)"
                @focus="focusedEntryName = entry.name"
                @keydown.down.prevent="focusEntryRow(entryIndex + 1)"
                @keydown.enter.prevent="openEntry(entry)"
                @keydown.home.prevent="focusEntryRow(0)"
                @keydown.end.prevent="focusEntryRow(displayedEntries.length - 1)"
                @keydown.up.prevent="focusEntryRow(entryIndex - 1)"
                @keydown.space.prevent="toggleEntrySelection(entry)">
                <div class="file-list-select-cell" role="cell">
                  <q-checkbox
                    v-if="!isParentDirectory(entry)"
                    :aria-label="`Select ${entry.name}`"
                    :model-value="isFileSelected(entry)"
                    size="sm"
                    @click.stop
                    @update:model-value="toggleEntrySelection(entry)" />
                </div>
                <div class="file-list-name-cell" role="cell">
                  <q-icon
                    :class="entry.isDirectory ? 'text-warning' : undefined"
                    :name="
                      entry.isDirectory ? tabFolderFilled : getIconFromFilenameExtension(entry.name)
                    "
                    :style="
                      entry.isDirectory
                        ? undefined
                        : `color:${getColorFromFilenameExtension(entry.name)}`
                    "
                    size="sm" />
                  <div class="file-entry-name-block">
                    <span class="file-name">{{ entry.name }}</span>
                    <span v-if="!isParentDirectory(entry)" class="file-entry-meta">
                      {{ bytesToSize(Number(entry.size)) }} ·
                      {{ formatTimestamp(entry.lastModified) }}
                    </span>
                  </div>
                </div>
                <div class="file-list-size-cell" role="cell">
                  {{ isParentDirectory(entry) ? '' : bytesToSize(Number(entry.size)) }}
                </div>
                <div class="file-list-modified-cell" role="cell">
                  {{ isParentDirectory(entry) ? '' : formatTimestamp(entry.lastModified) }}
                </div>
                <div class="file-list-menu-cell" role="cell">
                  <q-btn
                    v-if="!isParentDirectory(entry)"
                    :aria-label="`Actions for ${entry.name}`"
                    class="file-row-menu"
                    dense
                    flat
                    icon="more_vert"
                    round
                    @click.stop="openItemContextMenu($event, entry)" />
                </div>
              </div>
            </template>
          </q-virtual-scroll>
        </div>

        <q-menu ref="contextMenu" no-parent-event touch-position>
          <q-list class="file-context-menu" dense>
            <q-item-label header>{{ contextMenuLabel }}</q-item-label>
            <template v-if="contextMenuIsBackground">
              <q-item
                v-if="canMutateFiles"
                v-close-popup
                clickable
                @click="createFilesDialog = true">
                <q-item-section avatar><q-icon color="primary" name="add" /></q-item-section>
                <q-item-section>Create…</q-item-section>
              </q-item>
              <q-item
                v-if="canMutateFiles"
                v-close-popup
                clickable
                @click="fileUploaderDialog = true">
                <q-item-section avatar><q-icon color="positive" name="upload" /></q-item-section>
                <q-item-section>Upload</q-item-section>
              </q-item>
              <q-item v-if="canMutateFiles" v-close-popup clickable @click="openURLUploadDialog">
                <q-item-section avatar><q-icon color="info" name="link" /></q-item-section>
                <q-item-section>Upload from URL</q-item-section>
              </q-item>
              <q-separator v-if="canMutateFiles" />
              <q-item v-close-popup clickable @click="refreshFileList">
                <q-item-section avatar><q-icon color="info" name="refresh" /></q-item-section>
                <q-item-section>Refresh</q-item-section>
              </q-item>
              <q-item
                v-if="selectableEntries.length > 0"
                v-close-popup
                clickable
                @click="selectAllFiles = true">
                <q-item-section avatar
                  ><q-icon color="secondary" name="select_all"
                /></q-item-section>
                <q-item-section>Select all</q-item-section>
              </q-item>
            </template>
            <template v-else>
              <q-item v-if="selectedDirectory" v-close-popup clickable @click="openSelectedEntry">
                <q-item-section avatar
                  ><q-icon color="warning" name="folder_open"
                /></q-item-section>
                <q-item-section>Open</q-item-section>
              </q-item>
              <q-item
                v-if="editableSelectedFile"
                v-close-popup
                clickable
                @click="openSelectedEntry">
                <q-item-section avatar><q-icon color="info" name="edit_document" /></q-item-section>
                <q-item-section>Edit</q-item-section>
              </q-item>
              <q-item
                v-if="downloadButtonEnabled"
                v-close-popup
                clickable
                @click="downloadSelectedFiles">
                <q-item-section avatar><q-icon color="positive" name="download" /></q-item-section>
                <q-item-section>Download</q-item-section>
              </q-item>
              <q-separator
                v-if="selectedDirectory || editableSelectedFile || downloadButtonEnabled" />
              <q-item v-close-popup clickable @click="copySelectedPaths(true)">
                <q-item-section avatar
                  ><q-icon color="accent" name="content_copy"
                /></q-item-section>
                <q-item-section>Copy full path</q-item-section>
              </q-item>
              <q-item v-close-popup clickable @click="copySelectedPaths(false)">
                <q-item-section avatar
                  ><q-icon color="accent" name="content_copy"
                /></q-item-section>
                <q-item-section>Copy relative path</q-item-section>
              </q-item>
              <q-separator v-if="renameButtonEnabled || moveButtonEnabled || zipButtonEnabled" />
              <q-item
                v-if="renameButtonEnabled"
                v-close-popup
                clickable
                @click="renameFilesDialog = true">
                <q-item-section avatar
                  ><q-icon color="primary" name="drive_file_rename_outline"
                /></q-item-section>
                <q-item-section>Rename</q-item-section>
              </q-item>
              <q-item
                v-if="moveButtonEnabled"
                v-close-popup
                clickable
                @click="moveFilesDialog = true">
                <q-item-section avatar
                  ><q-icon color="secondary" name="drive_file_move"
                /></q-item-section>
                <q-item-section>Move</q-item-section>
              </q-item>
              <q-item
                v-if="zipButtonEnabled"
                v-close-popup
                clickable
                @click="archiveFilesDialog = true">
                <q-item-section avatar><q-icon color="warning" name="archive" /></q-item-section>
                <q-item-section>Archive</q-item-section>
              </q-item>
              <q-item
                v-if="extractButtonEnabled"
                v-close-popup
                clickable
                @click="extractFilesDialog = true">
                <q-item-section avatar><q-icon color="positive" name="unarchive" /></q-item-section>
                <q-item-section>Extract</q-item-section>
              </q-item>
              <q-separator v-if="deleteButtonEnabled" />
              <q-item
                v-if="deleteButtonEnabled"
                v-close-popup
                class="text-negative"
                clickable
                @click="deleteFilesDialog = true">
                <q-item-section avatar><q-icon color="negative" name="delete" /></q-item-section>
                <q-item-section>Delete</q-item-section>
              </q-item>
            </template>
            <q-separator />
            <q-item v-close-popup clickable>
              <q-item-section avatar
                ><q-icon class="text-xy-secondary" name="close"
              /></q-item-section>
              <q-item-section>Cancel</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </div>
    </file-uploader-drop>
  </q-card-section>
  <q-dialog
    v-model="urlUploadDialog"
    aria-labelledby="url-upload-title"
    backdrop-filter="blur(6px) brightness(15%)"
    persistent>
    <q-card class="file-url-upload-dialog">
      <q-form @submit.prevent="submitURLUpload">
        <q-card-section>
          <div id="url-upload-title" class="text-h6">Upload from URL</div>
          <div class="text-body2 text-xy-secondary q-mt-xs">
            Download a public HTTP or HTTPS file directly to this server.
          </div>
        </q-card-section>
        <q-card-section class="q-pt-none">
          <q-input
            v-model="urlUpload"
            autofocus
            clearable
            data-testid="url-upload-input"
            :disable="urlUploadLoading"
            label="File URL"
            lazy-rules
            outlined
            :rules="[validateUploadURL]"
            type="url" />
          <div class="text-caption text-xy-secondary">
            Destination:
            <span class="file-url-upload-target">{{ urlUploadDestination }}</span>
          </div>
          <div
            v-if="urlUploadError"
            aria-live="assertive"
            class="file-url-upload-error q-mt-md"
            role="alert">
            {{ urlUploadError }}
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn
            :disable="urlUploadLoading"
            flat
            label="Cancel"
            no-caps
            @click="closeURLUploadDialog" />
          <q-btn
            color="primary"
            data-testid="submit-url-upload"
            icon="download"
            label="Upload"
            :loading="urlUploadLoading"
            no-caps
            type="submit" />
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
  <q-dialog v-model="editorModal" backdrop-filter="blur(6px) brightness(15%)" no-shake persistent>
    <editor
      v-model:code-input="editorFileContent"
      :file-name="editorFilename"
      :full-file-path="editorFilePath"
      :game-server-id="gameServerId"
      @submit="editorSaved"></editor>
  </q-dialog>
  <archive-files
    v-model:archive-name="archiveName"
    v-model:show-dialog="archiveFilesDialog"
    :game-server-id="gameServerId"
    :path="loadedPath"
    :path-separator="pathSeparator"
    :selected-files="selectedFiles"
    @cancel="archiveFilesDialog = false"
    @submit="refreshFileList">
  </archive-files>
  <extract-files
    v-model:show-dialog="extractFilesDialog"
    :full-archive-path="
      GetRelativeFilePath(gameServer.directory, loadedPath, selectedFiles[0]?.name)
    "
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :path="loadedPath"
    @cancel="extractFilesDialog = false"
    @submit="refreshFileList">
  </extract-files>
  <create
    v-model:show-dialog="createFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :path="loadedPath"
    @submit="createFilesDialogSubmitted">
  </create>
  <rename-file
    v-model:show-dialog="renameFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :old-file-name="selectedFiles[0]?.name"
    :path="loadedPath"
    @submit="refreshFileList">
  </rename-file>
  <move-files
    v-model:show-dialog="moveFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :neighboring-directories-in-path="directories.map((f) => f.name)"
    :path="loadedPath"
    :selected-files="selectedFiles"
    @submit="refreshFileList">
  </move-files>
  <delete-game-server-files-dialog
    v-model:show-dialog="deleteFilesDialog"
    :current-path="loadedPath"
    :files-to-delete="selectedFiles"
    :game-server-i-d="gameServerId"
    :path-separator="pathSeparator"
    @files-deleted="refreshFileList()">
  </delete-game-server-files-dialog>
</template>

<script lang="ts" setup>
import { create, toJsonString } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import {
  computed,
  defineAsyncComponent,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'
import type { Ref } from 'vue'
import ArchiveFiles from '@/components/game_servers/ArchiveFiles.vue'
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- used in template as <create>
import Create from '@/components/game_servers/Create.vue'
import DeleteGameServerFilesDialog from '@/components/game_servers/DeleteGameServerFilesDialog.vue'
import ExtractFiles from '@/components/game_servers/ExtractFiles.vue'
import FileUploaderDrop from '@/components/game_servers/FileUploaderDrop.vue'
import MoveFiles from '@/components/game_servers/MoveFiles.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import RenameFile from '@/components/game_servers/RenameFile.vue'
import { copyToClipboard, QMenu, QVirtualScroll, useQuasar } from 'quasar'
import { tabFolderFilled } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { GameServer, GameServerSchema } from '@/proto/shared_pb'
import {
  DownloadFileRequest,
  DownloadFileRequestSchema,
  File as xylonaFile,
  FileSchema,
  GameServersFileDownloadFromURLRequest,
  GameServersFileDownloadFromURLRequestSchema,
  ListDirectoryFilesRequest,
  ListDirectoryFilesRequestSchema,
  ListDirectoryFilesResponse,
} from '@/proto/gameserver_files_operations_pb'
import {
  bytesToSize,
  ConnectErrorToString,
  getColorFromFilenameExtension,
  getIconFromFilenameExtension,
  GetRelativeFilePath,
  GetXylonaClient,
} from '@/utils/shared'
import { formatTimestamp } from '@/utils/format-timestamp'
import { useRoute } from 'vue-router'
import { GetGameServerRequest, GetGameServerRequestSchema } from '@/proto/xylona_pb'

const Editor = defineAsyncComponent(() => import('@/components/Editor.vue'))

type SortKey = 'name' | 'size' | 'modified'
type SortDirection = 'ascending' | 'descending'
type HistoryMode = 'push' | 'replace' | 'none'

const $q = useQuasar()
const uploadURL: Ref<string> = ref('/api/file/upload')

const route = useRoute()
const gameServerId: Ref<string> = ref(
  route.params.id instanceof Array ? route.params.id[0] : route.params.id,
)
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const files: Ref<Array<xylonaFile>> = ref([])
const directories: Ref<Array<xylonaFile>> = ref([])
const parentDirectoryEntry: xylonaFile = create(FileSchema, { name: '..', isDirectory: true })
const selectedFiles: Ref<Array<xylonaFile>> = ref([])
const path: Ref<string> = ref('')
const loadedPath: Ref<string> = ref('')
const directoryLoading: Ref<boolean> = ref(true)
const directoryError: Ref<string> = ref('')
const downloadingSelected: Ref<boolean> = ref(false)
const editorModal: Ref<boolean> = ref(false)
const editorFilename: Ref<string> = ref('')
const editorFilePath: Ref<string> = ref('')
const editorFileContent: Ref<string> = ref('')
const contextMenu: Ref<QMenu | null> = ref(null)
const fileVirtualScroll: Ref<QVirtualScroll | null> = ref(null)
const fileListContainer: Ref<HTMLElement | null> = ref(null)
const fileUploaderDialog: Ref<boolean> = ref(false)
const urlUploadDialog: Ref<boolean> = ref(false)
const urlUpload: Ref<string> = ref('')
const urlUploadError: Ref<string> = ref('')
const urlUploadLoading: Ref<boolean> = ref(false)
const pathEditing: Ref<boolean> = ref(false)
const filterQuery: Ref<string> = ref('')
const focusedEntryName: Ref<string> = ref('')
const sortKey: Ref<SortKey> = ref('name')
const sortDirection: Ref<SortDirection> = ref('ascending')
const navigationHistory: Ref<string[]> = ref([])
const navigationHistoryIndex: Ref<number> = ref(-1)
const contextMenuIsBackground: Ref<boolean> = ref(true)
let directoryRequestSequence = 0
let componentUnmounted = false
let selectionAnchorName = ''

// Archive
const archiveFilesDialog: Ref<boolean> = ref(false)
const archiveName: Ref<string> = ref('')

// Extract
const extractFilesDialog: Ref<boolean> = ref(false)

// Create
const createFilesDialog: Ref<boolean> = ref(false)

// Rename
const renameFilesDialog: Ref<boolean> = ref(false)

// Move
const moveFilesDialog: Ref<boolean> = ref(false)

// Delete
const deleteFilesDialog: Ref<boolean> = ref(false)

const allowedExtractExtensions: string[] = ['.zip', '.zst', '.gz', '.bz2', '.xz', '.7z']
const allowedFileEditExtensions: string[] = [
  '.txt',
  '.cfg',
  '.json',
  '.xml',
  '.yml',
  '.yaml',
  '.ini',
  '.log',
  '.properties',
  '.sh',
  '.ps1',
  '.bat',
  '.py',
  '.js',
  '.ts',
]

const pathSeparator = computed(() => {
  if (gameServer.value.directory.indexOf('\\') !== -1) {
    return '\\'
  }
  return '/'
})

const canEditFiles = computed(() => {
  return gameServer.value.effectivePermissions.includes('game_server.files.edit')
})

const directoryActionsEnabled = computed(
  () => !directoryLoading.value && directoryError.value === '' && path.value === loadedPath.value,
)

const canMutateFiles = computed(() => canEditFiles.value && directoryActionsEnabled.value)

const urlUploadDestination = computed(() => {
  const root = gameServer.value.directory.endsWith(pathSeparator.value)
    ? gameServer.value.directory.slice(0, -1)
    : gameServer.value.directory
  return loadedPath.value === '' ? root : root + pathSeparator.value + loadedPath.value
})

const displayedEntries = computed(() => {
  const query = filterQuery.value.trim().toLocaleLowerCase()
  const entries = directories.value.concat(files.value).filter((entry) => {
    return query === '' || entry.name.toLocaleLowerCase().includes(query)
  })

  entries.sort((left, right) => {
    if (left.isDirectory !== right.isDirectory) {
      return left.isDirectory ? -1 : 1
    }

    let comparison: number
    if (sortKey.value === 'name') {
      comparison = left.name.localeCompare(right.name, undefined, {
        numeric: true,
        sensitivity: 'base',
      })
    } else if (sortKey.value === 'size') {
      comparison = left.size < right.size ? -1 : left.size > right.size ? 1 : 0
    } else {
      const leftSeconds = left.lastModified?.seconds ?? 0n
      const rightSeconds = right.lastModified?.seconds ?? 0n
      comparison = leftSeconds < rightSeconds ? -1 : leftSeconds > rightSeconds ? 1 : 0
      if (comparison === 0) {
        comparison = (left.lastModified?.nanos ?? 0) - (right.lastModified?.nanos ?? 0)
      }
    }

    return sortDirection.value === 'ascending' ? comparison : -comparison
  })

  return loadedPath.value === '' ? entries : [parentDirectoryEntry, ...entries]
})

const selectableEntries = computed(() => {
  return displayedEntries.value.filter((entry) => !isParentDirectory(entry))
})

const selectAllFiles = computed({
  get() {
    return (
      selectableEntries.value.length > 0 &&
      selectableEntries.value.every((entry) => isFileSelected(entry))
    )
  },
  set(selected: boolean) {
    selectedFiles.value = selected ? [...selectableEntries.value] : []
    selectionAnchorName = selected ? (selectableEntries.value.at(-1)?.name ?? '') : ''
  },
})

const someVisibleFilesSelected = computed(() => {
  return !selectAllFiles.value && selectableEntries.value.some((entry) => isFileSelected(entry))
})

const breadcrumbSegments = computed(() => {
  const segments = normalizeDirectoryPath(path.value).split(pathSeparator.value).filter(Boolean)
  return segments.map((label, index) => ({
    label,
    path: segments.slice(0, index + 1).join(pathSeparator.value),
  }))
})

const canNavigateBack = computed(() => navigationHistoryIndex.value > 0)
const canNavigateForward = computed(
  () =>
    navigationHistoryIndex.value >= 0 &&
    navigationHistoryIndex.value < navigationHistory.value.length - 1,
)

const selectedDirectory = computed(() => {
  const selected = sanitizeSelectedFiles()
  return selected.length === 1 && selected[0].isDirectory ? selected[0] : undefined
})

const editableSelectedFile = computed(() => {
  const selected = sanitizeSelectedFiles()
  if (selected.length !== 1 || selected[0].isDirectory || !canMutateFiles.value) {
    return undefined
  }
  return isEditableFile(selected[0]) ? selected[0] : undefined
})

const contextMenuLabel = computed(() => {
  if (contextMenuIsBackground.value) {
    return loadedPath.value === '' ? 'Server root' : loadedPath.value
  }
  const selected = sanitizeSelectedFiles()
  return selected.length === 1 ? selected[0].name : `${selected.length} items selected`
})

async function refreshFileList() {
  await listDirectoryFiles(loadedPath.value, 'none')
}

async function editorSaved() {
  editorModal.value = false
  await refreshFileList()
}

async function createFilesDialogSubmitted(
  success: boolean,
  data: {
    fileName: string
    fullFilePath: string
    isDir: boolean
  } | null,
) {
  await listDirectoryFiles(loadedPath.value)
  createFilesDialog.value = false

  if (!success) {
    return
  }
  if (!data || data.isDir) {
    return
  }
  editorFilename.value = data.fileName
  editorFileContent.value = ''
  editorFilePath.value = data.fullFilePath
  editorModal.value = true
}

function fileIsSelectedClass(file: xylonaFile) {
  return { 'file-list-body-row--selected': isFileSelected(file) }
}

function entryAriaLabel(entry: xylonaFile): string {
  if (isParentDirectory(entry)) {
    return 'Parent directory'
  }
  const entryType = entry.isDirectory ? 'folder' : 'file'
  const selectedState = isFileSelected(entry) ? 'selected' : 'not selected'
  if (entry.isDirectory) {
    return `${entry.name}, ${entryType}, ${selectedState}`
  }
  return `${entry.name}, ${entryType}, ${bytesToSize(Number(entry.size))}, modified ${formatTimestamp(entry.lastModified)}, ${selectedState}`
}

function entryTabIndex(entry: xylonaFile, entryIndex: number): 0 | -1 {
  const focusedEntryVisible = displayedEntries.value.some(
    (visibleEntry) => visibleEntry.name === focusedEntryName.value,
  )
  return focusedEntryVisible
    ? entry.name === focusedEntryName.value
      ? 0
      : -1
    : entryIndex === 0
      ? 0
      : -1
}

async function focusEntryRow(targetIndex: number) {
  const boundedIndex = Math.max(0, Math.min(targetIndex, displayedEntries.value.length - 1))
  const targetEntry = displayedEntries.value[boundedIndex]
  if (!targetEntry) {
    return
  }
  focusedEntryName.value = targetEntry.name
  fileVirtualScroll.value?.scrollTo(boundedIndex, 'center')
  await nextTick()
  fileListContainer.value
    ?.querySelector<HTMLElement>(`[data-file-index="${boundedIndex}"]`)
    ?.focus()
}

const deleteButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return canMutateFiles.value && selected.length > 0
})

const downloadButtonEnabled = computed(() => {
  if (!directoryActionsEnabled.value) {
    return false
  }
  const selected = sanitizeSelectedFiles()
  if (selected.length <= 0) {
    return false
  }
  for (let i = 0; i < selected.length; i++) {
    if (selected[i].isDirectory) {
      return false
    }
  }
  return true
})

const zipButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return canMutateFiles.value && selected.length > 0
})

const createButtonEnabled = computed(() => {
  return canMutateFiles.value
})

const renameButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return canMutateFiles.value && selected.length === 1
})

const moveButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return canMutateFiles.value && selected.length > 0
})

const extractButtonEnabled = computed(() => {
  if (!canMutateFiles.value) {
    return false
  }
  const selected = sanitizeSelectedFiles()
  if (selected.length !== 1 || selected[0].isDirectory) {
    return false
  }
  return isExtractableFile(selected[0])
})

function clearSelection() {
  selectedFiles.value = []
  selectionAnchorName = ''
}

function sanitizeSelectedFiles(): xylonaFile[] {
  return [...selectedFiles.value]
}

function isFileSelected(file: xylonaFile): boolean {
  return selectedFiles.value.some((selected) => selected.name === file.name)
}

function isEditableFile(file: xylonaFile): boolean {
  const extension = file.name.substring(file.name.lastIndexOf('.')).toLocaleLowerCase()
  return allowedFileEditExtensions.includes(extension)
}

function isExtractableFile(file: xylonaFile): boolean {
  const fileName = file.name.toLocaleLowerCase()
  return allowedExtractExtensions.some((extension) => fileName.endsWith(extension))
}

onMounted(async () => {
  await getGameServerDetails()
  if (componentUnmounted) {
    return
  }
  window.addEventListener('hashchange', handleHashNavigation)
  path.value = pathFromLocationHash()
  await listDirectoryFiles(path.value, 'replace')
})

onBeforeUnmount(() => {
  componentUnmounted = true
  window.removeEventListener('hashchange', handleHashNavigation)
})

async function handleHashNavigation() {
  const hashPath = pathFromLocationHash()
  if (hashPath === path.value && hashPath === loadedPath.value) {
    return
  }

  const previousIndex = navigationHistoryIndex.value - 1
  const nextIndex = navigationHistoryIndex.value + 1
  let matchingIndex = -1
  if (navigationHistory.value[previousIndex] === hashPath) {
    matchingIndex = previousIndex
  } else if (navigationHistory.value[nextIndex] === hashPath) {
    matchingIndex = nextIndex
  }

  const loaded = await listDirectoryFiles(hashPath, 'none')
  if (!loaded) {
    return
  }
  if (matchingIndex >= 0) {
    navigationHistoryIndex.value = matchingIndex
  } else {
    commitNavigationHistory(hashPath, 'push')
  }
}

watch(filterQuery, () => {
  const visibleNames = new Set(selectableEntries.value.map((entry) => entry.name))
  selectedFiles.value = selectedFiles.value.filter((entry) => visibleNames.has(entry.name))
  if (!visibleNames.has(selectionAnchorName)) {
    selectionAnchorName = ''
  }
  if (!visibleNames.has(focusedEntryName.value)) {
    focusedEntryName.value = ''
  }
})

function selectEntryFromPointer(entry: xylonaFile, event: MouseEvent) {
  const pointerType = 'pointerType' in event ? (event as PointerEvent).pointerType : ''
  const modified = event.ctrlKey || event.metaKey || event.shiftKey
  if (isParentDirectory(entry)) {
    void openEntry(entry)
    return
  }
  if (entry.isDirectory && !modified) {
    void openEntry(entry)
    return
  }
  if (pointerType === 'touch' || ($q.platform.is.mobile && $q.platform.has.touch)) {
    void openEntry(entry)
    return
  }

  selectEntry(entry, event)
}

function selectEntry(entry: xylonaFile, event: MouseEvent) {
  if (!directoryActionsEnabled.value || isParentDirectory(entry)) {
    return
  }

  const additive = event.ctrlKey || event.metaKey
  if (event.shiftKey && selectionAnchorName !== '') {
    const anchorIndex = selectableEntries.value.findIndex(
      (file) => file.name === selectionAnchorName,
    )
    const entryIndex = selectableEntries.value.findIndex((file) => file.name === entry.name)
    if (anchorIndex >= 0 && entryIndex >= 0) {
      const start = Math.min(anchorIndex, entryIndex)
      const end = Math.max(anchorIndex, entryIndex)
      const range = selectableEntries.value.slice(start, end + 1)
      selectedFiles.value = additive ? uniqueFiles(selectedFiles.value.concat(range)) : range
      return
    }
  }

  if (additive) {
    toggleEntrySelection(entry)
    return
  }

  selectedFiles.value = [entry]
  selectionAnchorName = entry.name
}

function toggleEntrySelection(entry: xylonaFile) {
  if (!directoryActionsEnabled.value || isParentDirectory(entry)) {
    return
  }
  if (isFileSelected(entry)) {
    selectedFiles.value = selectedFiles.value.filter((selected) => selected.name !== entry.name)
  } else {
    selectedFiles.value = selectedFiles.value.concat(entry)
  }
  selectionAnchorName = entry.name
}

function uniqueFiles(entries: xylonaFile[]): xylonaFile[] {
  return Array.from(new Map(entries.map((entry) => [entry.name, entry])).values())
}

function openEntryFromDoubleClick(entry: xylonaFile, event: MouseEvent) {
  const pointerType = 'pointerType' in event ? (event as PointerEvent).pointerType : ''
  if (
    entry.isDirectory ||
    pointerType === 'touch' ||
    ($q.platform.is.mobile && $q.platform.has.touch)
  ) {
    return
  }
  void openEntry(entry)
}

async function openEntry(entry: xylonaFile) {
  if (entry.isDirectory) {
    await clickDirectory(entry)
    return
  }
  await clickFile(entry)
}

function openItemContextMenu(event: Event, entry: xylonaFile) {
  if (!directoryActionsEnabled.value || isParentDirectory(entry)) {
    return
  }
  if (!isFileSelected(entry)) {
    selectedFiles.value = [entry]
  }
  selectionAnchorName = entry.name
  contextMenuIsBackground.value = false
  contextMenu.value?.show?.(event)
}

function openBackgroundContextMenu(event: Event) {
  if (!directoryActionsEnabled.value) {
    return
  }
  clearSelection()
  contextMenuIsBackground.value = true
  contextMenu.value?.show?.(event)
}

function openSelectedEntry() {
  const selected = sanitizeSelectedFiles()
  if (selected.length === 1) {
    void openEntry(selected[0])
  }
}

async function clickDirectory(directory: xylonaFile) {
  if (!directoryActionsEnabled.value) {
    return
  }

  const nextPath = isParentDirectory(directory)
    ? loadedPath.value.split(pathSeparator.value).slice(0, -1).join(pathSeparator.value)
    : GetRelativeFilePath(gameServer.value.directory, loadedPath.value, directory.name)

  path.value = normalizeDirectoryPath(nextPath)
  await listDirectoryFiles(path.value, 'push')
}

function isParentDirectory(entry: xylonaFile): boolean {
  return entry.isDirectory && entry.name === '..'
}

async function clickFile(file: xylonaFile) {
  if (!directoryActionsEnabled.value) {
    return
  }
  if (canEditFiles.value && isEditableFile(file)) {
    await readFileOctetStream(file.name)
    return
  }
  await downloadGameServerFile(file.name)
}

function updatePathFromInput() {
  path.value = normalizeDirectoryPath(path.value)
  pathEditing.value = false
  void listDirectoryFiles(path.value, 'push')
}

function startPathEditing() {
  path.value = loadedPath.value
  pathEditing.value = true
}

function cancelPathEditing() {
  path.value = loadedPath.value
  pathEditing.value = false
}

function navigateToPath(directoryPath: string) {
  if (directoryLoading.value || normalizeDirectoryPath(directoryPath) === loadedPath.value) {
    return
  }
  void listDirectoryFiles(directoryPath, 'push')
}

function navigateUp() {
  if (directoryLoading.value || loadedPath.value === '') {
    return
  }
  const segments = loadedPath.value.split(pathSeparator.value)
  segments.pop()
  void listDirectoryFiles(segments.join(pathSeparator.value), 'push')
}

function openURLUploadDialog() {
  urlUpload.value = ''
  urlUploadError.value = ''
  urlUploadDialog.value = true
}

function closeURLUploadDialog() {
  if (urlUploadLoading.value) {
    return
  }
  urlUploadDialog.value = false
}

function validateUploadURL(value: string): true | string {
  try {
    const parsedURL = new URL(value.trim())
    return parsedURL.protocol === 'http:' || parsedURL.protocol === 'https:'
      ? true
      : 'Enter an HTTP or HTTPS URL.'
  } catch {
    return 'Enter a valid file URL.'
  }
}

async function submitURLUpload() {
  if (urlUploadLoading.value) {
    return
  }

  const rawURL = urlUpload.value.trim()
  const validationResult = validateUploadURL(rawURL)
  if (validationResult !== true) {
    urlUploadError.value = validationResult
    return
  }

  const operationPath = loadedPath.value
  const request: GameServersFileDownloadFromURLRequest = create(
    GameServersFileDownloadFromURLRequestSchema,
    {
      destinationBasePath: operationPath,
      gameServerId: gameServerId.value,
      url: rawURL,
    },
  )
  urlUploadLoading.value = true
  urlUploadError.value = ''
  try {
    const response = await GetXylonaClient().gameServerFilesDownloadFromURL(request)
    const fileName = response.filePath.split(/[\\/]/).pop() || 'File'
    urlUploadDialog.value = false
    urlUpload.value = ''
    $q.notify({
      message: `${fileName} uploaded from URL.`,
      position: 'top',
      type: 'xylona-success',
    })
    if (loadedPath.value === operationPath && path.value === operationPath) {
      await listDirectoryFiles(operationPath, 'none')
    }
  } catch (unknownError: unknown) {
    urlUploadError.value = ConnectErrorToString(ConnectError.from(unknownError))
  } finally {
    urlUploadLoading.value = false
  }
}

async function navigateHistory(offset: -1 | 1) {
  const targetIndex = navigationHistoryIndex.value + offset
  const targetPath = navigationHistory.value[targetIndex]
  if (directoryLoading.value || targetPath === undefined) {
    return
  }
  const loaded = await listDirectoryFiles(targetPath, 'none')
  if (loaded) {
    navigationHistoryIndex.value = targetIndex
  }
}

function commitNavigationHistory(directoryPath: string, mode: HistoryMode) {
  if (mode === 'none') {
    return
  }
  if (mode === 'replace' || navigationHistoryIndex.value < 0) {
    navigationHistory.value = [directoryPath]
    navigationHistoryIndex.value = 0
    return
  }
  if (navigationHistory.value[navigationHistoryIndex.value] === directoryPath) {
    return
  }

  navigationHistory.value = navigationHistory.value.slice(0, navigationHistoryIndex.value + 1)
  navigationHistory.value.push(directoryPath)
  navigationHistoryIndex.value = navigationHistory.value.length - 1
}

async function listDirectoryFiles(
  directoryPath: string,
  historyMode: HistoryMode = 'push',
): Promise<boolean> {
  const normalizedPath = normalizeDirectoryPath(directoryPath)
  const requestSequence = ++directoryRequestSequence
  const navigating = normalizedPath !== loadedPath.value
  clearSelection()
  contextMenu.value?.hide?.()
  fileUploaderDialog.value = false
  urlUploadDialog.value = false
  if (navigating) {
    filterQuery.value = ''
  }
  directoryLoading.value = true
  directoryError.value = ''

  const request: ListDirectoryFilesRequest = create(ListDirectoryFilesRequestSchema, {})
  try {
    request.gameServerId = gameServerId.value
    request.path = normalizedPath
    const response: ListDirectoryFilesResponse = await GetXylonaClient().listDirectoryFiles(request)
    if (requestSequence !== directoryRequestSequence) {
      return false
    }

    directories.value = response.files.filter((file) => file.isDirectory)
    files.value = response.files.filter((file) => !file.isDirectory)
    loadedPath.value = normalizedPath
    path.value = normalizedPath
    pathEditing.value = false
    syncLocationHash(normalizedPath)
    commitNavigationHistory(normalizedPath, historyMode)
    return true
  } catch (err) {
    if (requestSequence !== directoryRequestSequence) {
      return false
    }

    if (err instanceof ConnectError) {
      if (err.code === Code.NotFound) {
        directoryError.value = 'The requested directory was not found.'
        return false
      }
      console.error(`Error listing directory files: ${err.code} ${err.message}`)
      directoryError.value = err.message || 'The directory listing request failed.'
      return false
    }
    console.error(err)
    directoryError.value = err instanceof Error ? err.message : 'The directory listing failed.'
    return false
  } finally {
    if (requestSequence === directoryRequestSequence) {
      directoryLoading.value = false
    }
  }
}

function normalizeDirectoryPath(directoryPath: string): string {
  const segments = directoryPath.split(/[\\/]+/)
  const normalizedSegments: string[] = []

  for (const segment of segments) {
    if (segment === '' || segment === '.') {
      continue
    }
    if (segment === '..') {
      normalizedSegments.pop()
      continue
    }
    normalizedSegments.push(segment)
  }

  return normalizedSegments.join(pathSeparator.value)
}

function pathFromLocationHash(): string {
  const encodedSegments = window.location.hash.substring(1).split('/').filter(Boolean)
  const decodedSegments = encodedSegments.map((segment) => {
    try {
      return decodeURIComponent(segment)
    } catch {
      return segment
    }
  })
  return normalizeDirectoryPath(decodedSegments.join(pathSeparator.value))
}

function syncLocationHash(directoryPath: string) {
  const encodedPath = normalizeDirectoryPath(directoryPath)
    .split(pathSeparator.value)
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  const baseURL = window.location.pathname + window.location.search
  const nextURL = encodedPath === '' ? baseURL : `${baseURL}#${encodedPath}`
  window.history.replaceState(window.history.state, '', nextURL)
}

function retryDirectoryLoad() {
  void listDirectoryFiles(path.value, 'push')
}

function returnToLoadedDirectory() {
  clearSelection()
  path.value = loadedPath.value
  directoryError.value = ''
  directoryLoading.value = false
  pathEditing.value = false
  syncLocationHash(loadedPath.value)
}

function setSort(nextSortKey: SortKey) {
  if (sortKey.value === nextSortKey) {
    sortDirection.value = sortDirection.value === 'ascending' ? 'descending' : 'ascending'
    return
  }
  sortKey.value = nextSortKey
  sortDirection.value = 'ascending'
}

function sortIcon(column: SortKey): string {
  if (sortKey.value !== column) {
    return 'unfold_more'
  }
  return sortDirection.value === 'ascending' ? 'arrow_upward' : 'arrow_downward'
}

function sortAria(column: SortKey): 'none' | SortDirection {
  return sortKey.value === column ? sortDirection.value : 'none'
}

async function copySelectedPaths(fullPath: boolean) {
  const paths = sanitizeSelectedFiles().map((file) => {
    const relativePath = GetRelativeFilePath(
      gameServer.value.directory,
      loadedPath.value,
      file.name,
    )
    if (!fullPath) {
      return relativePath
    }
    const rootPath = gameServer.value.directory
    if (rootPath === '' || rootPath.endsWith(pathSeparator.value)) {
      return rootPath + relativePath
    }
    return rootPath + pathSeparator.value + relativePath
  })

  try {
    await copyToClipboard(paths.join('\n'))
    $q.notify({
      message: `${fullPath ? 'Full' : 'Relative'} ${paths.length === 1 ? 'path' : 'paths'} copied.`,
      position: 'top',
      type: 'xylona-success',
    })
  } catch (error) {
    console.error(error)
    $q.notify({
      message: 'Could not copy the selected paths.',
      position: 'top',
      type: 'xylona-error',
    })
  }
}

async function readFileOctetStream(fileName: string) {
  $q.loading.show({
    message: 'Reading file...',
    delay: 100,
  })
  const operationPath = loadedPath.value
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, operationPath, fileName)
  const fileRequest: DownloadFileRequest = create(DownloadFileRequestSchema, {})
  fileRequest.gameServerId = gameServerId.value
  fileRequest.path = fullFilePath
  try {
    const response = await fetch('/api/file/get', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: toJsonString(DownloadFileRequestSchema, fileRequest),
    })
    if (!response.ok) {
      throw new Error(response.statusText || `File request failed with status ${response.status}`)
    }
    const data = await response.text()
    if (loadedPath.value !== operationPath || path.value !== operationPath) {
      return
    }
    editorFilename.value = fileName
    editorFileContent.value = data
    editorFilePath.value = fullFilePath
    editorModal.value = true
  } catch (e) {
    console.error(e)
    $q.notify({
      caption: `Error reading file ${fileName}.`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    $q.loading.hide()
  }
}

async function downloadSelectedFiles() {
  if (downloadingSelected.value || !downloadButtonEnabled.value) {
    return
  }

  const operationPath = loadedPath.value
  const filesToDownload = sanitizeSelectedFiles()
  downloadingSelected.value = true
  try {
    for (const file of filesToDownload) {
      await downloadGameServerFile(file.name, operationPath)
    }
  } finally {
    downloadingSelected.value = false
  }
}

async function downloadGameServerFile(fileName: string, operationPath = loadedPath.value) {
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, operationPath, fileName)
  const encodedGameServerID = encodeURIComponent(gameServerId.value)
  const encodedFilePath = encodeURIComponent(fullFilePath)
  const rawURL = `${window.location.protocol}//${window.location.host}/api/file/download/${encodedGameServerID}/${encodedFilePath}`
  const url = encodeURI(rawURL)
  // If the URL will fit in a GET request, without hitting browser URL length limits, use GET.
  if (url.length < 2000) {
    const a = document.createElement('a')
    a.href = url
    a.download = fileName
    a.target = '_blank'
    a.style.display = 'none'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    return
  }
  // If the URL is too long for a GET request, use POST.
  try {
    const downloadForm = document.createElement('form')
    downloadForm.method = 'POST'
    downloadForm.action = '/api/file/download'
    downloadForm.style.display = 'none'

    const gameServerIDInput = document.createElement('input')
    gameServerIDInput.name = 'gameServerId'
    gameServerIDInput.value = gameServerId.value
    downloadForm.appendChild(gameServerIDInput)

    const filePathInput = document.createElement('input')
    filePathInput.name = 'path'
    filePathInput.value = fullFilePath
    downloadForm.appendChild(filePathInput)

    downloadForm.target = '_blank'
    document.body.appendChild(downloadForm)
    downloadForm.submit()

    document.body.removeChild(downloadForm)
  } catch (e) {
    console.error(e)
    $q.notify({
      caption: `Error downloading file ${fileName}.`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  }
}

async function getGameServerDetails() {
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }
    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
  }
}
</script>

<style scoped>
.files-page-header {
  margin-bottom: var(--xy-space-sm);
}

.files-page-section,
.files-page-section :deep(.file-uploader-drop-zone) {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.file-list-container {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  font-family: var(--xy-font-mono);
}

.file-url-upload-dialog {
  width: min(34rem, calc(100vw - var(--xy-space-xl)));
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.file-url-upload-target {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  overflow-wrap: anywhere;
}

.file-url-upload-error {
  padding: var(--xy-space-sm) var(--xy-space-base);
  color: var(--xy-danger-hover);
  background-color: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
}

.file-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 2.75rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background-color: var(--xy-surface-2);
  border-bottom: 1px solid var(--xy-border);
}

.file-toolbar-primary,
.file-toolbar-selection,
.file-navigation-buttons,
.file-path-display,
.file-breadcrumbs {
  display: flex;
  align-items: center;
}

.file-toolbar-primary,
.file-toolbar-selection {
  gap: var(--xy-space-xs);
}

.file-toolbar-selection {
  padding-inline-start: var(--xy-space-sm);
  border-inline-start: 1px solid var(--xy-border);
}

.file-selection-count {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  white-space: nowrap;
}

.file-read-only-badge {
  color: var(--xy-text-secondary);
  border-color: var(--xy-border-hover);
}

.file-navigation {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background-color: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.file-navigation-buttons {
  flex: 0 0 auto;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-2xs);
  background-color: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.file-path-shell {
  flex: 1 1 20rem;
  min-width: 12rem;
}

.file-path-display {
  min-height: 2.25rem;
  padding: 0 var(--xy-space-xs) 0 var(--xy-space-sm);
  background-color: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.file-breadcrumbs {
  flex: 1;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: thin;
  white-space: nowrap;
}

.file-breadcrumb-button {
  flex: 0 0 auto;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
}

.file-breadcrumb-button:last-of-type {
  color: var(--xy-text-primary);
}

.file-breadcrumb-separator {
  flex: 0 0 auto;
  color: var(--xy-text-muted);
}

.file-filter {
  flex: 0 1 18rem;
}

.file-list-surface {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  background-color: var(--xy-surface-0);
}

.file-list-header,
.file-list-body-row {
  display: grid;
  grid-template-columns: 2.25rem minmax(0, 1fr) 8rem minmax(11rem, 14rem) 2.25rem;
  align-items: center;
}

.file-list-header {
  min-height: 2rem;
  padding: 0 var(--xy-space-xs);
  color: var(--xy-text-secondary);
  background-color: var(--xy-surface-2);
  border-bottom: 1px solid var(--xy-border);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.file-sort-button {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs);
  color: inherit;
  background: none;
  border: 0;
  border-radius: var(--xy-radius-sm);
  font: inherit;
  cursor: pointer;
}

.file-sort-button:hover {
  color: var(--xy-text-primary);
  background-color: var(--xy-surface-3);
}

.file-list-scroll {
  min-height: 0;
  flex: 1;
  padding-top: var(--xy-space-xs);
  overflow: auto;
}

.file-list-body-row {
  min-height: 2rem;
  padding: 0 var(--xy-space-xs);
  border-bottom: 1px solid var(--xy-border);
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  cursor: pointer;
  transition: background-color var(--xy-transition-fast);
}

.file-list-body-row:hover {
  background-color: var(--xy-surface-2);
}

.file-list-body-row--selected,
.file-list-body-row--selected:hover {
  background-color: var(--xy-surface-3);
  box-shadow: inset 0 0 0 1px var(--xy-border-active);
}

.file-list-select-cell,
.file-list-name-cell,
.file-list-size-cell,
.file-list-modified-cell,
.file-list-menu-cell {
  display: flex;
  min-width: 0;
  align-items: center;
}

.file-list-name-cell {
  gap: var(--xy-space-xs);
}

.file-list-size-cell,
.file-list-modified-cell {
  color: var(--xy-text-secondary);
}

.file-list-menu-cell {
  justify-content: flex-end;
}

.file-entry-name-block {
  min-width: 0;
}

.file-name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-entry-meta {
  display: none;
  overflow: hidden;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-row-menu {
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--xy-transition-fast);
}

.file-list-body-row:hover .file-row-menu,
.file-list-body-row:focus-within .file-row-menu,
:global(.touch) .file-row-menu {
  opacity: 1;
  pointer-events: auto;
}

.file-context-menu {
  min-width: 14rem;
}

.file-empty-state {
  display: flex;
  min-height: 16rem;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--xy-space-3xl) var(--xy-space-md);
  text-align: center;
}

.file-directory-state {
  display: flex;
  min-height: 16rem;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-md);
  text-align: center;
}

.file-directory-error {
  max-width: 65ch;
  overflow-wrap: anywhere;
}

@media (min-width: 600px) {
  .file-list-select-cell :deep(.q-checkbox__inner) {
    font-size: var(--xy-font-size-xl);
  }

  .file-row-menu {
    width: 2rem;
    min-width: 2rem;
    height: 2rem;
    min-height: 2rem;
  }
}

@media (max-width: 1023px) {
  .file-list-header,
  .file-list-body-row {
    grid-template-columns: 2.25rem minmax(0, 1fr) 7rem minmax(9rem, 11rem) 2.25rem;
  }
}

@media (max-width: 599px) {
  .file-toolbar-selection {
    flex: 1 1 100%;
    flex-wrap: wrap;
    padding-top: var(--xy-space-xs);
    padding-inline-start: 0;
    border-top: 1px solid var(--xy-border);
    border-inline-start: 0;
  }

  .file-selection-count {
    flex: 1 0 100%;
    padding-block: var(--xy-space-xs);
  }

  .file-navigation-buttons {
    order: 1;
  }

  .file-path-shell {
    order: 2;
    flex-basis: 100%;
  }

  .file-filter {
    order: 3;
    flex-basis: 100%;
  }

  .file-list-header,
  .file-list-body-row {
    grid-template-columns: 2.75rem minmax(0, 1fr) 2.75rem;
  }

  .file-list-size-cell,
  .file-list-modified-cell {
    display: none;
  }

  .file-entry-meta {
    display: block;
  }

  .file-list-body-row {
    min-height: 3.25rem;
  }
}
</style>
