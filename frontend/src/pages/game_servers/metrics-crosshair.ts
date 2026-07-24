import { ref } from 'vue'

// Shared hovered timestamp for the metrics page. Every chart that imports it
// draws a crosshair at the same instant, so hovering any one chart correlates
// the same moment across all of them.
export const hoveredMetricTimestampMs = ref<number | null>(null)

export function nearestSampleTimestampMs(
  samples: readonly { timestampMs: number }[],
  targetMs: number,
): number | null {
  let nearest: number | null = null
  let nearestDistance = Number.POSITIVE_INFINITY
  for (const sample of samples) {
    const distance = Math.abs(sample.timestampMs - targetMs)
    if (distance < nearestDistance) {
      nearestDistance = distance
      nearest = sample.timestampMs
    }
  }
  return nearest
}
