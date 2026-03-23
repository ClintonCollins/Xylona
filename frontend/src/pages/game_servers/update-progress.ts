import type { StepState } from '@/components/game_servers/UpdateProgressPanel.types'
import { Status } from '@/proto/shared_pb'
import type { UpdateProgress } from '@/proto/xylona_pb'
import { StepStatus, UpdateStep } from '@/proto/xylona_pb'

const UPDATE_STEP_ORDER: Array<UpdateStep> = [
  UpdateStep.STOPPING,
  UpdateStep.BACKING_UP,
  UpdateStep.DOWNLOADING,
  UpdateStep.INSTALLING,
  UpdateStep.RESTARTING,
  UpdateStep.ROLLING_BACK,
]

function stepOrderIndex(step: StepState['step']): number {
  return UPDATE_STEP_ORDER.findIndex((orderedStep) => orderedStep === step)
}

function cloneStep(step: StepState): StepState {
  return { ...step }
}

export function buildUpdateSteps(status: Status): StepState[] {
  const wasRunning = status !== Status.OFFLINE && status !== Status.UNKNOWN
  const steps: StepState[] = []

  if (wasRunning) {
    steps.push({ step: UpdateStep.STOPPING, status: StepStatus.PENDING })
  }

  steps.push(
    { step: UpdateStep.BACKING_UP, status: StepStatus.PENDING },
    { step: UpdateStep.DOWNLOADING, status: StepStatus.PENDING },
    { step: UpdateStep.INSTALLING, status: StepStatus.PENDING },
  )

  if (wasRunning) {
    steps.push({ step: UpdateStep.RESTARTING, status: StepStatus.PENDING })
  }

  return steps
}

export function applyUpdateProgress(
  currentSteps: StepState[],
  progress: UpdateProgress,
): StepState[] {
  const nextSteps = currentSteps.map(cloneStep)
  const currentOrderIndex = stepOrderIndex(progress.step)

  for (const step of nextSteps) {
    if (
      step.status === StepStatus.IN_PROGRESS &&
      stepOrderIndex(step.step) !== -1 &&
      stepOrderIndex(step.step) < currentOrderIndex
    ) {
      step.status = StepStatus.COMPLETED
    }
  }

  const nextState: StepState = {
    step: progress.step,
    status: progress.stepStatus,
    message: progress.message || undefined,
  }

  const existingIndex = nextSteps.findIndex((step) => step.step === progress.step)
  if (existingIndex >= 0) {
    nextSteps[existingIndex] = nextState
    return nextSteps
  }

  const insertionIndex = nextSteps.findIndex(
    (step) => stepOrderIndex(step.step) > currentOrderIndex,
  )
  if (insertionIndex >= 0) {
    nextSteps.splice(insertionIndex, 0, nextState)
    return nextSteps
  }

  nextSteps.push(nextState)
  return nextSteps
}

export function isUpdateProgressTerminal(progress: UpdateProgress, steps: StepState[]): boolean {
  if (progress.step === UpdateStep.ROLLING_BACK && progress.stepStatus === StepStatus.COMPLETED) {
    return true
  }

  if (
    progress.stepStatus === StepStatus.FAILED &&
    progress.step !== UpdateStep.INSTALLING &&
    progress.step !== UpdateStep.RESTARTING
  ) {
    return true
  }

  if (progress.step === UpdateStep.RESTARTING && progress.stepStatus === StepStatus.COMPLETED) {
    return true
  }

  if (
    progress.step === UpdateStep.INSTALLING &&
    progress.stepStatus === StepStatus.COMPLETED &&
    !steps.some((step) => step.step === UpdateStep.RESTARTING)
  ) {
    return true
  }

  return false
}
