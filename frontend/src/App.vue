<script setup lang="ts">
import ErrorBoundary from "./components/ErrorBoundary.vue"
</script>

<template>
<!-- Global error boundary wraps all route views -->

  <ErrorBoundary>
    <router-view />
  </ErrorBoundary>
</template>

<style>
@import url('https://fonts.googleapis.com/css2?family=Noto+Sans+SC:wght@400;500;600;700;900&family=JetBrains+Mono:wght@400;500&display=swap');

:root {
  --sun:          #FF8C00;
  --sun-deep:     #F57C00;
  --sun-xdeep:    #E65100;
  --sun-light:    #FFF8F0;
  --sun-glow:     #FFF0DE;
  --gold:         #FFB300;
  --gold-deep:    #FFA000;
  --peach:        #FFD6A5;
  --coral:        #FF9F43;
  --bg:           #FFFFFF;
  --surface:      #FFFFFF;
  --surface-2:    #FEFCFA;
  --surface-3:    #FFF8F2;
  --border:       #E8DDD4;
  --border-light: #F3EDE7;
  --ink:          #2D1B0E;
  --ink-mid:      #5D4037;
  --ink-soft:     #8D7060;
  --ink-hint:     #BCA898;
  --ok:           #16A34A;
  --ok-bg:        #F0FDF4;
  --ok-border:    #BBF7D0;
  --err:          #DC2626;
  --err-bg:       #FFF5F5;
  --err-border:   #FECACA;
  --warn:         #D97706;
  --warn-bg:      #FFFBEB;
  --warn-border:  #FDE68A;
  --info:         #2563EB;
  --purple:       #7C3AED;
  --s1: 0 1px 3px rgba(150,60,0,.07), 0 1px 2px rgba(150,60,0,.05);
  --s2: 0 4px 16px rgba(150,60,0,.10), 0 2px 5px rgba(150,60,0,.06);
  --s3: 0 10px 36px rgba(150,60,0,.13), 0 4px 10px rgba(150,60,0,.08);
  --s-amber: 0 4px 18px rgba(249,115,22,.36);
  --r:    14px;
  --r-sm: 9px;
  --r-xs: 6px;
  --ease: cubic-bezier(.4,0,.2,1);
  --t:    .18s var(--ease);
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Noto Sans SC', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Helvetica Neue', sans-serif;
  font-size: 14px;
  background: var(--bg);
  color: var(--ink);
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}
a { color: var(--sun); text-decoration: none; font-weight: 600; }
a:hover { color: var(--sun-deep); }

::-webkit-scrollbar { width: 5px; height: 5px; }
::-webkit-scrollbar-track { background: var(--surface-2); }
::-webkit-scrollbar-thumb { background: var(--peach); border-radius: 3px; }

/* Override Element Plus theme */
.el-button--primary { background: linear-gradient(135deg, var(--sun) 0%, var(--coral) 100%) !important; border-color: transparent !important; box-shadow: var(--s-amber) !important; font-weight: 700 !important; }
.el-button--primary:hover { background: linear-gradient(135deg, var(--sun-deep) 0%, var(--sun) 100%) !important; box-shadow: 0 8px 24px rgba(249,115,22,.48) !important; }
.el-button--danger { background: linear-gradient(135deg, #DC2626, #EF4444) !important; border-color: transparent !important; }
.el-button--success { background: linear-gradient(135deg, #16A34A, #22C55E) !important; border-color: transparent !important; }
.el-button--warning { background: linear-gradient(135deg, #D97706, #F59E0B) !important; border-color: transparent !important; color: #fff !important; }
.el-button { font-family: 'Noto Sans SC', 'PingFang SC', 'Microsoft YaHei', sans-serif !important; font-weight: 700 !important; border-radius: var(--r-sm) !important; }
.el-input__wrapper { border-radius: var(--r-sm) !important; }
.el-input__wrapper:focus-within, .el-input__wrapper.is-focus { box-shadow: 0 0 0 1px var(--sun) !important; }
.el-card { border-radius: var(--r) !important; border: 1.5px solid var(--border-light) !important; box-shadow: var(--s1) !important; }
.el-card:hover { box-shadow: var(--s2) !important; }
.el-dialog { border-radius: var(--r) !important; }
.el-tag { border-radius: var(--r-xs) !important; font-weight: 700 !important; }
.el-table th.el-table__cell { background: linear-gradient(180deg, var(--surface-3) 0%, #FDECD4 100%) !important; font-weight: 800 !important; text-transform: uppercase !important; letter-spacing: .08em !important; font-size: 11px !important; color: var(--ink-mid) !important; }
.el-table td.el-table__cell { border-bottom: 1px solid var(--border-light) !important; }
.el-table--striped .el-table__body tr.el-table__row--striped td.el-table__cell { background: var(--surface-2) !important; }
.el-table__body tr:hover > td.el-table__cell { background: var(--sun-light) !important; }
.el-menu { border-right: none !important; }
.el-tabs__item.is-active { color: var(--sun) !important; font-weight: 700 !important; }
.el-tabs__active-bar { background-color: var(--sun) !important; }
.el-progress-bar__inner { background: linear-gradient(135deg, var(--sun) 0%, var(--coral) 100%) !important; }
.el-descriptions__label { font-weight: 700 !important; color: var(--ink-mid) !important; }
.el-page-header__back .el-icon { color: var(--sun) !important; }
</style>
