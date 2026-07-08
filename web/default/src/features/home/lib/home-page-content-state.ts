export type HomePageContentInitialState = {
  readonly content: string
  readonly isLoaded: boolean
}

export function getInitialHomePageContentState(
  cachedContent: string | null
): HomePageContentInitialState {
  return {
    content: cachedContent || '',
    isLoaded: true,
  }
}
