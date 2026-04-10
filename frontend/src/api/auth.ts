import { create } from '@bufbuild/protobuf'

import {
  CheckUserAuthenticatedRequestSchema,
  type CheckUserAuthenticatedResponse,
  LogoutRequestSchema,
} from '@/proto/xylona_pb'

import { getXylonaClient } from './connect-client'

type AuthClient = ReturnType<typeof getXylonaClient>

export async function checkUserAuthenticated(
  client: AuthClient = getXylonaClient(),
): Promise<CheckUserAuthenticatedResponse> {
  return client.checkUserAuthenticated(create(CheckUserAuthenticatedRequestSchema, {}))
}

export async function logout(client: AuthClient = getXylonaClient()): Promise<void> {
  await client.logout(create(LogoutRequestSchema, {}))
}
