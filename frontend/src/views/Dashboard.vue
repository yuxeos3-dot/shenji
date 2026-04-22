<template>
  <div>
    <!-- Stats Strip -->
    <div class="stats-grid">
      <div class="stat-card" v-for="stat in stats" :key="stat.label">
        <div class="stat-icon">{{ stat.icon }}</div>
        <div class="stat-label">{{ stat.label }}</div>
        <div class="stat-val" :class="stat.cls">{{ stat.value }}</div>
        <div class="stat-sub">{{ stat.sub }}</div>
      </div>
    </div>

    <!-- Online Users — admin-only. Members not only don't see other
         people's IPs, the query hook itself doesn't fire so there's
         no 403-toast noise on mount. -->
    <div class="panel online-panel" v-if="isAdmin && online.users.length">
      <div class="panel-header">
        <h2><span class="online-dot"></span> 在线用户 <span class="online-badge">{{ online.online_count }} 个会话 · {{ online.user_count }} 人在线</span></h2>
      </div>
      <div class="online-grid">
        <div class="online-card" v-for="u in online.users" :key="u.user_id">
          <div class="online-avatar">{{ (u.name||'U')[0] }}</div>
          <div class="online-info">
            <div class="online-name">{{ u.name }} <span class="online-role">{{ u.role }}</span></div>
            <div class="online-ips">
              <span class="ip-tag" v-for="ip in u.ips" :key="ip">{{ ip }}</span>
            </div>
          </div>
          <div class="online-sessions" v-if="u.session_count > 1">{{ u.session_count }} 会话</div>
        </div>
      </div>
    </div>

    <!-- Main content -->
    <div class="dash-row">
      <div class="panel" style="flex:2">
        <div class="panel-header">
          <h2>最近审计记录</h2>
          <router-link to="/sites" class="btn-link">查看全部 →</router-link>
        </div>
        <table class="table">
          <thead><tr><th>站点</th><th>归属</th><th>状态</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="s in sites.filter(s => s.last_audit_status)" :key="s.id">
              <td style="font-weight:700">{{ s.name }}</td>
              <td class="owner-cell">
                <span class="owner-name">{{ s.owner_name || '—' }}</span>
                <span v-if="s.department_name || s.group_name" class="owner-meta">
                  {{ [s.department_name, s.group_name].filter(Boolean).join(' · ') }}
                </span>
              </td>
              <td><span class="badge" :class="s.last_audit_status==='done'?'badge-ok':s.last_audit_status==='running'?'badge-warn':'badge-err'">{{ s.last_audit_status }}</span></td>
              <td style="color:var(--ink-soft);font-size:12px">{{ s.last_audit_time || '-' }}</td>
            </tr>
            <tr v-if="!sites.filter(s => s.last_audit_status).length"><td colspan="4" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
          </tbody>
        </table>
      </div>

      <div class="panel" style="flex:1">
        <div class="panel-header"><h2>站点健康</h2></div>
        <div v-for="site in sites" :key="site.id" class="health-item" @click="$router.push(`/sites/${site.id}`)">
          <div class="health-left">
            <span class="health-name">{{ site.name }}</span>
            <span v-if="site.owner_name || site.department_name" class="health-owner">
              {{ site.owner_name || '—' }}<span v-if="site.department_name || site.group_name" class="health-team">
                · {{ [site.department_name, site.group_name].filter(Boolean).join(' / ') }}
              </span>
            </span>
          </div>
          <span class="badge" :class="site.last_audit_status==='done'?'badge-ok':site.last_audit_status==='error'?'badge-err':'badge-muted'">
            {{ site.last_audit_status || '未审计' }}
          </span>
        </div>
        <div v-if="!sites.length" style="text-align:center;color:var(--ink-hint);padding:30px;font-size:13px">
          暂无站点，<router-link to="/sites">去添加</router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useSites, useOnlineUsers } from '../composables/useApi'
import { useUserStore } from '../stores/user'

const userStore = useUserStore()
const isAdmin = computed(() => userStore.isAdmin())

const { data: sitesData } = useSites()
// Only admins hit /v1/auth/online — the endpoint is AdminRequired on
// the backend, so calling it from a member view would surface a 403
// error toast + pollute the audit log. Conditional enabling means the
// query never fires for members and their Dashboard is clean.
const { data: onlineData } = useOnlineUsers(isAdmin)

const sites = computed(() => sitesData.value || [])
const online = computed(() => onlineData.value || { online_count: 0, user_count: 0, users: [] })

// Stats row: "在线人数" card is admin-only for the same reason the
// online panel is. Members see the three site-related cards only.
const stats = computed(() => {
  const base = [
    { label: '站点总数', value: sites.value.length, icon: '🌐', cls: 'val-sun', sub: '已接入' },
    { label: '健康站点', value: sites.value.filter(s => s.last_audit_status === 'done').length, icon: '✅', cls: 'val-ok', sub: '审计通过' },
    { label: '待审计', value: sites.value.filter(s => !s.last_audit_status).length, icon: '⏳', cls: 'val-warn', sub: '未开始' },
  ]
  if (isAdmin.value) {
    base.push({
      label: '在线人数',
      value: online.value.user_count,
      icon: '👥', cls: 'val-ok',
      sub: online.value.online_count + ' 个会话',
    })
  }
  return base
})
</script>

<style scoped>
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; margin-bottom: 24px; }
.stat-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  padding: 20px;
  box-shadow: var(--s1);
  transition: all .2s;
}
.stat-card:hover { transform: translateY(-2px); box-shadow: var(--s2); }
.stat-icon { font-size: 22px; margin-bottom: 10px; }
.stat-label { font-size: 11px; font-weight: 700; color: var(--ink-hint); text-transform: uppercase; letter-spacing: .06em; margin-bottom: 6px; }
.stat-val { font-size: 32px; font-weight: 800; letter-spacing: -1px; line-height: 1; margin-bottom: 4px; }
.val-sun { color: var(--sun); }
.val-ok { color: var(--ok); }
.val-warn { color: var(--warn); }
.val-err { color: var(--err); }
.stat-sub { font-size: 11px; color: var(--ink-hint); }

.dash-row { display: flex; gap: 18px; }

.panel {
  background: var(--surface);
  border-radius: var(--r);
  box-shadow: var(--s1);
  border: 1.5px solid var(--border-light);
  padding: 22px 26px;
}
.panel-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.panel-header h2 { font-size: 15px; font-weight: 700; }
.btn-link { font-size: 13px; font-weight: 600; color: var(--sun); }
.btn-link:hover { color: var(--sun-deep); }

.table { width: 100%; border-collapse: collapse; }
.table th {
  padding: 10px 14px; text-align: left; font-size: 11px; font-weight: 800;
  color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em;
  border-bottom: 2px solid var(--border); background: var(--surface-3);
}
.table td { padding: 10px 14px; border-bottom: 1px solid var(--border-light); font-size: 13px; }
.table tr:hover td { background: var(--sun-light); }

.badge {
  display: inline-block; font-size: 11px; font-weight: 800; padding: 2px 10px;
  border-radius: 20px; text-transform: uppercase;
}
.badge-ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.badge-warn { background: var(--warn-bg); color: var(--warn); border: 1px solid var(--warn-border); }
.badge-err { background: var(--err-bg); color: var(--err); border: 1px solid var(--err-border); }
.badge-muted { background: var(--surface-3); color: var(--ink-hint); border: 1px solid var(--border); }

.health-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 0; border-bottom: 1px solid var(--border-light);
  cursor: pointer; transition: all .15s;
}
.health-item:hover { padding-left: 6px; }
.health-item:last-child { border-bottom: none; }
.health-left { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.health-name { font-size: 13px; font-weight: 600; color: var(--ink); }
.health-owner { font-size: 11px; color: var(--ink-soft); font-weight: 500; }
.health-team { color: var(--ink-hint); }
.owner-cell { font-size: 12px; }
.owner-name { font-weight: 700; color: var(--ink-mid); }
.owner-meta { display: block; font-size: 11px; color: var(--ink-soft); margin-top: 2px; }

.online-panel { margin-bottom: 18px; }
.online-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #22C55E; margin-right: 6px; animation: pulse-dot 1.5s infinite; }
@keyframes pulse-dot { 0%, 100% { opacity: 1; } 50% { opacity: .4; } }
.online-badge { font-size: 12px; font-weight: 500; color: var(--ink-hint); margin-left: 8px; }
.online-grid { display: flex; flex-wrap: wrap; gap: 12px; }
.online-card { display: flex; align-items: center; gap: 10px; padding: 10px 14px; background: var(--surface-2, #f9fafb); border: 1px solid var(--border-light); border-radius: 10px; min-width: 240px; }
.online-avatar { width: 34px; height: 34px; border-radius: 50%; background: linear-gradient(135deg, var(--sun), var(--coral, #f97316)); display: flex; align-items: center; justify-content: center; font-size: 14px; font-weight: 700; color: #fff; flex-shrink: 0; }
.online-info { flex: 1; min-width: 0; }
.online-name { font-size: 13px; font-weight: 700; color: var(--ink); }
.online-role { font-size: 11px; font-weight: 500; color: var(--ink-hint); background: var(--surface-3, #f3f4f6); padding: 1px 6px; border-radius: 4px; margin-left: 4px; }
.online-ips { display: flex; gap: 4px; flex-wrap: wrap; margin-top: 3px; }
.ip-tag { font-size: 11px; font-family: 'Consolas', monospace; color: var(--sun); background: rgba(249,115,22,.08); padding: 1px 6px; border-radius: 4px; }
.online-sessions { font-size: 11px; font-weight: 700; color: var(--sun); background: rgba(249,115,22,.1); padding: 3px 8px; border-radius: 6px; flex-shrink: 0; }
</style>
