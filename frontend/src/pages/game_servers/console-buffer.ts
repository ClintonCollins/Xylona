export type ConsoleLine = {
  id: number
  html: string
}

const consoleChunkPattern = /[^\n]*\n|[^\n]+/g

export function splitConsoleChunk(html: string): string[] {
  return html.match(consoleChunkPattern) ?? []
}

export function trimConsoleLines(
  lines: ConsoleLine[],
  maxConsoleCharacters: number,
): {
  lines: ConsoleLine[]
  totalChars: number
  truncated: boolean
} {
  const trimmedLines = [...lines]
  let totalChars = trimmedLines.reduce((sum, line) => sum + line.html.length, 0)
  let truncated = false

  while (totalChars > maxConsoleCharacters && trimmedLines.length > 0) {
    const removed = trimmedLines.shift()
    if (!removed) {
      break
    }
    totalChars -= removed.html.length
    truncated = true
  }

  return {
    lines: trimmedLines,
    totalChars,
    truncated,
  }
}
