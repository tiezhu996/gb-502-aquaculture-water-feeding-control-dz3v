<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { CircleCheck, Clock, List, Plus, VideoPlay } from '@element-plus/icons-vue'
import { executionApi } from '@/api/executions'
import { planApi } from '@/api/plans'
import { pondApi } from '@/api/ponds'
import { recommendationApi } from '@/api/recommendations'
import MetricCard from '@/components/common/MetricCard.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PlanDrawer from '@/components/common/PlanDrawer.vue'
import RecommendationSnapshot from '@/components/common/RecommendationSnapshot.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAuth } from '@/hooks/useAuth'
import { useQueryParams } from '@/hooks/useQueryParams'
import type { ControlExecution, ExecutionInput, FeedingPlan, FeedingRecommendation, Pond } from '@/types/models'
import { errorMessage } from '@/utils/errors'
import { formatDateTime, formatNumber, toISO, toLocalInput } from '@/utils/format'

const { canOperate } = useAuth()
const { params } = useQueryParams({ status: '', pondId: '', page: 1 })
const executions = ref<ControlExecution[]>([])
const ponds = ref<Pond[]>([])
const plans = ref<FeedingPlan[]>([])
const recommendations = ref<FeedingRecommendation[]>([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const editorOpen = ref(false)
const completeOpen = ref(false)
const deleteOpen = ref(false)
const drawerOpen = ref(false)
const basisDrawerOpen = ref(false)
const target = ref<ControlExecution | null>(null)
const basisTarget = ref<ControlExecution | null>(null)
const selectedPlan = ref<FeedingPlan | null>(null)
const scheduledLocal = ref(toLocalInput(new Date(Date.now() + 3600000)))
const form = reactive<ExecutionInput>({ pondId: 0, feedingPlanId: 0, recommendationSnapshotId: 0, scheduledAt: '', plannedAmountKg: 0, weather: '' })
const completion = reactive({ actualAmountKg: 0, oxygenSnapshot: 6, feedback: '' })

const scheduledCount = computed(() => executions.value.filter((item) => item.status === 'scheduled').length)
const runningCount = computed(() => executions.value.filter((item) => item.status === 'running').length)
const completedAmount = computed(() => executions.value.filter((item) => item.status === 'completed').reduce((sum, item) => sum + item.actualAmountKg, 0))
const availablePlans = computed(() => plans.value.filter((plan) => plan.status === 'approved' && (!form.pondId || plan.pondId === form.pondId)))
// 安排执行只能选用当前养殖池下仍有效的建议快照。
const availableRecommendations = computed(() => recommendations.value.filter((rec) => rec.valid && rec.pondId === form.pondId && (!form.feedingPlanId || rec.feedingPlanId === form.feedingPlanId)))
const selectedRecommendation = computed(() => recommendations.value.find((rec) => rec.id === form.recommendationSnapshotId) || null)

async function loadValidRecommendations(pondId = 0) {
  if (!pondId) {
    recommendations.value = []
    return
  }
  try {
    const result = await recommendationApi.list({ pondId, page: 1, pageSize: 100, validOnly: true })
    recommendations.value = result.items
  } catch (error) {
    ElMessage.error(errorMessage(error))
  }
}

async function load() {
  loading.value = true
  try {
    const [result, pondResult, planResult] = await Promise.all([
      executionApi.list({ page: Number(params.page), pageSize: 20, status: String(params.status), pondId: Number(params.pondId) || undefined }),
      pondApi.list({ page: 1, pageSize: 100 }),
      planApi.list({ page: 1, pageSize: 100 }),
    ])
    executions.value = result.items
    total.value = result.total
    ponds.value = pondResult.items
    plans.value = planResult.items
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  const firstPlan = plans.value.find((item) => item.status === 'approved')
  const pondId = firstPlan?.pondId || 0
  Object.assign(form, {
    pondId, feedingPlanId: firstPlan?.id || 0, recommendationSnapshotId: 0, plannedAmountKg: 0, weather: '晴朗，微风',
  })
  scheduledLocal.value = toLocalInput(new Date(Date.now() + 3600000))
  void loadValidRecommendations(pondId)
  editorOpen.value = true
}

function onPondChange(pondId: number) {
  form.feedingPlanId = 0
  form.recommendationSnapshotId = 0
  form.plannedAmountKg = 0
  void loadValidRecommendations(pondId)
}

function onPlanChange(planId: number) {
  const plan = plans.value.find((item) => item.id === planId)
  form.recommendationSnapshotId = 0
  if (!plan) return
  // 默认匹配该计划下最新的一条有效建议。
  const match = recommendations.value.find((rec) => rec.valid && rec.feedingPlanId === planId)
  if (match) {
    form.recommendationSnapshotId = match.id
    form.plannedAmountKg = Number(match.amountPerFeedingKg.toFixed(2))
    form.weather = match.weatherWindow || form.weather
  } else {
    form.plannedAmountKg = Number((plan.dailyAmountKg / plan.frequencyPerDay).toFixed(2))
  }
}

function onRecommendationChange(recId: number) {
  const rec = recommendations.value.find((item) => item.id === recId)
  if (!rec) return
  form.plannedAmountKg = Number(rec.amountPerFeedingKg.toFixed(2))
  form.weather = rec.weatherWindow || form.weather
}

async function create() {
  if (!form.pondId || !form.feedingPlanId || !form.recommendationSnapshotId || form.plannedAmountKg <= 0) {
    ElMessage.warning('请选择养殖池、已批准计划和一条有效投喂建议')
    return
  }
  saving.value = true
  try {
    await executionApi.create({ ...form, scheduledAt: toISO(scheduledLocal.value) })
    ElMessage.success('已依据有效建议安排投喂')
    editorOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    saving.value = false
  }
}

async function start(execution: ControlExecution) {
  saving.value = true
  try {
    await executionApi.update(execution.id, {
      scheduledAt: execution.scheduledAt, plannedAmountKg: execution.plannedAmountKg, weather: execution.weather, status: 'running',
    })
    ElMessage.success('执行已开始')
    await load()
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    saving.value = false
  }
}

function openComplete(execution: ControlExecution) {
  target.value = execution
  Object.assign(completion, { actualAmountKg: execution.plannedAmountKg, oxygenSnapshot: execution.oxygenSnapshot || 6, feedback: '' })
  completeOpen.value = true
}

async function complete() {
  if (!target.value || completion.actualAmountKg <= 0 || completion.feedback.trim().length < 2) {
    ElMessage.warning('请完整填写实际数量、现场溶解氧和反馈')
    return
  }
  saving.value = true
  try {
    await executionApi.complete(target.value.id, { ...completion })
    ElMessage.success('执行反馈已提交，计划状态已同步')
    completeOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!target.value) return
  saving.value = true
  try {
    await executionApi.remove(target.value.id)
    ElMessage.success('待执行安排已删除')
    deleteOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    saving.value = false
  }
}

function showPlan(execution: ControlExecution) {
  selectedPlan.value = execution.feedingPlan || plans.value.find((item) => item.id === execution.feedingPlanId) || null
  drawerOpen.value = true
}

function showBasis(execution: ControlExecution) {
  basisTarget.value = execution
  basisDrawerOpen.value = true
}

let timer: number | undefined
watch(params, () => { window.clearTimeout(timer); timer = window.setTimeout(load, 200) }, { deep: true })
onMounted(load)
</script>

<template>
  <div class="page-stack">
    <section class="metrics-grid">
      <MetricCard label="执行记录" :value="total" :icon="List" />
      <MetricCard label="待执行" :value="scheduledCount" :icon="Clock" tone="amber" />
      <MetricCard label="执行中" :value="runningCount" :icon="VideoPlay" tone="blue" />
      <MetricCard label="页内已投喂" :value="`${formatNumber(completedAmount)} kg`" :icon="CircleCheck" tone="green" />
    </section>
    <section class="workspace-panel">
      <div class="panel-toolbar">
        <div class="filters">
          <el-select v-model="params.pondId" placeholder="全部养殖池" clearable><el-option v-for="pond in ponds" :key="pond.id" :label="pond.name" :value="String(pond.id)" /></el-select>
          <el-select v-model="params.status" placeholder="全部状态" clearable><el-option label="待执行" value="scheduled" /><el-option label="执行中" value="running" /><el-option label="已完成" value="completed" /><el-option label="已取消" value="cancelled" /></el-select>
        </div>
        <el-button v-if="canOperate()" type="primary" :icon="Plus" @click="openCreate">安排执行</el-button>
      </div>
      <el-table v-loading="loading" :data="executions" stripe empty-text="暂无执行记录">
        <el-table-column label="养殖池 / 计划" min-width="230"><template #default="{ row }"><div class="primary-cell"><strong>{{ row.pond?.name }}</strong><button class="inline-link" @click="showPlan(row)">{{ row.feedingPlan?.name }} · v{{ row.feedingPlan?.version }}</button></div></template></el-table-column>
        <el-table-column label="安排时间" min-width="165"><template #default="{ row }">{{ formatDateTime(row.scheduledAt) }}</template></el-table-column>
        <el-table-column label="建议依据" min-width="150"><template #default="{ row }"><button v-if="row.recommendationNo" class="table-link" @click="showBasis(row)"><strong>{{ row.recommendationNo }}</strong><small>查看编号与依据</small></button><span v-else class="valid-dash">历史记录</span></template></el-table-column>
        <el-table-column label="计划 / 实际" min-width="130"><template #default="{ row }">{{ row.plannedAmountKg }} / {{ row.actualAmountKg || '—' }} kg</template></el-table-column>
        <el-table-column label="天气" prop="weather" min-width="130" show-overflow-tooltip />
        <el-table-column label="操作人" prop="operator" width="110" />
        <el-table-column label="状态" width="110"><template #default="{ row }"><StatusBadge :status="row.status" /></template></el-table-column>
        <el-table-column v-if="canOperate()" label="操作" width="190" fixed="right"><template #default="{ row }">
          <el-button v-if="row.status === 'scheduled'" link type="primary" :loading="saving" @click="start(row)">开始</el-button>
          <el-button v-if="row.status === 'scheduled' || row.status === 'running'" link type="success" @click="openComplete(row)">提交反馈</el-button>
          <el-button v-if="row.status === 'scheduled'" link type="danger" @click="target = row; deleteOpen = true">删除</el-button>
        </template></el-table-column>
      </el-table>
      <div class="pagination"><el-pagination v-model:current-page="params.page" layout="total, prev, pager, next" :total="total" :page-size="20" /></div>
    </section>
    <el-dialog v-model="editorOpen" title="安排投喂执行" width="640px">
      <el-alert title="必须从当前养殖池的有效投喂建议中选择一条作为依据；出现新水质、计划撤销或审批变化后，旧建议将不可选用。" type="info" :closable="false" show-icon />
      <el-form label-position="top" class="form-grid form-with-alert">
        <el-form-item label="养殖池"><el-select v-model="form.pondId" @change="onPondChange"><el-option v-for="pond in ponds.filter((item) => item.status === 'active')" :key="pond.id" :label="pond.name" :value="pond.id" /></el-select></el-form-item>
        <el-form-item label="已批准计划"><el-select v-model="form.feedingPlanId" @change="onPlanChange"><el-option v-for="plan in availablePlans" :key="plan.id" :label="`${plan.name} · v${plan.version}`" :value="plan.id" /></el-select></el-form-item>
        <el-form-item label="有效投喂建议" class="form-span">
          <el-select v-model="form.recommendationSnapshotId" :placeholder="form.pondId ? '选择一条有效建议' : '请先选择养殖池'" @change="onRecommendationChange">
            <el-option v-for="rec in availableRecommendations" :key="rec.id" :label="`${rec.snapshotNo} · ${rec.action === 'hold' ? '暂停' : rec.action === 'reduce' ? '减量' : '正常'} · ${formatNumber(rec.dailyAmountKg, 2)}kg/日`" :value="rec.id" :disabled="rec.action === 'hold'" />
          </el-select>
          <div v-if="form.pondId && availableRecommendations.length === 0" class="rec-hint">该池暂无有效建议（暂停类建议不可安排执行），请到投喂计划页重新生成。</div>
        </el-form-item>
        <el-form-item label="执行时间"><el-date-picker v-model="scheduledLocal" type="datetime" value-format="YYYY-MM-DDTHH:mm" /></el-form-item>
        <el-form-item label="计划数量（kg）"><el-input-number v-model="form.plannedAmountKg" :min="0.1" :step="1" /></el-form-item>
        <el-form-item label="天气窗口" class="form-span"><el-input v-model="form.weather" placeholder="例：晴朗，微风" /></el-form-item>
      </el-form>
      <div v-if="selectedRecommendation" class="basis-preview"><RecommendationSnapshot :recommendation="selectedRecommendation" compact /></div>
      <template #footer><el-button @click="editorOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="create">确认安排</el-button></template>
    </el-dialog>
    <el-dialog v-model="completeOpen" title="提交执行反馈" width="580px">
      <el-form label-position="top" class="form-grid">
        <el-form-item label="实际投喂量（kg）"><el-input-number v-model="completion.actualAmountKg" :min="0.1" :step="0.5" /></el-form-item>
        <el-form-item label="现场溶解氧（mg/L）"><el-input-number v-model="completion.oxygenSnapshot" :min="0" :max="30" :step="0.1" /></el-form-item>
        <el-form-item label="执行反馈" class="form-span"><el-input v-model="completion.feedback" type="textarea" :rows="4" placeholder="记录摄食、设备与异常情况" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="completeOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="complete">完成并留痕</el-button></template>
    </el-dialog>
    <ConfirmDialog v-model="deleteOpen" title="删除执行安排" message="只能删除尚未开始的执行安排，确认继续？" danger :loading="saving" @confirm="remove" />
    <PlanDrawer v-model="drawerOpen" :plan="selectedPlan" />
    <el-drawer v-model="basisDrawerOpen" title="执行依据" size="460px">
      <div v-if="basisTarget" class="basis-drawer-body">
        <div v-if="basisTarget.recommendation" class="basis-snapshot-wrap">
          <RecommendationSnapshot :recommendation="basisTarget.recommendation" />
        </div>
        <el-descriptions :column="1" border size="small" class="basis-meta">
          <el-descriptions-item label="建议编号">{{ basisTarget.recommendationNo || '—' }}</el-descriptions-item>
          <el-descriptions-item label="安排人">{{ basisTarget.operator }}</el-descriptions-item>
          <el-descriptions-item label="执行时天气">{{ basisTarget.weather || '—' }}</el-descriptions-item>
          <el-descriptions-item label="安排时溶氧">{{ formatNumber(basisTarget.oxygenSnapshot, 1) }} mg/L</el-descriptions-item>
        </el-descriptions>
        <section v-if="basisTarget.basis" class="basis-text">
          <h4>执行记录留痕</h4>
          <p>{{ basisTarget.basis }}</p>
        </section>
        <el-alert v-if="basisTarget.recommendation && !basisTarget.recommendation.valid" :title="basisTarget.recommendation.invalidReason || '该建议现已失效（执行安排时为有效）'" type="warning" :closable="false" show-icon />
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.basis-preview { margin-top: 12px; padding: 12px; background: var(--el-fill-color-light); border-radius: 8px; }
.rec-hint { margin-top: 6px; color: var(--el-color-warning); font-size: 12px; }
.basis-drawer-body { display: flex; flex-direction: column; gap: 16px; }
.basis-text p { white-space: pre-wrap; line-height: 1.7; color: var(--el-text-color-regular); }
</style>
