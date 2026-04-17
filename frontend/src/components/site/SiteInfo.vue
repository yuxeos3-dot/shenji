<template>
  <div class="panel panel-amber" style="margin-bottom:18px">
    <div class="panel-header">
      <h2>站点信息</h2>
      <button class="btn btn-primary" @click="$emit('trigger-audit')" :disabled="auditing">
        {{ auditing ? '巡检中...' : '立即巡检' }}
      </button>
    </div>
    <div class="info-grid">
      <div class="info-item">
        <span class="info-label">域名</span>
        <span class="info-val">
          <a :href="site.domain" target="_blank" rel="noopener noreferrer" class="domain-link">
            {{ site.domain }} <span class="link-icon">↗</span>
          </a>
        </span>
      </div>
      <div class="info-item"><span class="info-label">名称</span><span class="info-val">{{ site.name }}</span></div>
      <div class="info-item"><span class="info-label">状态</span><span class="info-val"><span class="badge badge-ok">{{ site.status === 'active' ? '启用' : '停用' }}</span></span></div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  site: { type: Object, required: true },
  auditing: { type: Boolean, default: false },
})
defineEmits(['trigger-audit'])
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }
.info-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }
.info-val { font-size:14px; font-weight:600; color:var(--ink); }
.domain-link { color:var(--info); font-weight:700; font-size:14px; display:inline-flex; align-items:center; gap:4px; }
.domain-link:hover { color:var(--sun); text-decoration:underline; }
.link-icon { font-size:12px; opacity:.7; }
.badge { display:inline-block; font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep) 0%,var(--sun) 100%); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; transform:none; }
</style>
