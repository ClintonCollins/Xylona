import { describe, expect, it } from 'vitest'

import { parseConsole } from './console'

describe('parseConsole', () => {
  it.each([
    {
      name: 'highlights a SteamCMD launcher and executable path for every game',
      game: 'minecraft',
      input:
        'steamcmd.sh[1288]: Starting  /home/clinton/.local/share/Steam/steamcmd/linux32/steamcmd',
      expected:
        "<span class='text-cyan-5'>steamcmd.sh[1288]:</span> <span class='text-blue-4'>Starting</span>  <span class='text-purple-3'>/home/clinton/.local/share/Steam/steamcmd/linux32/steamcmd</span>",
    },
    {
      name: 'highlights SteamCMD log paths',
      game: 'default',
      input: "Redirecting stderr to '/home/clinton/.local/share/Steam/logs/stderr.txt'",
      expected:
        "<span class='text-cyan-5'>Redirecting stderr to</span> <span class='text-purple-3'>'/home/clinton/.local/share/Steam/logs/stderr.txt'</span>",
    },
    {
      name: 'highlights bootstrap progress markers and byte counts',
      game: 'default',
      input: '[  4%] Downloading update (4,326 of 29,585 KB)...',
      expected:
        "<span class='text-cyan-5'>[  4%]</span> <span class='text-blue-4'>Downloading update</span> (<span class='text-purple-3'>4,326</span> of <span class='text-purple-3'>29,585 KB</span>)...",
    },
    {
      name: 'highlights app update state and removes ANSI control sequences',
      game: 'default',
      input: '\u001b[0m Update state (0x61) downloading, progress: 2.07 (106863076 / 5158199037)',
      expected:
        " <span class='text-cyan-5'>Update state</span><span class='text-purple-3'> (0x61)</span> <span class='text-blue-4'>downloading</span>, progress: <span class='text-amber-4'>2.07</span> (<span class='text-purple-3'>106863076</span> / <span class='text-purple-3'>5158199037</span>)",
    },
    {
      name: 'highlights successful Steam service connections',
      game: 'default',
      input: 'Loading Steam API...\u001b[0mOK',
      expected:
        "<span class='text-cyan-5'>Loading Steam API</span>...<span class='text-green-5'>OK</span>",
    },
    {
      name: 'highlights slow Steam IPC warnings',
      game: 'default',
      input: 'IPC function call IClientAppManager::GetUpdateInfo took too long: 46 msec',
      expected:
        "<span class='text-yellow-5'>IPC function call </span><span class='text-purple-3'>IClientAppManager::GetUpdateInfo</span><span class='text-yellow-5'> took too long: 46 msec</span>",
    },
    {
      name: 'highlights a successful app installation',
      game: 'default',
      input: "Success! App '232250' fully installed.",
      expected:
        "<span class='text-green-5'>Success!</span> App '<span class='text-purple-3'>232250</span>' fully installed.",
    },
    {
      name: 'highlights SteamCMD prompt commands',
      game: 'default',
      input: 'Steam>app_update 232250 validate',
      expected:
        "<span class='text-cyan-5'>Steam&gt;</span><span class='text-amber-4'>app_update 232250 validate</span>",
    },
    {
      name: 'highlights Palworld Unreal timestamps, categories, and display verbosity',
      game: 'palworld',
      input: '[2024.01.20-00.25.38:073][  0]LogPal: Display: Listening on port 8211',
      expected:
        "<span class='text-grey-6'>[2024.01.20-00.25.38:073][  0]</span><span class='text-cyan-5'>LogPal</span>: <span class='text-green-6'>Display</span>: Listening on port 8211",
    },
    {
      name: 'highlights Palworld REST access logs in the native server format',
      game: 'palworld',
      input: '[2026-07-15 00:01:45] [LOG] REST accessed endpoint /v1/api/info OK',
      expected:
        "<span class='text-grey-6'>[2026-07-15 00:01:45]</span> <span class='text-cyan-5'>[LOG]</span> REST accessed endpoint <span class='text-purple-3'>/v1/api/info</span> <span class='text-green-5'>OK</span>",
    },
    {
      name: 'highlights Palworld player joins and identifiers',
      game: 'palworld',
      input:
        '[2026-07-15 00:21:43] [LOG] Clinton joined the server. (User id: steam_76561197991986111, Player id: CE66DB46000000000000000000000000)',
      expected:
        "<span class='text-grey-6'>[2026-07-15 00:21:43]</span> <span class='text-cyan-5'>[LOG]</span> <span class='text-green-5'>Clinton</span> <span class='text-green-5'>joined</span> the server. (User id: <span class='text-purple-3'>steam_76561197991986111</span>, Player id: <span class='text-purple-3'>CE66DB46000000000000000000000000</span>)",
    },
    {
      name: 'highlights Palworld player departures',
      game: 'palworld',
      input:
        '[2026-07-15 00:24:03] [LOG] Clinton left the server. (User id: steam_76561197991986111, Player id: CE66DB46000000000000000000000000)',
      expected:
        "<span class='text-grey-6'>[2026-07-15 00:24:03]</span> <span class='text-cyan-5'>[LOG]</span> <span class='text-yellow-5'>Clinton</span> <span class='text-yellow-5'>left</span> the server. (User id: <span class='text-purple-3'>steam_76561197991986111</span>, Player id: <span class='text-purple-3'>CE66DB46000000000000000000000000</span>)",
    },
    {
      name: 'highlights Palworld warnings without a timestamp',
      game: 'palworld',
      input: 'LogNet: Warning: Network connection timed out',
      expected:
        "<span class='text-cyan-5'>LogNet</span>: <span class='text-yellow-5'>Warning</span>: Network connection timed out",
    },
    {
      name: 'highlights Palworld errors',
      game: 'palworld',
      input: 'LogOnline: Error: Failed to initialize session',
      expected:
        "<span class='text-cyan-5'>LogOnline</span>: <span class='text-red-5'>Error</span>: Failed to initialize session",
    },
    {
      name: 'highlights Palworld Unreal categories without explicit verbosity',
      game: 'palworld',
      input: 'LogInit: Build: PalServer-Linux-Shipping',
      expected: "<span class='text-cyan-5'>LogInit</span>: Build: PalServer-Linux-Shipping",
    },
    {
      name: 'highlights Palworld Steam API failures',
      game: 'palworld',
      input: '[S_API FAIL] SteamAPI_Init() failed',
      expected:
        "<span class='text-red-5'>[S_API FAIL]</span> <span class='text-red-5'>SteamAPI_Init() failed</span>",
    },
    {
      name: 'highlights successful Palworld Steam API initialization',
      game: 'palworld',
      input: "[S_API] SteamAPI_Init(): Loaded local 'steamclient.so' OK.",
      expected:
        "<span class='text-cyan-5'>[S_API]</span> SteamAPI_Init(): Loaded local 'steamclient.so' <span class='text-green-5'>OK</span>.",
    },
    {
      name: 'highlights the Palworld Steam AppID',
      game: 'palworld',
      input: 'Setting breakpad minidump AppID = 2394010',
      expected:
        "<span class='text-cyan-5'>Setting breakpad minidump AppID</span> = <span class='text-purple-3'>2394010</span>",
    },
    {
      name: 'highlights Palworld startup loader warnings',
      game: 'palworld',
      input: 'dlopen failed trying to load:',
      expected: "<span class='text-yellow-5'>dlopen failed trying to load:</span>",
    },
    {
      name: 'highlights a successful Palworld Steam client load',
      game: 'palworld',
      input: "steamclient.so OK. (First tried local 'steamclient.so')",
      expected:
        "steamclient.so <span class='text-green-5'>OK</span>. (First tried local 'steamclient.so')",
    },
    {
      name: 'highlights Palworld runtime version output',
      game: 'palworld',
      input: 'Game version is v1.0.0.100427',
      expected:
        "<span class='text-cyan-5'>Game version is</span> <span class='text-purple-3'>v1.0.0.100427</span>",
    },
    {
      name: 'highlights the Palworld REST listener port',
      game: 'palworld',
      input: 'REST API started on port 8212',
      expected:
        "<span class='text-cyan-5'>REST API</span> <span class='text-green-5'>started</span> on port <span class='text-purple-3'>8212</span>",
    },
    {
      name: 'highlights the Palworld game listener port',
      game: 'palworld',
      input: 'Running Palworld dedicated server on :8211',
      expected:
        "<span class='text-cyan-5'>Running Palworld dedicated server on</span> <span class='text-purple-3'>:8211</span>",
    },
    {
      name: 'highlights Palworld shutdown requests',
      game: 'palworld',
      input: 'FUnixPlatformMisc::RequestExitWithStatus',
      expected: "<span class='text-yellow-5'>FUnixPlatformMisc::RequestExitWithStatus</span>",
    },
    {
      name: 'highlights abnormal Palworld exits and error codes',
      game: 'palworld',
      input: 'Exiting abnormally (error code: 130)',
      expected:
        "<span class='text-red-5'>Exiting abnormally</span> (error code: <span class='text-red-5'>130</span>)",
    },
    {
      name: 'highlights existing Palworld Steam client paths',
      game: 'palworld',
      input:
        'The file already exists: /home/clinton/xylona/clinton/boomer-palworld/Pal/Binaries/Linux/steamclient.so',
      expected:
        "<span class='text-cyan-5'>The file already exists:</span> <span class='text-purple-3'>/home/clinton/xylona/clinton/boomer-palworld/Pal/Binaries/Linux/steamclient.so</span>",
    },
    {
      name: 'keeps Palworld-specific highlighting scoped to Palworld',
      game: 'default',
      input: 'LogNet: Warning: Network connection timed out',
      expected: 'LogNet: Warning: Network connection timed out',
    },
    {
      name: 'keeps unrecognized output escaped',
      game: 'default',
      input: "<script>alert('&')</script>",
      expected: "&lt;script&gt;alert('&amp;')&lt;/script&gt;",
    },
  ])('$name', ({ game, input, expected }) => {
    expect(parseConsole(game, input)).toBe(expected)
  })
})
