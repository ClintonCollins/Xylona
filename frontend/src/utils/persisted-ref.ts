import { ref, watch, type Ref } from 'vue'

export function usePersistedRef<T>(key: string, initialValue: T): Ref<T> {
  let startingValue = initialValue
  try {
    const raw = localStorage.getItem(key)
    if (raw !== null) {
      startingValue = JSON.parse(raw) as T
    }
  } catch {
    startingValue = initialValue
  }

  const value = ref(startingValue) as Ref<T>
  watch(
    value,
    (nextValue) => {
      localStorage.setItem(key, JSON.stringify(nextValue))
    },
    { deep: true },
  )
  return value
}
