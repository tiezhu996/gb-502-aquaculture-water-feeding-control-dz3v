import type { ExecutionStatus, PlanStatus, PondStatus, RiskLevel, UserRole } from './enums'

export interface BaseModel {
  id: number
  createdAt: string
  updatedAt: string
}

export interface User {
  id: number
  username: string
  displayName: string
  role: UserRole
}

export interface Pond extends BaseModel {
  code: string
  name: string
  species: string
  areaSquareMeters: number
  capacityKg: number
  growthStage: string
  status: PondStatus
  manager: string
  notes: string
}

export interface WaterReading extends BaseModel {
  pondId: number
  pond?: Pond
  dissolvedOxygen: number
  temperature: number
  ph: number
  ammonia: number
  turbidity: number
  measuredAt: string
  source: 'sensor' | 'manual' | 'import'
  riskLevel: RiskLevel
  alertMessage: string
  confirmed: boolean
  confirmedBy: string
  confirmedAt?: string
  confirmationNote: string
}

export interface FeedingPlan extends BaseModel {
  pondId: number
  pond?: Pond
  name: string
  version: number
  dailyAmountKg: number
  frequencyPerDay: number
  feedType: string
  targetGrowthStage: string
  minOxygen: number
  startDate: string
  endDate: string
  status: PlanStatus
  rationale: string
  createdBy: string
  reviewedBy: string
  reviewedAt?: string
}

export interface FeedingRecommendation {
  id: number
  snapshotNo: string
  pondId: number
  pond?: Pond
  feedingPlanId: number
  feedingPlan?: FeedingPlan
  waterReadingId?: number
  planVersion: number
  growthStage: string
  readingMeasuredAt: string
  dissolvedOxygen: number
  temperature: number
  ph: number
  ammonia: number
  turbidity: number
  riskLevel: RiskLevel
  weatherWindow: string
  action: 'feed' | 'reduce' | 'hold'
  dailyAmountKg: number
  amountPerFeedingKg: number
  frequencyPerDay: number
  adjustmentPercent: number
  reasons: string[]
  generatedBy: string
  valid: boolean
  invalidReason: string
  invalidatedAt?: string
  createdAt: string
}

export interface ControlExecution extends BaseModel {
  pondId: number
  pond?: Pond
  feedingPlanId: number
  feedingPlan?: FeedingPlan
  recommendationId?: number
  recommendation?: FeedingRecommendation
  recommendationNo: string
  basis: string
  scheduledAt: string
  startedAt?: string
  completedAt?: string
  plannedAmountKg: number
  actualAmountKg: number
  status: ExecutionStatus
  operator: string
  weather: string
  oxygenSnapshot: number
  feedback: string
}

export interface AuditLog extends BaseModel {
  actor: string
  role: string
  action: string
  entityType: string
  entityId: number
  fromState: string
  toState: string
  reason: string
  requestId: string
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

export interface PageQuery {
  page?: number
  pageSize?: number
  search?: string
  status?: string
  pondId?: number
  planId?: number
  unconfirmed?: boolean
  [key: string]: string | number | boolean | undefined
}

export interface PondInput {
  code: string
  name: string
  species: string
  areaSquareMeters: number
  capacityKg: number
  growthStage: string
  status: PondStatus
  manager: string
  notes: string
}

export interface WaterReadingInput {
  pondId: number
  dissolvedOxygen: number
  temperature: number
  ph: number
  ammonia: number
  turbidity: number
  measuredAt: string
  source: 'sensor' | 'manual' | 'import'
}

export interface FeedingPlanInput {
  pondId: number
  name: string
  dailyAmountKg: number
  frequencyPerDay: number
  feedType: string
  targetGrowthStage: string
  minOxygen: number
  startDate: string
  endDate: string
  rationale: string
}

export interface ExecutionInput {
  pondId: number
  feedingPlanId: number
  recommendationSnapshotId: number
  scheduledAt: string
  plannedAmountKg: number
  weather: string
}
