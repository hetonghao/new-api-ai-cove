type StorageLike = Pick<Storage, 'getItem' | 'removeItem' | 'setItem'>

type LocationLike = {
  readonly pathname?: string
  reload: () => void
}

type DocumentLike = {
  readonly documentElement?: {
    getAttribute: (name: string) => string | null
  }
  querySelector?: (
    selector: string
  ) => { getAttribute: (name: string) => string | null } | null
  readonly scripts?: Iterable<{ readonly src?: string }>
}

export type ChunkLoadRecoveryRuntime = {
  readonly buildRevision?: string
  readonly document?: DocumentLike
  readonly location?: LocationLike
  readonly now?: () => number
  readonly storage?: StorageLike
}

const CHUNK_RELOAD_PREFIX = 'chunk-reload'
const FALLBACK_BUILD_ID = 'unknown'

function getDefaultRuntime(): ChunkLoadRecoveryRuntime {
  if (typeof window === 'undefined') return {}

  return {
    buildRevision: window.__APP_BUILD__?.rev,
    document: typeof document === 'undefined' ? undefined : document,
    location: window.location,
    now: () => Date.now(),
    storage: window.sessionStorage,
  }
}

function normalizeText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}

function getLastPathSegment(value: string | undefined): string | undefined {
  if (!value) return undefined
  const segments = value.split('/')
  for (let index = segments.length - 1; index >= 0; index -= 1) {
    const segment = normalizeText(segments[index])
    if (segment) return segment
  }
  return undefined
}

function getBuildIdFromDocument(
  documentLike: DocumentLike | undefined
): string | undefined {
  const htmlBuild = normalizeText(
    documentLike?.documentElement?.getAttribute('data-build-rev')
  )
  if (htmlBuild) return htmlBuild

  const metaBuild = normalizeText(
    documentLike
      ?.querySelector?.('meta[name="build-id"]')
      ?.getAttribute('content')
  )
  if (metaBuild) return metaBuild

  const scripts = documentLike?.scripts ? [...documentLike.scripts] : []
  const entryScript = scripts
    .map((script) => normalizeText(script.src))
    .filter((src): src is string => Boolean(src))
    .reverse()
    .find(
      (src) =>
        src.includes('/static/js/') && !src.includes('/static/js/async/')
    )

  if (!entryScript) return undefined

  try {
    const url = new URL(entryScript, 'https://ai-cove.invalid')
    return normalizeText(getLastPathSegment(url.pathname))
  } catch {
    const cleanPath = entryScript.split('?')[0]?.split('#')[0]
    return normalizeText(getLastPathSegment(cleanPath))
  }
}

function getBuildId(runtime: ChunkLoadRecoveryRuntime): string {
  return (
    normalizeText(runtime.buildRevision) ??
    getBuildIdFromDocument(runtime.document) ??
    FALLBACK_BUILD_ID
  )
}

function getPathname(runtime: ChunkLoadRecoveryRuntime): string {
  return normalizeText(runtime.location?.pathname) ?? '/'
}

export function isChunkLoadError(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const record = error as { message?: unknown; name?: unknown }
  if (record.name !== 'ChunkLoadError') return false
  if (typeof record.message !== 'string') return false

  return (
    record.message.includes('Loading chunk') &&
    record.message.includes('failed') &&
    record.message.includes('missing:') &&
    record.message.includes('/static/js/async/')
  )
}

export function getChunkReloadKey(
  runtime: ChunkLoadRecoveryRuntime = getDefaultRuntime()
): string {
  return `${CHUNK_RELOAD_PREFIX}:${getBuildId(runtime)}:${getPathname(runtime)}`
}

export function recoverFromChunkLoadError(
  error: unknown,
  runtime: ChunkLoadRecoveryRuntime = getDefaultRuntime()
): boolean {
  if (!isChunkLoadError(error)) return false
  if (!runtime.location || !runtime.storage) return false

  const key = getChunkReloadKey(runtime)

  try {
    if (runtime.storage.getItem(key)) return false
    runtime.storage.setItem(key, String(runtime.now?.() ?? Date.now()))
    runtime.location.reload()
    return true
  } catch {
    return false
  }
}

export function clearChunkLoadRecoveryMarker(
  runtime: ChunkLoadRecoveryRuntime = getDefaultRuntime()
): void {
  if (!runtime.storage) return

  try {
    runtime.storage.removeItem(getChunkReloadKey(runtime))
  } catch {
  }
}
