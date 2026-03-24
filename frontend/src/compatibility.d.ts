/// <reference types="chrome" />
/// <reference types="web-bluetooth" />

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
