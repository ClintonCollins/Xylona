import {createPromiseClient} from "@connectrpc/connect";
import {createConnectTransport} from "@connectrpc/connect-web";
import {Xylona} from "src/proto/xylona_connect";
import {computed, onMounted, ref} from "vue";
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

export enum WebsocketRequestType {
    RequestUnknown,
    RequestGetGameServerConsole,
    RequestGetGameServerStatus,
    RequestRemoveGameServerConsole,
    RequestRemoveGameServerStatus,
}


export enum WebsocketMessageType {
    Unknown,
    GameServerConsole,
    GameServerStatus,
    Raw
}

export type GameServerStatusUpdate = {
    gameServerID: string;
    status: Status;
}

export type GameServerConsoleOutput = {
    gameServerID: string;
    output: string;
}

export type WebsocketRequest = {
    gameServerID?: string;
    type: WebsocketRequestType;
}

export type WebsocketMessage = {
    type: WebsocketMessageType;
    gameServerConsoleOutput?: GameServerConsoleOutput;
    gameServerStatusUpdate?: GameServerStatusUpdate;
    rawData?: any;
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
