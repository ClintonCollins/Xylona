import {createPromiseClient} from "@connectrpc/connect";
import {createConnectTransport} from "@connectrpc/connect-web";
import { Xylona } from "src/proto/xylona_connect";

const XylonaAPIBaseURL: string = "https://localhost";

export function GetXylonaClient() {
  const transport = createConnectTransport({
    baseUrl: XylonaAPIBaseURL,
    credentials: "include",
  });
  return createPromiseClient(Xylona, transport)
}
