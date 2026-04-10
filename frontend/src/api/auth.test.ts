import { create } from '@bufbuild/protobuf'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CheckUserAuthenticatedRequestSchema,
  CheckUserAuthenticatedResponseSchema,
  LogoutRequestSchema,
} from '@/proto/xylona_pb'

import { checkUserAuthenticated, logout } from './auth'

describe('auth api', () => {
  const client = {
    checkUserAuthenticated: vi.fn(),
    logout: vi.fn(),
  }

  beforeEach(() => {
    client.checkUserAuthenticated.mockReset()
    client.logout.mockReset()
  })

  it('creates the checkUserAuthenticated request inside the API layer', async () => {
    const response = create(CheckUserAuthenticatedResponseSchema, { authenticated: true })
    client.checkUserAuthenticated.mockResolvedValue(response)

    await expect(checkUserAuthenticated(client as never)).resolves.toBe(response)
    expect(client.checkUserAuthenticated).toHaveBeenCalledWith(
      create(CheckUserAuthenticatedRequestSchema, {}),
    )
  })

  it('creates the logout request inside the API layer', async () => {
    client.logout.mockResolvedValue(undefined)

    await expect(logout(client as never)).resolves.toBeUndefined()
    expect(client.logout).toHaveBeenCalledWith(create(LogoutRequestSchema, {}))
  })
})
