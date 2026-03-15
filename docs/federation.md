# Multi-Node Federation

## Overview

Xylona supports optional multi-node federation, allowing a single panel to display and manage game servers across multiple Xylona instances. Federation is fully optional — a standalone node continues to work exactly as before without any configuration.

## Architecture

Federation uses a **direct peer model** with **local-first authority**:

- Each node is authoritative only for its own local game servers
- Remote data is cached locally as read-only summaries
- The browser only communicates with the local node
- The local node aggregates local + remote data for the UI
- Remote nodes are never treated as database replicas

### Unified Node Model

Local and remote nodes are stored in a single `node` table. Local nodes have `is_local = true`; remote (peer) nodes have `is_local = false` with federation fields (`base_url`, `health_status`, `last_sync_at`, etc.). This consolidation means the UI presents a single "Nodes" page showing both local and remote nodes together.

### Key Design Principles

1. **Local-first**: Local functionality never depends on peer reachability
2. **Eventual consistency**: Remote state may be slightly stale; this is surfaced in the UI
3. **Separation of authority**: Local state is authoritative; remote state is cached
4. **Bounded blast radius**: A bad peer cannot degrade the whole node
5. **Explicit degradation**: The UI shows stale/offline indicators when remote data is unavailable

## Setup

### Prerequisites

Each node must have:
- A unique Node ID (auto-generated on first startup via `local_settings`)
- At least one Secret Key for authentication

### Step 1: Generate a Secret Key on the Remote Node

1. Navigate to **Secret Keys** in the remote node's panel
2. Click **Create Secret Key** and give it a name
3. Copy the generated key (it's only shown once)

### Step 2: Add the Peer on the Local Node

1. Navigate to **Nodes** in the local node's panel
2. Click **Add Peer Node**
3. Enter:
   - **Name** (optional): A display name for this peer
   - **Base URL**: The full URL of the remote node (e.g., `http://192.168.1.100:8080`)
   - **Secret Key**: The key generated in Step 1
4. Click **Add Peer**

The local node will:
- Perform a handshake with the remote node
- Verify the secret key is valid
- Fetch the remote node's identity and capabilities
- Prevent self-registration and duplicate peers
- Begin periodic synchronization

### Step 3: View Aggregated Servers

Navigate to **Game Servers** to see servers from both local and remote nodes in a unified list. Remote servers are tagged with a "remote" badge and show the source node name.

## How It Works

### Authentication

Federation uses the existing **Local Secret Keys** mechanism. When a local node connects to a peer, it sends the stored secret key. The remote node validates it against its Argon2id-hashed key store. Keys are never exposed to the browser.

### Sync Engine

The sync engine runs in the background with per-peer worker goroutines:

- **Health checks**: Every 30 seconds, pings each peer via the Handshake endpoint
- **Server sync**: Every 60 seconds, fetches server summaries from each peer
- **Status streaming**: Opens a persistent `StreamServerStatuses` connection per peer for real-time status updates (reconnects with backoff on failure)
- **Backoff**: On sync failure, uses exponential backoff with jitter (up to 5 minutes)
- **Recovery**: Automatically resumes when a peer comes back online
- **Isolation**: Each peer has its own worker; a slow peer doesn't block others

### Real-Time Updates

Remote server status and console output are streamed in real-time, matching the behavior of local servers:

#### Status Updates
1. The sync engine opens a `Federation.StreamServerStatuses` streaming connection to each peer
2. The peer polls its local game servers every 1 second and streams status changes
3. The sync engine updates the `remote_server_cache` status and broadcasts to all connected WebSocket clients
4. The frontend updates the status badge in real-time via the existing `gameServerStatus` event bus

#### Console Streaming
1. When a user views a remote server's console, the WebSocket handler detects it's a remote server
2. It opens a `Federation.StreamConsoleOutput` streaming connection to the peer node
3. Console output from the peer is bridged into the browser's WebSocket channel
4. Console input is proxied via `Federation.SendConsoleInput`
5. The initial console buffer is fetched via `Federation.ReadConsoleBuffer`

All streaming connections are cleaned up when the browser disconnects.

### Data Flow

```
Remote Node A ──[Federation API]──> Local Node ──[Xylona API/WebSocket]──> Browser
Remote Node B ──[Federation API]──┘
```

1. Sync engine polls each peer's `Federation.ListServerSummaries` endpoint
2. Results are upserted into the local `remote_server_cache` table
3. Status streaming updates cache in real-time between sync intervals
4. The `ListAggregatedGameServers` RPC combines local servers + cached remote servers
5. The frontend renders both in a single table with node origin indicators
6. Console output streams through federation → WebSocket → browser

### Remote Operations

All operations that work on local game servers are transparently proxied through the local node for remote servers. When an RPC receives a server ID that isn't found locally, it looks up the remote server cache and forwards the request to the owning peer node's Federation API.

#### Supported Remote Operations

| Operation | Federation RPC | Description |
|---|---|---|
| Start/Stop/Restart | `StartRemoteServer` / `StopRemoteServer` / `RestartRemoteServer` | Server lifecycle control |
| Update | `UpdateRemoteServer` | Trigger a game server update |
| Edit/Configure | `EditRemoteServer` | Edit game server settings |
| Remove | `RemoveRemoteServer` | Delete a game server |
| Console I/O | `StreamConsoleOutput` / `SendConsoleInput` / `ReadConsoleBuffer` | Real-time console |
| List Files | `ListRemoteDirectoryFiles` | Browse game server files |
| Edit File | `EditRemoteFile` | Edit file content |
| Delete Files | `DeleteRemoteFiles` | Delete files/directories |
| Rename File | `RenameRemoteFile` | Rename a file |
| Move Files | `MoveRemoteFiles` | Move files to a new path |
| Create File/Dir | `CreateRemoteFileOrDirectory` | Create a file or directory |
| Download from URL | `DownloadRemoteFileFromURL` | Download a file from a URL to the server |

The browser never connects directly to peer nodes — all remote operations go through the local panel backend.

## Failure Behavior

| Scenario | Behavior |
|---|---|
| Remote node offline | Local panel still works; remote servers shown as stale |
| Remote node slow | Per-peer timeout (15s); doesn't block other peers |
| Remote node returns bad data | Logged per-peer; doesn't poison the unified view |
| Peer removed | Sync stops; cached data deleted |
| Version mismatch | Handshake detects protocol version; shown in peer info |
| Duplicate peer | Blocked by node ID and URL uniqueness checks |
| Self-registration | Blocked by comparing remote node ID to local node ID |
| Node restart | Sync engine resumes; cached data survives in SQLite |

## Database Schema

### `node`
Stores all nodes — both the local node (`is_local = true`) and configured remote peers (`is_local = false`). Remote nodes have federation fields: `base_url`, `enabled`, `health_status`, `last_seen_at`, `last_sync_at`, `last_sync_status`, `version`, `protocol_version`, `capabilities`.

### `remote_server_cache`
Stores cached game server summaries from remote peers. Keyed by `(source_node_id, remote_server_id)` for upsert. References `node(id)` via `node_id`. Includes `is_stale` flag and timestamps.

### `peer_sync_state`
Tracks sync cursor, retry count, backoff timing, and last error per node. References `node(id)` via `node_id`.

## API Endpoints

### Federation Service (node-to-node)
- `Federation.Handshake` — Identity exchange and auth verification
- `Federation.ListServerSummaries` — Paginated server list for sync
- `Federation.GetServerDetail` — Single server detail
- `Federation.StartRemoteServer` / `StopRemoteServer` / `RestartRemoteServer` — Lifecycle actions
- `Federation.UpdateRemoteServer` — Trigger game server update
- `Federation.EditRemoteServer` — Edit game server configuration
- `Federation.RemoveRemoteServer` — Delete a game server
- `Federation.StreamConsoleOutput` — Server-streaming console output
- `Federation.SendConsoleInput` — Send a command to a game server's console
- `Federation.ReadConsoleBuffer` — Get the current console output buffer
- `Federation.StreamServerStatuses` — Server-streaming real-time status changes
- `Federation.ListRemoteDirectoryFiles` — List files in a game server directory
- `Federation.EditRemoteFile` — Edit a file on a game server
- `Federation.DeleteRemoteFiles` — Delete files on a game server
- `Federation.RenameRemoteFile` — Rename a file on a game server
- `Federation.MoveRemoteFiles` — Move files on a game server
- `Federation.CreateRemoteFileOrDirectory` — Create a file or directory
- `Federation.DownloadRemoteFileFromURL` — Download a file from a URL

### Xylona Service (UI-facing)
- `Xylona.ListNodes` / `GetNode` / `AddNode` / `EditNode` / `RemoveNode` — Local node CRUD
- `Xylona.ListPeerNodes` / `GetPeerNode` / `AddPeerNode` / `EditPeerNode` / `RemovePeerNode` — Remote node CRUD
- `Xylona.SyncPeerNode` — Trigger manual sync
- `Xylona.ListAggregatedGameServers` — Unified local + remote server list
- All game server RPCs (`EditGameServer`, `RemoveGameServer`, `UpdateGameServer`, `ListDirectoryFiles`, file operations, etc.) transparently proxy to remote nodes when the server ID isn't found locally

## Security

- Federation APIs require a valid secret key (Argon2id hashed)
- Credentials are stored in SQLite and never sent to the browser
- The browser only connects to the local node
- Peer URLs are validated before storage
- Secret keys are not logged

## Operational Notes

- Federation is disabled by default (no peers configured = single-node behavior)
- Adding the first peer activates the sync engine automatically
- The sync engine is resilient to restarts; state is persisted in SQLite
- Health status and last sync time are visible in the Nodes UI
- Manual sync can be triggered from the UI at any time
- The Nodes page shows both local and remote nodes in a unified view with type badges
