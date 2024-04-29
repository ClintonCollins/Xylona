import {createPromiseClient} from "@connectrpc/connect";
import {createConnectTransport} from "@connectrpc/connect-web";
import {Xylona} from "src/proto/xylona_connect";
import {onMounted, ref} from "vue";
import {Status} from "src/proto/xylona_pb";

const XylonaAPIBaseURL: string = "https://localhost";

const formSubmitting = ref(false)
const port = ref(0)
const queryPort = ref(0)

export function GetXylonaClient() {
    const transport = createConnectTransport({
        baseUrl: XylonaAPIBaseURL,
        credentials: "include",
    });
    return createPromiseClient(Xylona, transport)
}

export function StringToColor(str: string): string {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        hash = str.charCodeAt(i) + ((hash << 5) - hash);
    }

    const hue = hash % 360;
    return 'hsl(' + hue + ', 100%, 50%)';
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
            return "Unknown"
        case Status.ONLINE:
            return "Online"
        case Status.OFFLINE:
            return "Offline"
        case Status.UPDATING:
            return "Updating"
        case Status.INSTALLING:
            return "Installing"
        default:
            return "Unknown"
    }
}

// The conversion function
export function bytesToSize(bytes: number): string {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + sizes[i];
}
