export interface ServerSoftwareOperationEvent {
  status: 'installing' | 'complete' | 'failed'
  softwareId: string
  softwareName: string
  versionLabel?: string
  error?: string
}
