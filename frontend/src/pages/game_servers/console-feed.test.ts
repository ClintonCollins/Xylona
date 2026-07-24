import { describe, expect, it } from 'vitest'

import {
  buildPlayerEventHtml,
  classifyConsoleLine,
  consoleLineMatchesFilter,
  consoleLinePlainText,
  diffRoster,
  getConsoleFeedClassifier,
  getConsoleFeedFilterOptions,
  type ConsoleFeedFilter,
  type ConsoleFeedKind,
} from './console-feed'

describe('classifyConsoleLine', () => {
  const minecraft = getConsoleFeedClassifier('minecraft')

  const cases: {
    name: string
    html: string
    classifier: 'minecraft' | null
    want: ConsoleFeedKind
  }[] = [
    {
      name: 'minecraft chat line with escaped angle brackets',
      html: "<span class='text-grey-6'>[12:03:11]</span> [Server thread/INFO]: &lt;M4rmalade&gt; anyone selling emeralds?",
      classifier: 'minecraft',
      want: 'chat',
    },
    {
      name: 'minecraft not-secure chat line',
      html: '[12:03:11] [Server thread/INFO]: [Not Secure] &lt;checkers_wife&gt; hi',
      classifier: 'minecraft',
      want: 'chat',
    },
    {
      name: 'minecraft join line',
      html: '[12:03:11] [Server thread/INFO]: PhoenixRaider joined the game',
      classifier: 'minecraft',
      want: 'player',
    },
    {
      name: 'minecraft leave line',
      html: '[12:03:11] [Server thread/INFO]: PhoenixRaider left the game',
      classifier: 'minecraft',
      want: 'player',
    },
    {
      name: 'minecraft uuid auth line',
      html: '[12:03:10] [User Authenticator #3/INFO]: UUID of player PhoenixRaider is 0b7f…',
      classifier: 'minecraft',
      want: 'player',
    },
    {
      name: 'minecraft plain server line',
      html: '[12:03:12] [Server thread/INFO]: Saving chunks for level world',
      classifier: 'minecraft',
      want: 'server',
    },
    {
      name: 'minecraft advancement mentioning a name without chat prefix',
      html: '[12:03:12] [Server thread/INFO]: checkers_wife has made the advancement [Hot Stuff]',
      classifier: 'minecraft',
      want: 'server',
    },
    {
      name: 'no classifier means everything is server',
      html: '&lt;M4rmalade&gt; looks like chat but the game is unknown',
      classifier: null,
      want: 'server',
    },
  ]

  it.each(cases)('$name', ({ html, classifier, want }) => {
    expect(classifyConsoleLine(html, classifier === 'minecraft' ? minecraft : null)).toBe(want)
  })

  it('palworld classifier tags join and leave log lines as player', () => {
    const palworld = getConsoleFeedClassifier('palworld')
    expect(
      classifyConsoleLine('Riley joined the server. (User id: steam_1, Player id: 42)', palworld),
    ).toBe('player')
    expect(classifyConsoleLine('REST API started on port 8212', palworld)).toBe('server')
  })

  it('unknown games have no classifier', () => {
    expect(getConsoleFeedClassifier('valheim')).toBeNull()
  })
})

describe('consoleLinePlainText', () => {
  it('strips highlight spans and unescapes entities', () => {
    expect(
      consoleLinePlainText(
        "<span class='text-green-6'>[INFO]</span>: &lt;Rae&gt; it&#39;s &amp; time",
      ),
    ).toBe("[INFO]: <Rae> it's & time")
  })
})

describe('getConsoleFeedFilterOptions', () => {
  it('always offers all; adds server/chat with a chat classifier; players via events or classifier', () => {
    expect(
      getConsoleFeedFilterOptions({ classifier: null, playerEventsAvailable: false }).map(
        (option) => option.value,
      ),
    ).toEqual(['all'])
    expect(
      getConsoleFeedFilterOptions({
        classifier: getConsoleFeedClassifier('minecraft'),
        playerEventsAvailable: true,
      }).map((option) => option.value),
    ).toEqual(['all', 'server', 'chat', 'player'])
    expect(
      getConsoleFeedFilterOptions({ classifier: null, playerEventsAvailable: true }).map(
        (option) => option.value,
      ),
    ).toEqual(['all', 'player'])
    expect(
      getConsoleFeedFilterOptions({
        classifier: getConsoleFeedClassifier('palworld'),
        playerEventsAvailable: false,
      }).map((option) => option.value),
    ).toEqual(['all', 'player'])
  })
})

describe('consoleLineMatchesFilter', () => {
  const cases: {
    name: string
    kind: ConsoleFeedKind | undefined
    filter: ConsoleFeedFilter
    want: boolean
  }[] = [
    { name: 'all matches everything', kind: undefined, filter: 'all', want: true },
    { name: 'undefined kind counts as server', kind: undefined, filter: 'server', want: true },
    { name: 'chat matches chat', kind: 'chat', filter: 'chat', want: true },
    { name: 'chat does not match server', kind: 'chat', filter: 'server', want: false },
    { name: 'player matches player', kind: 'player', filter: 'player', want: true },
  ]

  it.each(cases)('$name', ({ kind, filter, want }) => {
    expect(consoleLineMatchesFilter(kind, filter)).toBe(want)
  })
})

describe('diffRoster', () => {
  const cases: {
    name: string
    previous: string[]
    next: string[]
    want: { joined: string[]; left: string[] }
  }[] = [
    { name: 'no change', previous: ['a', 'b'], next: ['a', 'b'], want: { joined: [], left: [] } },
    { name: 'join', previous: ['a'], next: ['a', 'b'], want: { joined: ['b'], left: [] } },
    { name: 'leave', previous: ['a', 'b'], next: ['b'], want: { joined: [], left: ['a'] } },
    {
      name: 'simultaneous join and leave',
      previous: ['a', 'b'],
      next: ['b', 'c'],
      want: { joined: ['c'], left: ['a'] },
    },
    { name: 'empty to populated', previous: [], next: ['a'], want: { joined: ['a'], left: [] } },
  ]

  it.each(cases)('$name', ({ previous, next, want }) => {
    expect(diffRoster(previous, next)).toEqual(want)
  })
})

describe('buildPlayerEventHtml', () => {
  it('escapes player names and includes capacity when known', () => {
    const html = buildPlayerEventHtml({
      type: 'join',
      name: '<img src=x>',
      playerCount: 5,
      playerCapacity: 20,
    })
    expect(html).toContain('&lt;img src=x&gt;')
    expect(html).not.toContain('<img')
    expect(html).toContain('5/20 online')
    expect(html).toContain('console-player-event--join')
    expect(html.endsWith('\n')).toBe(true)
  })

  it('omits capacity when unknown', () => {
    const html = buildPlayerEventHtml({
      type: 'leave',
      name: 'Rae',
      playerCount: 3,
      playerCapacity: 0,
    })
    expect(html).toContain('3 online')
    expect(html).not.toContain('3/0')
    expect(html).toContain('console-player-event--leave')
  })
})
