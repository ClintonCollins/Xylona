import { describe, expect, it } from 'vitest'

import { legacyGameServerEditRedirect } from './game-server-route-helpers'

describe('game server routes', () => {
  it('redirects the legacy edit route to settings', () => {
    expect(legacyGameServerEditRedirect('server-1')).toEqual({
      path: '/game-servers/server-1/settings',
    })
  })
})
