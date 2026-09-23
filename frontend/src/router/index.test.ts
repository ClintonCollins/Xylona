import { defineComponent } from 'vue'
import type { Router } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'

import createAppRouter from './index'

vi.mock('./routes', () => {
  const page = defineComponent({ render: () => null })
  return {
    default: [
      { path: '/', component: page, meta: { title: 'Game servers' } },
      { path: '/console', component: page, meta: { title: 'Console' } },
    ],
  }
})

describe('router page titles', () => {
  it('titles the page it reached and leaves the title alone when navigation fails', async () => {
    const router = (createAppRouter as unknown as (params: object) => Router)({})

    await router.push('/console')
    expect(document.title).toBe('Console · Xylona')

    // The server layout refines the title, then the active tab is clicked again.
    document.title = 'E2E Test Server · Console · Xylona'
    const failure = await router.push('/console')

    expect(failure).toBeTruthy()
    expect(document.title).toBe('E2E Test Server · Console · Xylona')
  })
})
