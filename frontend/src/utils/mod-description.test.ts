import { describe, expect, it } from 'vitest'

import { renderModDescription } from './mod-description'

function render(source: string): HTMLElement {
  const container = document.createElement('div')
  container.innerHTML = renderModDescription(source)
  return container
}

describe('renderModDescription', () => {
  it('converts Markdown into readable HTML', () => {
    // happy-dom's DOMPurify unwraps the first top-level element, so the
    // assertions target the blocks after the opening paragraph.
    const container = render(
      [
        'A **fast** mod. See [the wiki](https://example.com/wiki).',
        '',
        '## Features',
        '',
        '| Key | Action |',
        '| --- | ------ |',
        '| V | Talk |',
      ].join('\n'),
    )

    expect(container.querySelector('h2')?.textContent).toBe('Features')
    expect(container.querySelector('strong')?.textContent).toBe('fast')
    expect(container.querySelector('a')?.getAttribute('href')).toBe('https://example.com/wiki')
    expect(container.querySelector('table td')?.textContent).toBe('V')
    expect(container.innerHTML).not.toContain('##')
    expect(container.innerHTML).not.toContain('| Key |')
  })

  it('drops images, scripts, event handlers and unsafe links', () => {
    const container = render(
      [
        '![badge](https://img.example.com/badge.svg)',
        '',
        '<script>alert("xss")</script>',
        '<img src="x" onerror="alert(1)" />',
        '<a href="javascript:alert(2)" onclick="alert(3)">bad link</a>',
        '',
        '[also bad](javascript:alert(4))',
      ].join('\n'),
    )
    const html = container.innerHTML

    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(html).not.toContain('badge.svg')
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('onclick')
    expect(html).not.toContain('javascript:')
  })

  it('renders plain summaries as text and empty input as nothing', () => {
    expect(render('Adds proximity voice chat.').textContent?.trim()).toBe(
      'Adds proximity voice chat.',
    )
    expect(renderModDescription('   ')).toBe('')
  })
})
