import { create } from '@bufbuild/protobuf'
import { ExportGameRequestSchema } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

export async function exportGameDefinitionJSON(gameID: string): Promise<string> {
  const response = await GetXylonaClient().exportGame(
    create(ExportGameRequestSchema, {
      gameId: gameID,
    }),
  )

  downloadJSONFile(response.fileName, response.gameDefinitionJson)
  return response.fileName
}

function downloadJSONFile(fileName: string, contents: string): void {
  const blob = new Blob([contents], { type: 'application/json;charset=utf-8' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')

  link.href = url
  link.download = fileName || 'game-definition.json'
  link.style.display = 'none'

  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  window.URL.revokeObjectURL(url)
}
