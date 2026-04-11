import { ref } from 'vue'

function isCompactRuntimeViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }

  return window.matchMedia('(max-width: 900px)').matches
}

export function useGameFormRuntimePanels() {
  const runtimeSequenceExpanded = ref(false)
  const runtimePolicyExpanded = ref(false)

  function toggleRuntimePolicy(): void {
    const nextValue = !runtimePolicyExpanded.value
    runtimePolicyExpanded.value = nextValue
    if (nextValue && isCompactRuntimeViewport()) {
      runtimeSequenceExpanded.value = false
    }
  }

  function updateRuntimeSequenceExpanded(value: boolean): void {
    runtimeSequenceExpanded.value = value
    if (value && isCompactRuntimeViewport()) {
      runtimePolicyExpanded.value = false
    }
  }

  return {
    runtimeSequenceExpanded,
    runtimePolicyExpanded,
    toggleRuntimePolicy,
    updateRuntimeSequenceExpanded,
  }
}
