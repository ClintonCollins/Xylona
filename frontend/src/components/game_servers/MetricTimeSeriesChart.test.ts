import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'

import { hoveredMetricTimestampMs } from '@/pages/game_servers/metrics-crosshair'
import MetricTimeSeriesChart from './MetricTimeSeriesChart.vue'

vi.mock('vue-chartjs', () => ({ Line: defineComponent({ render: () => null }) }))

type Sample = { timestampMs: number; value: number | null }

// A reading, a gap, then a reading, or a gap last when the latest is missing.
function mountLane(latest: number | null) {
  const samples: Sample[] = [
    { timestampMs: 1_000, value: 5 },
    { timestampMs: 2_000, value: null },
    { timestampMs: 3_000, value: latest },
  ]
  return mount(MetricTimeSeriesChart<Sample>, {
    props: {
      title: 'Query latency',
      description: 'Round trip',
      emptyLabel: 'No samples',
      samples,
      series: [{ label: 'Latency', colorToken: '--xy-series-1', value: (sample) => sample.value }],
      summary: {
        latest,
        minimum: 5,
        average: 5,
        maximum: 5,
        coverageRatio: null,
        sampleCount: samples.length,
      },
      formatValue: (value) => (value === null ? 'Unknown' : `${value} ms`),
      variant: 'lane',
      laneCaption: 'gap = failed query',
    },
  })
}

async function hover(timestampMs: number | null): Promise<void> {
  hoveredMetricTimestampMs.value = timestampMs
  await nextTick()
}

describe('MetricTimeSeriesChart lane value', () => {
  afterEach(() => {
    hoveredMetricTimestampMs.value = null
  })

  it('only mutes a hovered gap, so the lane keeps its height', async () => {
    const value = mountLane(7).get('.metric-lane__value')

    for (const timestampMs of [null, 1_000, 2_000, 3_000]) {
      await hover(timestampMs)
      expect(value.classes('metric-lane__value--missing')).toBe(timestampMs === 2_000)
      expect(value.classes('metric-lane__value--stacked')).toBe(false)
    }
    await hover(2_000)
    expect(value.text()).toContain('Unknown')
  })

  it('stacks the caption while the latest reading is missing, hovered or not', async () => {
    const value = mountLane(null).get('.metric-lane__value')

    for (const timestampMs of [null, 1_000, 2_000]) {
      await hover(timestampMs)
      expect(value.classes('metric-lane__value--stacked')).toBe(true)
    }
    expect(value.classes('metric-lane__value--missing')).toBe(true)
    await hover(1_000)
    expect(value.classes('metric-lane__value--missing')).toBe(false)
  })
})
