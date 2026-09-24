import { client, type ApiEnvelope } from './client'
import type { PageQuery, PageResult, RecommendationSnapshot } from '@/types/models'

export const recommendationApi = {
  async list(params: PageQuery = {}) {
    const response = await client.get<ApiEnvelope<PageResult<RecommendationSnapshot>>>('/recommendations', { params })
    return response.data.data
  },
  async create(input: { pondId: number; weather: string }) {
    const response = await client.post<ApiEnvelope<RecommendationSnapshot>>('/recommendations', input)
    return response.data.data
  },
}
