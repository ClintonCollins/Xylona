import { UpdateStep, StepStatus } from '@/proto/xylona_pb'

export interface StepState {
  step: UpdateStep
  status: StepStatus
  message?: string
}
