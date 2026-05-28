type CacheAwareLogOther = {
  claude?: boolean
  cache_tokens?: number
  cache_creation_tokens?: number
  cache_creation_tokens_5m?: number
  cache_creation_tokens_1h?: number
}

export function getLogCacheWriteTokens(
  other: CacheAwareLogOther | null | undefined
): number {
  if (!other) return 0

  const splitCacheWriteTokens =
    (other.cache_creation_tokens_5m || 0) +
    (other.cache_creation_tokens_1h || 0)

  if (splitCacheWriteTokens > 0) {
    return splitCacheWriteTokens
  }

  return other.cache_creation_tokens || 0
}

export function getDisplayPromptTokens(
  promptTokens: number,
  other: CacheAwareLogOther | null | undefined
): number {
  if (other?.claude === true) {
    return promptTokens
  }

  return Math.max(
    0,
    promptTokens - (other?.cache_tokens || 0) - getLogCacheWriteTokens(other)
  )
}
