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

    <!-- Main content -->
    <div class="dash-row">
      <div class="panel" style="flex:2">
        <div class="panel-header">
          <h2>最近审计记录</h2>
          <router-link to="/sites" class="btn-link">查看全部 →</router-link>
        </div>
        <table class="table">
          <thead><tr><th>站点</th><th>状态</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="s in sites.filter(s => s.last_audit_status)" :key="s.id">
              <td style="font-weight:700">{{ s.name }}</td>
              <td><span class="badge" :class="s.last_audit_status==='done'?'badge-ok':s.last_audit_status==='running'?'badge-warn':'badge-err'">{{ s.last_audit_status }}</span></td>
              <td style="color:var(--ink-soft);font-size:12px">{{ s.last_audit_time || '-' }}</td>
            </tr>
            <tr v-if="!sites.filter(s => s.last_audit_status).length"><td colspan="3" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
          </tbody>
        </table>
      </div>

      <div class="panel" style="flex:1">
        <div class="panel-header"><h2>站点健康</h2></div>
        <div v-for="site in sites" :key="site.id" class="health-item" @click="$router.push(`/sites/${site.id}`)">
          <span class="health-name">{{ site.name }}</span>
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

<script setup>
import { ref, onMounted, computed } from 'vue'
import api from '../api'

const sites = ref([])
const stats = computed(() => [
  { label: '站点总数', value: sites.value.length, icon: '🌐', cls: 'val-sun', sub: '已接入' },
  { label: '健康站点', value: sites.value.filter(s => s.last_audit_status === 'done').length, icon: '✅', cls: 'val-ok', sub: '审计通过' },
  { label: '待审计', value: sites.value.filter(s => !s.last_audit_status).length, icon: '⏳', cls: 'val-warn', sub: '未开始' },
  { label: '异常站点', value: sites.value.filter(s => s.last_audit_status === 'error').length, icon: '⚠️', cls: 'val-err', sub: '需关注' },
])

onMounted(async () => {
  try { const res = await api.listSites(); sites.value = res.data } catch {}
})
</script>

<style scoped>
.stats-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 24px; }
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
.health-name { font-size: 13px; font-weight: 600; color: var(--ink); }
</style>
