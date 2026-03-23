import { UpdateStep, StepStatus } from '@/proto/xylona_pb'

export interface StepState {
  step: UpdateStep | string
  label?: string
  status: StepStatus
  message?: string
}

export interface OperationContextFact {
  label: string
  value: string
}
