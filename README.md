<div align="center">

![LTAPI](/web/default/public/logo.png)

# LTAPI

**基于 [New API](https://github.com/QuantumNous/new-api) 二开的 AI API 网关**

</div>

## 项目简介

LTAPI 是基于 [New API](https://github.com/QuantumNous/new-api) 进行二次开发的 AI API 网关项目。在保留 New API 全部核心功能的基础上，新增了以下功能特性。

## 二开新增功能

| 功能 | 说明 |
|------|------|
| Key 额度查询 | 开发 Key 的额度查询功能，方便用户实时查看令牌剩余额度 |
| 问答记录 | 记录每一次用户询问的问题和模型的回复，完整保留对话上下文 |
| 日志增强 | 使用日志中添加显示令牌、令牌使用者的信息，提升日志可追溯性 |
| 多维度查询 | 添加令牌、令牌使用者、令牌创建者的查询，支持按多维度筛选和检索日志 |

## 部署

### Docker Compose（推荐）

```bash
git clone https://github.com/QX9967/LTAPI.git
cd LTAPI

# 编辑 docker-compose.yml 配置
nano docker-compose.yml

# 启动服务
docker-compose up -d
```

### Docker 命令

```bash
# 使用 SQLite（默认）
docker run --name ltapi -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest
```

部署完成后访问 `http://localhost:3000` 即可使用。

## 技术栈

- **后端**: Go, Gin, GORM
- **前端**: React 19, TypeScript, Rsbuild, Tailwind CSS
- **数据库**: SQLite / MySQL / PostgreSQL

## 致谢

本项目基于 [New API](https://github.com/QuantumNous/new-api) 二次开发，感谢原项目团队的杰出工作。

## 许可证

本项目继承原项目的 [AGPLv3 许可证](./LICENSE)。
