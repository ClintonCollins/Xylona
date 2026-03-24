import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import { Status } from '@/proto/shared_pb'
import { StepStatus, UpdateProgressSchema, UpdateStep } from '@/proto/xylona_pb'
import {
  applyUpdateProgress,
  buildUpdateStepLabels,
  buildUpdateSteps,
  isUpdateProgressTerminal,
} from './update-progress'

describe('buildUpdateSteps', () => {
  it('omits stopping and restarting for offline servers', () => {
    expect(buildUpdateSteps(Status.OFFLINE)).toEqual([
      { step: UpdateStep.BACKING_UP, status: StepStatus.PENDING },
      { step: UpdateStep.DOWNLOADING, status: StepStatus.PENDING },
      { step: UpdateStep.INSTALLING, status: StepStatus.PENDING },
    ])
  })

  it('includes stopping and restarting for running servers', () => {
    expect(buildUpdateSteps(Status.ONLINE)).toEqual([
      { step: UpdateStep.STOPPING, status: StepStatus.PENDING },
      { step: UpdateStep.BACKING_UP, status: StepStatus.PENDING },
      { step: UpdateStep.DOWNLOADING, status: StepStatus.PENDING },
      { step: UpdateStep.INSTALLING, status: StepStatus.PENDING },
      { step: UpdateStep.RESTARTING, status: StepStatus.PENDING },
    ])
  })

  it('can apply steamcmd-specific labels', () => {
    expect(buildUpdateSteps(Status.OFFLINE, buildUpdateStepLabels({ usesSteamcmd: true }))).toEqual([
      { step: UpdateStep.BACKING_UP, status: StepStatus.PENDING },
      { step: UpdateStep.DOWNLOADING, status: StepStatus.PENDING, label: 'Preparing SteamCMD' },
      { step: UpdateStep.INSTALLING, status: StepStatus.PENDING, label: 'Running SteamCMD' },
    ])
  })
})

describe('applyUpdateProgress', () => {
  it('completes downloading when installing starts', () => {
    const steps = buildUpdateSteps(Status.OFFLINE)

    const downloading = applyUpdateProgress(
      steps,
      create(UpdateProgressSchema, {
        gameServerId: 'server-1',
        step: UpdateStep.DOWNLOADING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Downloading update',
      }),
    )

    const installing = applyUpdateProgress(
      downloading,
      create(UpdateProgressSchema, {
        gameServerId: 'server-1',
        step: UpdateStep.INSTALLING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Installing update',
      }),
    )

    expect(installing).toEqual([
      { step: UpdateStep.BACKING_UP, status: StepStatus.PENDING },
      {
        step: UpdateStep.DOWNLOADING,
        status: StepStatus.COMPLETED,
        message: 'Downloading update',
      },
      {
        step: UpdateStep.INSTALLING,
        status: StepStatus.IN_PROGRESS,
        message: 'Installing update',
      },
    ])
  })
})

describe('isUpdateProgressTerminal', () => {
  it('treats installing complete as terminal when the update finished without a restart', () => {
    const steps = buildUpdateSteps(Status.OFFLINE)
    const progress = create(UpdateProgressSchema, {
      gameServerId: 'server-1',
      step: UpdateStep.INSTALLING,
      stepStatus: StepStatus.COMPLETED,
      message: 'Installed Paper 1.21.5 with paper-1.21.5-28.jar',
    })

    expect(isUpdateProgressTerminal(progress, steps)).toBe(true)
  })

  it('does not treat installing complete as terminal when a restart is still expected', () => {
    const steps = buildUpdateSteps(Status.ONLINE)
    const progress = create(UpdateProgressSchema, {
      gameServerId: 'server-1',
      step: UpdateStep.INSTALLING,
      stepStatus: StepStatus.COMPLETED,
      message: 'Update installed',
    })

    expect(isUpdateProgressTerminal(progress, steps)).toBe(false)
  })
})
