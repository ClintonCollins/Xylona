import { PalworldMapActorKind, type PalworldMapActor } from '@/proto/xylona_pb'

export interface PalworldMapCategory {
  kind: PalworldMapActorKind
  label: string
  singular: string
  icon: string
  colorToken: string
  defaultVisible: boolean
  labeledMarker: boolean
}

export const palworldMapCategories: PalworldMapCategory[] = [
  {
    kind: PalworldMapActorKind.PLAYER,
    label: 'Players',
    singular: 'Player',
    icon: 'person',
    colorToken: '--xy-primary',
    defaultVisible: true,
    labeledMarker: true,
  },
  {
    kind: PalworldMapActorKind.BASE,
    label: 'Bases',
    singular: 'Base',
    icon: 'home_work',
    colorToken: '--xy-warning',
    defaultVisible: true,
    labeledMarker: true,
  },
  {
    kind: PalworldMapActorKind.BASE_WORKER,
    label: 'Base workers',
    singular: 'Base worker',
    icon: 'engineering',
    colorToken: '--xy-success',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.COMPANION_PAL,
    label: 'Companion Pals',
    singular: 'Companion Pal',
    icon: 'pets',
    colorToken: '--xy-accent',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.WILD_PAL,
    label: 'Wild Pals',
    singular: 'Wild Pal',
    icon: 'cruelty_free',
    colorToken: '--xy-purple',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.NPC,
    label: 'NPCs',
    singular: 'NPC',
    icon: 'record_voice_over',
    colorToken: '--xy-info',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.OTHER,
    label: 'Other actors',
    singular: 'Actor',
    icon: 'adjust',
    colorToken: '--xy-text-secondary',
    defaultVisible: true,
    labeledMarker: false,
  },
]

const categoriesByKind = new Map(palworldMapCategories.map((category) => [category.kind, category]))

export function palworldMapCategory(kind: PalworldMapActorKind): PalworldMapCategory {
  return (
    categoriesByKind.get(kind) ?? {
      kind,
      label: 'Other actors',
      singular: 'Actor',
      icon: 'adjust',
      colorToken: '--xy-text-secondary',
      defaultVisible: false,
      labeledMarker: false,
    }
  )
}

export function initialPalworldMapVisibility(): Record<number, boolean> {
  return Object.fromEntries(
    palworldMapCategories.map((category) => [category.kind, category.defaultVisible]),
  )
}

export function filterPalworldMapActors(
  actors: PalworldMapActor[],
  visibleKinds: Record<number, boolean>,
  search: string,
): PalworldMapActor[] {
  const normalizedSearch = search.trim().toLocaleLowerCase()
  return actors.filter((actor) => {
    if (!visibleKinds[actor.kind]) {
      return false
    }
    if (normalizedSearch === '') {
      return true
    }
    return [actor.name, actor.guildName, actor.trainerName, actor.className].some((value) =>
      value.toLocaleLowerCase().includes(normalizedSearch),
    )
  })
}

export function formatPalworldCoordinate(value: number): string {
  return Number.isFinite(value) ? value.toFixed(2) : '—'
}
