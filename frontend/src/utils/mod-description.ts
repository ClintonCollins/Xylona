import createDOMPurify from 'dompurify'
import { marked } from 'marked'

// Mod descriptions come from third-party registries. Modrinth bodies are
// Markdown (with inline HTML), so they are converted first and every result,
// Markdown or not, goes through the same DOMPurify allowlist before v-html.
const sanitizeConfig = {
  ALLOWED_TAGS: [
    'a',
    'blockquote',
    'br',
    'code',
    'del',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'li',
    'ol',
    'p',
    'pre',
    'strong',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
  ],
  ALLOWED_ATTR: ['href', 'title'],
  // Images stay out: they would load third-party trackers and badge walls.
  FORBID_TAGS: ['iframe', 'img', 'object', 'script', 'style'],
}
const allowedTags = new Set(sanitizeConfig.ALLOWED_TAGS)

let domPurify: ReturnType<typeof createDOMPurify> | undefined

/** Converts a mod description (Markdown or HTML) into sanitized HTML. */
export function renderModDescription(source: string): string {
  if (source.trim() === '') {
    return ''
  }
  const html = marked.parse(source, { async: false, gfm: true })
  domPurify ??= createDOMPurify(window)
  const template = document.createElement('template')
  template.innerHTML = domPurify.sanitize(html, sanitizeConfig)
  stripUnsafeElements(template.content)
  stripUnsafeAttributes(template.content)
  return template.innerHTML
}

function stripUnsafeElements(node: ParentNode): void {
  for (const child of Array.from(node.childNodes)) {
    if (child.nodeType !== Node.ELEMENT_NODE) {
      continue
    }

    const element = child as HTMLElement
    if (!allowedTags.has(element.tagName.toLowerCase())) {
      const parent = element.parentNode
      if (!parent) {
        continue
      }
      while (element.firstChild) {
        parent.insertBefore(element.firstChild, element)
      }
      element.remove()
      continue
    }

    stripUnsafeElements(element)
  }
}

function stripUnsafeAttributes(node: ParentNode): void {
  for (const child of Array.from(node.childNodes)) {
    if (child.nodeType !== Node.ELEMENT_NODE) {
      continue
    }

    const element = child as HTMLElement
    for (const attr of Array.from(element.attributes)) {
      const attrName = attr.name.toLowerCase()
      if (attrName.startsWith('on')) {
        element.removeAttribute(attr.name)
        continue
      }
      if (element.tagName === 'A' && attrName === 'href' && !isSafeHref(attr.value)) {
        element.removeAttribute(attr.name)
      }
    }

    if (element.tagName === 'A') {
      // Badge links lose their image above and would be unnamed tab stops.
      if (element.textContent?.trim() === '') {
        element.remove()
        continue
      }
      // External links must not navigate the Xylona tab away.
      if (/^https?:\/\//i.test(element.getAttribute('href')?.trim() ?? '')) {
        element.setAttribute('target', '_blank')
        element.setAttribute('rel', 'noopener noreferrer nofollow')
      }
    }

    stripUnsafeAttributes(element)
  }
}

// Relative and fragment links would resolve against Xylona, not the registry.
function isSafeHref(href: string): boolean {
  const trimmedHref = href.trim().toLowerCase()
  return ['http://', 'https://', 'mailto:', 'tel:'].some((prefix) => trimmedHref.startsWith(prefix))
}
