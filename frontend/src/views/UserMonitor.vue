<template>
  <div class="user-monitor">
    <div class="hdr">
      <h1>📡 用户监控</h1>
      <p class="hdr-sub">
        查看每个员工绑定的站点、4 平台数据、协同任务和配置状态。管理员只读视图 — 这里看不到任何用户的密钥明文。
      </p>
    </div>

    <div class="monitor-layout">
      <!-- Left: user list -->
      <div class="user-list panel">
        <div class="user-list-hdr">
          <input class="search-input" v-model="search" placeholder="搜索姓名 / 邮箱..." />
          <div class="user-count">{{ filteredUsers.length }} / {{ users.length }} 人</div>
        </div>
        <div class="user-rows">
          <div
            v-for="u in filteredUsers"
            :key="u.id"
            :class="['user-row', selected?.id === u.id && 'selected']"
            @click="selectUser(u)"
          >
            <div class="u-name">{{ u.name }}
              <span v-if="u.role === 'admin'" class="role-admin">admin</span>
            </div>
            <div class="u-email">{{ u.email }}</div>
            <div class="u-meta">{{ u.department_name || '—' }}<span v-if="u.group_name"> · {{ u.group_name }}</span></div>
          </div>
          <div v-if="!filteredUsers.length" class="empty-cell">没有匹配的用户</div>
        </div>
      </div>

      <!-- Right: selected user details -->
      <div class="detail">
        <div v-if="!selected" class="panel empty-hint">
          <div class="empty-icon">👈</div>
          <div>从左侧选择一个用户查看详情</div>
        </div>

        <template v-else>
          <!-- Overview header -->
          <div class="panel overview-card">
            <div class="ov-head">
              <div>
                <div class="ov-name">{{ selected.name }}
                  <span v-if="selected.role === 'admin'" class="role-admin">admin</span>
                </div>
                <div class="ov-email">{{ selected.email }}</div>
                <div class="ov-org">
                  <span>{{ overview?.department || '—' }}</span>
                  <span v-if="overview?.group"> · {{ overview.group }}</span>
                  <span v-if="overview?.last_login"> · 最后登录 {{ (overview.last_login || '').slice(0,16) }}</span>
                </div>
              </div>
              <div class="ov-counts">
                <div class="count-box">
                  <div class="count-num">{{ overview?.sites?.total || 0 }}</div>
                  <div class="count-lbl">绑定站点</div>
                </div>
                <div class="count-box">
                  <div class="count-num">{{ overview?.coord?.total || 0 }}</div>
                  <div class="count-lbl">协同任务</div>
                </div>
                <div class="count-box">
                  <div class="count-num">{{ platformBoundCount }}</div>
                  <div class="count-lbl">平台已绑</div>
                </div>
              </div>
            </div>
          </div>

          <div class="panel tabs-panel">
            <el-tabs v-model="activeTab" @tab-change="onTabChange">
              <!-- ─── 绑定站点 ─── -->
              <el-tab-pane label="🌐 绑定站点" name="sites">
                <div v-if="loading.sites" class="loading-cell">加载中...</div>
                <table v-else-if="sites.length" class="table">
                  <thead>
                    <tr>
                      <th style="width:32%">站点名称</th>
                      <th>域名</th>
                      <th style="width:100px">状态</th>
                      <th style="width:120px">最近审计</th>
                      <th style="width:180px">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="s in sites" :key="s.id">
                      <td style="font-weight:600">{{ s.name }}</td>
                      <td style="color:var(--ink-soft);font-size:12px">{{ s.domain }}</td>
                      <td>
                        <span :class="['badge', auditBadge(s.last_audit_status)]">
                          {{ auditLabel(s.last_audit_status) }}
                        </span>
                      </td>
                      <td style="font-size:11px;color:var(--ink-soft)">
                        {{ s.last_audit_time ? String(s.last_audit_time).slice(0, 16) : '—' }}
                      </td>
                      <td>
                        <router-link :to="`/sites/${s.id}/data`" class="btn btn-xs">📊 4 平台数据</router-link>
                        <router-link :to="`/sites/${s.id}`" class="btn btn-xs" style="margin-left:4px">详情</router-link>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <div v-else class="empty-cell">该用户还没有绑定站点</div>
              </el-tab-pane>

              <!-- ─── 协同任务 ─── -->
              <el-tab-pane label="🤝 协同任务" name="coord">
                <div v-if="loading.coord" class="loading-cell">加载中...</div>
                <table v-else-if="coordTasks.length" class="table">
                  <thead>
                    <tr>
                      <th>任务</th>
                      <th style="width:120px">站点</th>
                      <th style="width:80px">角色</th>
                      <th style="width:100px">状态</th>
                      <th style="width:140px">创建时间</th>
                      <th style="width:80px">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="t in coordTasks" :key="t.id">
                      <td>
                        <div style="font-weight:600">{{ t.issue_title || '未命名任务' }}</div>
                        <div v-if="t.issue_severity" style="font-size:11px;color:var(--ink-hint)">
                          {{ t.issue_severity }} · {{ t.issue_module }}
                        </div>
                      </td>
                      <td style="font-size:12px;color:var(--ink-soft)">{{ t.site_name || t.site_id }}</td>
                      <td>
                        <span class="role-chip" :class="roleOf(t) === 'creator' ? 'chip-creator' : 'chip-assignee'">
                          {{ roleOf(t) === 'creator' ? '发起人' : '负责人' }}
                        </span>
                      </td>
                      <td>
                        <span :class="['badge', coordBadge(t.status)]">{{ coordLabel(t.status) }}</span>
                      </td>
                      <td style="font-size:11px;color:var(--ink-soft)">{{ String(t.created_at || '').slice(0, 16) }}</td>
                      <td>
                        <router-link :to="`/coordination/${t.id}`" class="btn btn-xs">查看</router-link>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <div v-else class="empty-cell">该用户没有相关的协同任务</div>
              </el-tab-pane>

              <!-- ─── 平台配置 ─── -->
              <el-tab-pane label="🔑 平台配置" name="platforms">
                <div v-if="!overview?.platforms" class="loading-cell">加载中...</div>
                <div v-else class="platform-grid">
                  <div v-for="p in platformRows" :key="p.key" class="platform-card">
                    <div class="platform-name">{{ p.label }}</div>
                    <div class="platform-status" :class="p.bound ? 'ok' : 'off'">
                      {{ p.statusText }}
                    </div>
                  </div>
                </div>
              </el-tab-pane>
            </el-tabs>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const users = ref([])
const search = ref('')
const selected = ref(null)
const overview = ref(null)
const sites = ref([])
const coordTasks = ref([])
const activeTab = ref('sites')
const loading = ref({ sites: false, coord: false })

const filteredUsers = computed(() => {
  const q = search.value.toLowerCase().trim()
  if (!q) return users.value
  return users.value.filter(u =>
    (u.name || '').toLowerCase().includes(q) ||
    (u.email || '').toLowerCase().includes(q),
  )
})

const platformBoundCount = computed(() => {
  const p = overview.value?.platforms
  if (!p) return 0
  let n = 0
  for (const k of ['gsc', 'ga', 'yandex', 'bing', 'telegram', 'frw', 'ai']) {
    if (p[k]?.bound) n++
  }
  return n
})

const platformRows = computed(() => {
  const p = overview.value?.platforms
  if (!p) return []
  return [
    {
      key: 'ai', label: '🤖 AI 接口',
      bound: p.ai?.bound,
      statusText: p.ai?.bound ? `已配置 (${p.ai.slots} 组)` : '未配置',
    },
    {
      key: 'gsc', label: '🟦 Google Search Console',
      bound: p.gsc?.bound,
      statusText: p.gsc?.bound
        ? `已授权${p.gsc.expires_at ? ' · 至 ' + String(p.gsc.expires_at).slice(0,16) : ''}`
        : (p.gsc?.has_client_id ? '仅填写应用凭据' : '未配置'),
    },
    {
      key: 'ga', label: '🟩 Google Analytics',
      bound: p.ga?.bound,
      statusText: p.ga?.bound
        ? `已授权${p.ga.expires_at ? ' · 至 ' + String(p.ga.expires_at).slice(0,16) : ''}`
        : (p.ga?.has_client_id ? '仅填写应用凭据' : '未配置'),
    },
    {
      key: 'yandex', label: '🟥 Yandex Webmaster',
      bound: p.yandex?.bound,
      statusText: p.yandex?.bound
        ? `已授权${p.yandex.expires_at ? ' · 至 ' + String(p.yandex.expires_at).slice(0,16) : ''}`
        : (p.yandex?.has_client_id ? '仅填写应用凭据' : '未配置'),
    },
    {
      key: 'bing', label: '🟦 Bing Webmaster',
      bound: p.bing?.bound,
      statusText: p.bing?.bound ? '已配置' : '未配置',
    },
    {
      key: 'telegram', label: '📱 Telegram',
      bound: p.telegram?.bound,
      statusText: p.telegram?.bound ? '已配置' : '未配置',
    },
    {
      key: 'frw', label: '🎨 FRW (图/视频)',
      bound: p.frw?.bound,
      statusText: p.frw?.bound ? '已配置' : '未配置',
    },
  ]
})

onMounted(async () => {
  try {
    const res = await api.listUsers()
    users.value = res.data || []
    if (users.value.length) selectUser(users.value[0])
  } catch (e) {
    ElMessage.error('加载用户列表失败')
  }
})

function selectUser(u) {
  selected.value = u
  activeTab.value = 'sites'
  overview.value = null
  sites.value = []
  coordTasks.value = []
  loadOverview()
  loadSites()
}

async function loadOverview() {
  if (!selected.value) return
  try {
    const res = await api.getUserOverview(selected.value.id)
    overview.value = res.data
  } catch (e) {
    ElMessage.error('加载用户概览失败')
  }
}

async function loadSites() {
  if (!selected.value) return
  loading.value.sites = true
  try {
    const res = await api.listSites({ user_id: selected.value.id })
    sites.value = res.data || []
  } catch (e) {
    ElMessage.error('加载站点失败')
  } finally {
    loading.value.sites = false
  }
}

async function loadCoord() {
  if (!selected.value) return
  loading.value.coord = true
  try {
    const res = await api.coordListTasks({ involved_user_id: selected.value.id, per_page: 200 })
    coordTasks.value = res.data?.items || []
  } catch (e) {
    ElMessage.error('加载协同任务失败')
  } finally {
    loading.value.coord = false
  }
}

function onTabChange() {
  if (activeTab.value === 'coord' && !coordTasks.value.length) {
    loadCoord()
  }
}

watch(() => selected.value?.id, () => {
  if (activeTab.value === 'coord') loadCoord()
})

function roleOf(t) {
  if (!selected.value) return ''
  return Number(t.created_by) === Number(selected.value.id) ? 'creator' : 'assignee'
}

function auditLabel(s) {
  return { done: '完成', error: '失败', running: '进行中' }[s] || '未审计'
}
function auditBadge(s) {
  return { done: 'badge-ok', error: 'badge-err', running: 'badge-warn' }[s] || 'badge-muted'
}
function coordLabel(s) {
  return {
    pending: '待分配', in_progress: '修复中', awaiting_answer: '等待回复',
    under_review: '待验收', done: '已完成', rejected: '已驳回',
  }[s] || s
}
function coordBadge(s) {
  return {
    pending: 'badge-muted', in_progress: 'badge-warn', awaiting_answer: 'badge-warn',
    under_review: 'badge-warn', done: 'badge-ok', rejected: 'badge-err',
  }[s] || 'badge-muted'
}
</script>

<style scoped>
.user-monitor { max-width: 1400px; }
.hdr { margin-bottom: 14px; }
.hdr h1 { font-size: 18px; font-weight: 800; color: var(--ink); }
.hdr-sub { font-size: 13px; color: var(--ink-soft); line-height: 1.7; margin-top: 6px; }

.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); }
.empty-hint { padding: 60px 30px; text-align: center; color: var(--ink-hint); font-size: 14px; }
.empty-icon { font-size: 36px; margin-bottom: 10px; }

.monitor-layout { display: grid; grid-template-columns: 320px 1fr; gap: 14px; align-items: flex-start; }

.user-list { padding: 12px; max-height: calc(100vh - 180px); overflow-y: auto; position: sticky; top: 14px; }
.user-list-hdr { padding: 4px 6px 12px; border-bottom: 1.5px solid var(--border-light); margin-bottom: 8px; }
.search-input {
  width: 100%; padding: 8px 12px; border: 1.5px solid var(--border); border-radius: 8px;
  font-size: 13px; font-family: inherit; outline: none; transition: border-color .18s;
}
.search-input:focus { border-color: var(--sun); }
.user-count { font-size: 11px; color: var(--ink-hint); margin-top: 6px; }

.user-rows { display: flex; flex-direction: column; gap: 4px; }
.user-row {
  padding: 10px 12px; border-radius: 8px; cursor: pointer;
  border: 1.5px solid transparent; transition: all .15s;
}
.user-row:hover { background: var(--sun-light); }
.user-row.selected {
  background: linear-gradient(135deg, var(--sun-light), var(--surface));
  border-color: var(--sun);
}
.u-name { font-size: 13px; font-weight: 700; color: var(--ink); display: flex; align-items: center; gap: 6px; }
.u-email { font-size: 11px; color: var(--ink-soft); margin-top: 2px; }
.u-meta { font-size: 11px; color: var(--ink-hint); margin-top: 2px; }

.role-admin {
  font-size: 10px; font-weight: 700; padding: 1px 6px; border-radius: 4px;
  background: var(--sun); color: #fff;
}

.detail { display: flex; flex-direction: column; gap: 14px; }

.overview-card { padding: 20px 24px; }
.ov-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 14px; }
.ov-name { font-size: 18px; font-weight: 800; color: var(--ink); display: flex; align-items: center; gap: 8px; }
.ov-email { font-size: 13px; color: var(--ink-soft); margin-top: 4px; }
.ov-org { font-size: 12px; color: var(--ink-hint); margin-top: 6px; }

.ov-counts { display: flex; gap: 12px; }
.count-box {
  text-align: center; padding: 10px 20px; background: var(--surface-3);
  border: 1.5px solid var(--border-light); border-radius: 10px;
  min-width: 84px;
}
.count-num { font-size: 22px; font-weight: 800; color: var(--sun-deep); letter-spacing: -0.5px; }
.count-lbl { font-size: 11px; color: var(--ink-hint); margin-top: 2px; font-weight: 600; }

.tabs-panel { padding: 14px 20px; }

.table { width: 100%; border-collapse: collapse; }
.table th {
  padding: 10px 12px; text-align: left; font-size: 11px; font-weight: 800;
  color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em;
  border-bottom: 2px solid var(--border); background: var(--surface-3);
}
.table td { padding: 10px 12px; border-bottom: 1px solid var(--border-light); font-size: 13px; }
.table tr:hover td { background: var(--sun-light); }

.empty-cell { text-align: center; color: var(--ink-hint); padding: 40px 20px; font-size: 13px; }
.loading-cell { text-align: center; color: var(--ink-soft); padding: 30px; font-size: 13px; }

.badge { display: inline-block; font-size: 11px; font-weight: 800; padding: 2px 10px; border-radius: 20px; }
.badge-ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.badge-warn { background: var(--warn-bg); color: var(--warn); border: 1px solid var(--warn-border); }
.badge-err { background: var(--err-bg); color: var(--err); border: 1px solid var(--err-border); }
.badge-muted { background: var(--surface-3); color: var(--ink-hint); border: 1px solid var(--border); }

.role-chip { font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 4px; }
.chip-creator { background: #EEF2FF; color: #4338CA; }
.chip-assignee { background: #FEF3C7; color: #92400E; }

.btn {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 4px 10px; border-radius: 6px; border: 1.5px solid var(--border);
  background: var(--surface); cursor: pointer; font-family: inherit;
  font-size: 12px; font-weight: 600; color: var(--ink-mid);
  text-decoration: none; transition: all .15s;
}
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn-xs { padding: 3px 10px; font-size: 11px; }

.platform-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 10px 0; }
.platform-card {
  padding: 14px 16px; background: var(--surface-3); border: 1.5px solid var(--border-light);
  border-radius: 8px; display: flex; justify-content: space-between; align-items: center;
}
.platform-name { font-size: 13px; font-weight: 700; color: var(--ink); }
.platform-status { font-size: 12px; font-weight: 700; padding: 2px 10px; border-radius: 12px; }
.platform-status.ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.platform-status.off { background: var(--surface); color: var(--ink-hint); border: 1px solid var(--border); }
</style>
