<template>
  <div>
    <el-page-header @back="$router.push(`/sites/${$route.params.id}`)">
      <template #content><span>数据中心</span></template>
    </el-page-header>

    <el-tabs v-model="activeTab" style="margin-top:16px">
      <el-tab-pane label="Google Search Console" name="gsc">
        <div style="margin-bottom:12px">
          <el-button type="primary" size="small" @click="syncGSC" :loading="syncing.gsc">同步GSC数据</el-button>
        </div>
        <el-table :data="gscData" stripe>
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="clicks" label="点击" width="100" />
          <el-table-column prop="impressions" label="展示" width="100" />
          <el-table-column prop="ctr" label="CTR" width="100">
            <template #default="{row}">{{ (row.ctr*100).toFixed(2) }}%</template>
          </el-table-column>
          <el-table-column prop="position" label="平均排名" width="100">
            <template #default="{row}">{{ row.position?.toFixed(1) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="Google Analytics" name="ga">
        <div style="margin-bottom:12px">
          <el-button type="primary" size="small" @click="syncGA" :loading="syncing.ga">同步GA数据</el-button>
        </div>
        <el-table :data="gaData" stripe>
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="sessions" label="会话" />
          <el-table-column prop="users" label="用户" />
          <el-table-column prop="pageviews" label="页面浏览" />
          <el-table-column prop="bounce_rate" label="跳出率">
            <template #default="{row}">{{ (row.bounce_rate*100).toFixed(1) }}%</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="Yandex Webmaster" name="yandex">
        <div style="margin-bottom:12px">
          <el-button type="primary" size="small" @click="syncYandex" :loading="syncing.yandex">同步Yandex数据</el-button>
        </div>
        <el-table :data="yandexData" stripe>
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="clicks" label="点击" />
          <el-table-column prop="impressions" label="展示" />
          <el-table-column prop="index_count" label="收录数" />
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const route = useRoute()
const activeTab = ref('gsc')
const gscData = ref([])
const gaData = ref([])
const yandexData = ref([])
const syncing = ref({ gsc: false, ga: false, yandex: false })

async function loadData() {
  const id = route.params.id
  try {
    const [g, a, y] = await Promise.all([
      apiClient.getGSCData(id, 30).catch(() => ({ data: [] })),
      apiClient.getGAData(id).catch(() => ({ data: [] })),
      apiClient.getYandexData(id).catch(() => ({ data: [] })),
    ])
    gscData.value = g.data
    gaData.value = a.data
    yandexData.value = y.data
  } catch {}
}

async function syncGSC() {
  syncing.value.gsc = true
  try { await apiClient.syncGSC(route.params.id); ElMessage.success('GSC同步成功'); loadData() } catch {} finally { syncing.value.gsc = false }
}
async function syncGA() {
  syncing.value.ga = true
  try { await apiClient.syncGA(route.params.id); ElMessage.success('GA同步成功'); loadData() } catch {} finally { syncing.value.ga = false }
}
async function syncYandex() {
  syncing.value.yandex = true
  try { await apiClient.syncYandex(route.params.id); ElMessage.success('Yandex同步成功'); loadData() } catch {} finally { syncing.value.yandex = false }
}

onMounted(loadData)
</script>
