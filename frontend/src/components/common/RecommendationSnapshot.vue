<script setup lang="ts">
import { computed } from 'vue'
import RiskTag from './RiskTag.vue'
import type { FeedingRecommendation } from '@/types/models'
import { formatDateTime, formatNumber } from '@/utils/format'

const props = defineProps<{ recommendation: FeedingRecommendation | null; compact?: boolean }>()

const actionLabel = computed(() => {
  switch (props.recommendation?.action) {
    case 'hold': return '暂停投喂'
    case 'reduce': return '减量投喂'
    default: return '按计划投喂'
  }
})

const adjustmentLabel = computed(() => {
  const value = props.recommendation?.adjustmentPercent ?? 0
  if (value === 0) return '不调整'
  return `${value > 0 ? '+' : ''}${formatNumber(value, 1)}%`
})
</script>

<template>
  <div v-if="recommendation" class="rec-snapshot" :data-action="recommendation.action" :data-valid="recommendation.valid">
    <div class="rec-head">
      <div class="rec-no">
        <span class="rec-badge">{{ recommendation.snapshotNo }}</span>
        <el-tag v-if="recommendation.valid" type="success" size="small" effect="dark" round>有效</el-tag>
        <el-tag v-else type="danger" size="small" effect="dark" round>已失效</el-tag>
      </div>
      <div class="rec-action">
        <strong>{{ actionLabel }}</strong>
        <small>{{ formatNumber(recommendation.dailyAmountKg, 2) }} kg/日 · {{ formatNumber(recommendation.amountPerFeedingKg, 2) }} kg × {{ recommendation.frequencyPerDay }} 次</small>
      </div>
    </div>

    <el-alert
      v-if="!recommendation.valid"
      :title="recommendation.invalidReason || '该建议已失效，不能再用于安排执行'"
      type="error" :closable="false" show-icon class="rec-invalid"
    />

    <el-descriptions :column="compact ? 1 : 2" size="small" border class="rec-desc">
      <el-descriptions-item label="计划版本">v{{ recommendation.planVersion }}</el-descriptions-item>
      <el-descriptions-item label="调整比例">{{ adjustmentLabel }}</el-descriptions-item>
      <el-descriptions-item label="水质时间">{{ formatDateTime(recommendation.readingMeasuredAt) }}</el-descriptions-item>
      <el-descriptions-item label="水质风险"><RiskTag :level="recommendation.riskLevel" /></el-descriptions-item>
      <el-descriptions-item label="溶解氧">{{ formatNumber(recommendation.dissolvedOxygen, 1) }} mg/L</el-descriptions-item>
      <el-descriptions-item label="水温">{{ formatNumber(recommendation.temperature, 1) }} ℃</el-descriptions-item>
      <el-descriptions-item label="天气窗口">{{ recommendation.weatherWindow || '未填写' }}</el-descriptions-item>
      <el-descriptions-item label="生成人">{{ recommendation.generatedBy }}</el-descriptions-item>
    </el-descriptions>

    <ul v-if="!compact || recommendation.reasons.length" class="rec-reasons">
      <li v-for="reason in recommendation.reasons" :key="reason">{{ reason }}</li>
    </ul>

    <div v-if="recommendation.invalidatedAt" class="rec-invalid-at">失效时间：{{ formatDateTime(recommendation.invalidatedAt) }}</div>
  </div>
</template>

<style scoped>
.rec-snapshot {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.rec-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.rec-no { display: flex; align-items: center; gap: 8px; }
.rec-badge {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-weight: 700;
  letter-spacing: 0.4px;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  border-radius: 6px;
  padding: 4px 10px;
}
.rec-action { text-align: right; display: flex; flex-direction: column; gap: 2px; }
.rec-action small { color: var(--el-text-color-secondary); }
.rec-snapshot[data-action='hold'] .rec-action strong { color: var(--el-color-danger); }
.rec-snapshot[data-action='reduce'] .rec-action strong { color: var(--el-color-warning); }
.rec-snapshot[data-action='feed'] .rec-action strong { color: var(--el-color-success); }
.rec-reasons { margin: 0; padding-left: 18px; color: var(--el-text-color-regular); display: flex; flex-direction: column; gap: 4px; }
.rec-invalid-at { color: var(--el-text-color-secondary); font-size: 12px; }
</style>
