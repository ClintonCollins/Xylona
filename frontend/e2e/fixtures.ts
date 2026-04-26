import * as fs from 'fs'
import * as path from 'path'

export const AUTH_DIR = path.join(import.meta.dirname, '.auth')
const TEST_USERS_FILE = path.join(AUTH_DIR, 'test-users.json')
const TEST_STATE_FILE = path.join(AUTH_DIR, 'test-state.json')

export interface TestUser {
  id: string
  username: string
  password: string
  superUser: boolean
}

export interface TestState {
  mode?: string
  backendUrl?: string
  dataDir?: string
  controllerDir?: string
  controllerHomeDir?: string
  controllerPid?: number
  nodeDir?: string
  nodeHomeDir?: string
  remoteNodePid?: number
  gameServerId?: string
  gameServerDir?: string
  gameId?: string
  gameName?: string
  targetNodeId?: string
  remoteNodeId?: string
  dummyServerPath?: string
}

export function loadTestUsers(): TestUser[] {
  if (!fs.existsSync(TEST_USERS_FILE)) return []
  return JSON.parse(fs.readFileSync(TEST_USERS_FILE, 'utf-8')) as TestUser[]
}

export function loadTestState(): TestState {
  if (!fs.existsSync(TEST_STATE_FILE)) return {}
  return JSON.parse(fs.readFileSync(TEST_STATE_FILE, 'utf-8')) as TestState
}

export function requireTestState(): Required<
  Pick<TestState, 'gameServerId' | 'gameId' | 'gameServerDir' | 'dummyServerPath'>
> &
  TestState {
  const state = loadTestState()
  if (!state.gameServerId || !state.gameId || !state.gameServerDir || !state.dummyServerPath) {
    throw new Error('E2E test state is missing required fixture data')
  }
  return state as Required<
    Pick<TestState, 'gameServerId' | 'gameId' | 'gameServerDir' | 'dummyServerPath'>
  > &
    TestState
}
