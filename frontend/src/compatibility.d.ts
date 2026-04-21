declare namespace chrome {
  namespace runtime {
    interface Port {
      name: string
      disconnect(): void
      postMessage(message: unknown): void
      onDisconnect: {
        addListener(callback: (port: Port) => void): void
        removeListener(callback: (port: Port) => void): void
      }
      onMessage: {
        addListener(callback: (message: unknown, port: Port) => void): void
        removeListener(callback: (message: unknown, port: Port) => void): void
      }
    }
  }
}

interface BluetoothLEScanFilter {
  name?: string
  namePrefix?: string
  services?: BluetoothServiceUUID[]
}

type BluetoothServiceUUID = string | number

interface BluetoothDevice {
  id: string
  name?: string
  gatt?: BluetoothRemoteGATTServer
}

interface BluetoothRemoteGATTServer {
  connected: boolean
  device: BluetoothDevice
  connect(): Promise<BluetoothRemoteGATTServer>
  disconnect(): void
}

declare module 'quasar/dist/types/globals'

// Quasar's optional app-vite peers are only needed for its config typings.
declare module 'electron-builder' {
  export interface Configuration {
    [key: string]: unknown
  }
}

declare module 'builder-util' {
  export type Arch = string
}

declare module '@electron/packager' {
  export interface Options {
    [key: string]: unknown
  }

  export type OfficialArch = string
  export type OfficialPlatform = string
}

declare module 'workbox-build' {
  export interface GenerateSWOptions {
    [key: string]: unknown
  }

  export interface InjectManifestOptions {
    [key: string]: unknown
  }
}

declare module 'ini' {
  export function parse(
    input: string,
  ): Record<string, string | number | boolean | Record<string, string>>
}
