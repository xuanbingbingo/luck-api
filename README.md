# Luck API

私有部署版本，AI 模型聚合与分发网关。

本项目 fork 自 [QuantumNous/new-api](https://github.com/QuantumNous/new-api)（AGPL-3.0），仅在 AGPL 允许的范围内对前端用户可见品牌做了最小改动（HTML title、Logo 内嵌 title 文本），并以新名称 `luck-api` 部署运行。

- 上游原 README：见 [`README.upstream.md`](./README.upstream.md)
- 上游许可：[AGPL-3.0](./LICENSE)（继续遵守，本 fork 同样以 AGPL-3.0 开源）
- 上游版权：原文件版权头与 `LICENSE` 中的 `Copyright (C) ... QuantumNous` 全部保留
- Go module path 保持 `github.com/QuantumNous/new-api` 不变（不改 import，便于跟踪上游更新）
- 上游开发规范：见 [`README.upstream.md`](./README.upstream.md) 与原 `CLAUDE.md`

## 修改清单（相对上游）

| 文件 | 改动 |
|---|---|
| `web/default/index.html` | `<title>` + `<meta name="title">` → `Luck API` |
| `web/classic/index.html` | `<title>` → `Luck API` |
| `web/default/src/assets/logo.tsx` | SVG `<title>` → `Luck API` |
| `VERSION` | 写入 `v0.12.10-luck.1` 标识本 fork 版本 |
| `README.md` | 替换为本说明（原文件保留为 `README.upstream.md`） |

## 部署

```
docker compose up -d
```

镜像：`luck-api:0.1.0`（本地 docker build，服务器 docker load 部署，详见运维 memory）

数据目录：`./data/`（SQLite 数据库 + logs）
