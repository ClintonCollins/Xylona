const appName = 'Xylona'

/** Joins the non-empty parts, most specific first, and ends with the app name. */
export function formatPageTitle(...parts: (string | undefined)[]): string {
  return [...parts.map((part) => part?.trim() ?? '').filter((part) => part !== ''), appName].join(
    ' · ',
  )
}

export function setPageTitle(...parts: (string | undefined)[]): void {
  document.title = formatPageTitle(...parts)
}
