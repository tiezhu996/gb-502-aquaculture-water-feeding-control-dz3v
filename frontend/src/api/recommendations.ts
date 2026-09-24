import { client, type ApiEnvelope } from './client'
import type { FeedingRecommendation, PageQuery, PageResult } from '@/types/models'

export const recommendationApi = {
  async list(params: PageQuery = {}) {
    const response = await client.get<ApiEnvelope<PageResult<FeedingRecommendation>>>('/recommendations', { params })
    return response.data.data
  },
  async generate(pondId: number, weather: string) {
    const response = await client.post<ApiEnvelope<FeedingRecommendation>>('/recommendations/generate', { pondId, weather })
    return response.data.data
  },
}
