import { StepStatus, UpdateStep } from '@/proto/xylona_pb'

export interface StepState {
  step: UpdateStep | string
  label?: string
  status: StepStatus
  message?: string
}
