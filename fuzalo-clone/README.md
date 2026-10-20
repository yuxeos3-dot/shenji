# fuzalo-clone — 黄果短剧官网镜像

`https://fuzalo.com/` 的原版静态镜像（Vite 预渲染 React 站，HTML 字节保真）。

## 内容
- `site/` — 服务器布局镜像：8 个页面（`/`、`/download/`、`/ranking/`、`/daily/`、`/faq/`、`/reviews/`、`/creators/`、`/sitemap/`）+ 35 个资源 + `sitemap.xml`/`robots.txt`/`llms.txt`/`.well-known/`
- `start.bat` — 本地启动器（Python 静态服务 :8130）

## 本地运行
```bash
cd site && python -m http.server 8130
```
目录式 URL 与 React hydration 需走 HTTP 服务，不能用 `file://` 直接打开。
