import { createCallbackClient, createPromiseClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { Xylona } from 'src/proto/xylona_connect'
import { onMounted, ref } from 'vue'
import { Status } from 'src/proto/xylona_pb'
import { GameServerFilesCompressionType } from 'src/proto/gameserver_files_operations_pb'
import {
    tabArchive,
    tabFileFilled,
    tabFileSettings,
    tabFileTypeTxt,
    tabFileTypeZip,
    tabFileZip,
    tabFilterSearch,
    tabJson
} from 'quasar-extras-svg-icons/tabler-icons-v2'

const XylonaAPIBaseURL: string = 'https://localhost'

export function GetXylonaClient() {
    const transport = createConnectTransport({
        baseUrl: XylonaAPIBaseURL,
        credentials: 'include'
    })
    return createPromiseClient(Xylona, transport)
}

export function GetXylonaClientCallback() {
    const transport = createConnectTransport({
        baseUrl: XylonaAPIBaseURL,
        credentials: 'include'
    })
    return createCallbackClient(Xylona, transport)
}

export function StringToColor(str: string): string {
    let hash = 0
    for (let i = 0; i < str.length; i++) {
        hash = str.charCodeAt(i) + ((hash << 5) - hash)
    }

    const hue = hash % 360
    return 'hsl(' + hue + ', 100%, 50%)'
}

export function GetRelativeFilePath(referencePathForSeparator: string, ...filePaths: string[]): string {
    let pathSeparator = '/'
    if (referencePathForSeparator.indexOf('\\') !== -1) {
        pathSeparator = '\\'
    }
    if (filePaths.length < 1) {
        return ''
    }
    if (filePaths[0] === '') {
        filePaths.shift()
    }
    console.log(filePaths)
    return filePaths.join(pathSeparator)
}

export function WindowWidth() {
    const windowWidth = ref(window.innerWidth)

    function updateWindowWidth() {
        windowWidth.value = window.innerWidth
    }

    onMounted(() => {
        window.addEventListener('resize', () => updateWindowWidth())
        window.removeEventListener('resize', () => updateWindowWidth())
    })

    return windowWidth
}

export function StatusToString(status: Status): string {
    switch (status) {
        case Status.UNKNOWN:
            return 'Unknown'
        case Status.ONLINE:
            return 'Online'
        case Status.OFFLINE:
            return 'Offline'
        case Status.UPDATING:
            return 'Updating'
        case Status.INSTALLING:
            return 'Installing'
        default:
            return 'Unknown'
    }
}

export function ArchiveTypeToString(archiveType: GameServerFilesCompressionType): string {
    switch (archiveType) {
        case GameServerFilesCompressionType.ZIP:
            return 'ZIP (.zip)'
        case GameServerFilesCompressionType.GZIP:
            return 'Gzip (.gz)'
        case GameServerFilesCompressionType.BZIP2:
            return 'Bzip2 (.bz2)'
        case GameServerFilesCompressionType.ZST:
            return 'Zstandard (.zst)'
        case GameServerFilesCompressionType.XZ:
            return 'XZ (.xz)'
        default:
            return 'Unknown'
    }
}

export function ArchiveTypeToExtension(archiveType: GameServerFilesCompressionType): string {
    switch (archiveType) {
        case GameServerFilesCompressionType.ZIP:
            return '.zip'
        case GameServerFilesCompressionType.GZIP:
            return '.tar.gz'
        case GameServerFilesCompressionType.BZIP2:
            return '.tar.bz2'
        case GameServerFilesCompressionType.ZST:
            return '.tar.zst'
        case GameServerFilesCompressionType.XZ:
            return '.tar.xz'
        default:
            return '.unknown'
    }
}

// The conversion function
export function bytesToSize1(bytes: number): string {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
    if (bytes === 0) return '0 Bytes'
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + sizes[i]
}

export function bytesToSize(bytes: number): string {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
    if (bytes === 0) return '0 Byte'
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + sizes[i]
}

export function getIconFromFilenameExtension(fileName: string): string {
    const fileNameSplit = fileName.split('.')
    if (fileNameSplit.length <= 1) {
        return tabFileFilled
    }
    const extension = fileNameSplit[fileNameSplit.length - 1]
    switch (extension) {
        case 'json':
            return tabJson
        case 'txt':
            return tabFileTypeTxt
        case 'log':
            return tabFilterSearch
        case 'settings':
            return tabFileSettings
        case 'jar':
            return tabArchive
        case 'zip':
            return tabFileTypeZip
        case 'xz':
            return tabFileZip
        case 'gz':
            return tabFileZip
        case 'bz2':
            return tabFileZip
        case 'zst':
            return tabFileZip
        default:
            return tabFileFilled
    }
}

export function getColorFromFilenameExtension(fileName: string): string {
    const fileNameSplit = fileName.split('.')
    if (fileNameSplit.length <= 1) {
        return 'whitesmoke'
    }
    const extension = fileNameSplit[fileNameSplit.length - 1]
    switch (extension) {
        case 'json':
            return '#74c639'
        case 'txt':
            return '#94c2e6'
        case 'log':
            return '#818181'
        case 'settings':
            return 'orange'
        case 'jar':
            return '#f0db4f'
        case 'zip':
            return '#f0db4f'
        case 'xz':
            return '#3e9b00'
        case 'gz':
            return '#674753'
        case 'bz2':
            return '#757de7'
        case 'zst':
            return '#f07f4f'
        default:
            return 'whitesmoke'
    }
}
