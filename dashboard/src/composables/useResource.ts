import { onMounted, ref, shallowRef, type WatchSource, watch } from 'vue'

export function useResource<T>(loader: () => Promise<T>, sources: WatchSource[] = []) {
  const data = shallowRef<T>()
  const loading = ref(false)
  const error = shallowRef<Error>()
  let generation = 0

  async function reload(): Promise<void> {
    const current = ++generation
    loading.value = true
    error.value = undefined
    try {
      const result = await loader()
      if (current === generation) data.value = result
    } catch (caught) {
      if (current === generation) {
        error.value = caught instanceof Error ? caught : new Error('The request failed.')
      }
    } finally {
      if (current === generation) loading.value = false
    }
  }

  onMounted(() => void reload())
  if (sources.length > 0) watch(sources, () => void reload())

  return { data, loading, error, reload }
}
