# Lapin

Learn like a rabbit. Jump like a genius.

Lapin 是一个可自部署的多人学习平台。当前 MVP 支持：

- 邮箱注册与登录；
- 生成、查看和撤销 OpenAPI Access Token；
- 通过 OpenAPI 导入带章节和标签的科目；
- 首页集中展示科目，点击科目会在新浏览器标签页打开独立详情页；
- 科目所有者可在页面内修改科目标题、简介以及章节标题和 Markdown 正文；
- 使用 Tiptap 编辑章节正文，数据库只保存 Markdown，并支持 LaTeX 公式；
- 在章节正文上添加标注，使用透明 Excalidraw 叠层保存个人白板；
- 登录用户共同查看科目并在章节内讨论；
- PrimeVue + Vue 3 Web 界面嵌入 Go 单二进制。

## 快速启动

需要 Docker 及 Docker Compose：

```bash
docker compose up --build
```

打开 <http://localhost:8080>，注册账号即可使用。

直接启动服务或使用 Docker Compose 时，默认不创建管理员账号，也不存在默认管理员密码。如需首次启动时创建管理员，请在不会提交到 Git 的 `.env` 中成对设置：

```dotenv
ADMIN_EMAIL=<管理员邮箱>
ADMIN_PASSWORD=<由密码管理器生成的至少12字符且不超过128字节的强密码>
```

初始化在 HTTP 监听前完成。账号首次创建后，重复启动不会重置密码；如果该邮箱已经属于普通用户，服务会拒绝启动而不是自动提权。首次创建成功后可以从运行环境中移除这两个变量。

## OpenAPI 导入

先在页面右上角生成 Access Token，然后调用：

```bash
curl -X POST http://localhost:8080/openapi/v1/subjects/import \
  -H 'Authorization: Bearer lpn_替换为你的Token' \
  -H 'Content-Type: application/json' \
  -d '{
    "external_id": "go-basics-2026",
    "title": "Go 入门",
    "description": "从语言基础到 HTTP 服务",
    "tags": ["Go", "后端"],
    "chapters": [
      {"external_id": "part-language", "title": "第一部分：语言基础", "content": "", "children": [
        {"external_id": "chapter-syntax", "title": "第一章：基础语法", "content": "## 基础语法\n\n从 **package**、变量与函数开始，行内公式 $E = mc^2$。"}
      ]},
      {"external_id": "chapter-concurrency", "title": "第二章：并发", "content": "理解 goroutine 和 channel。"}
    ]
  }'
```

同一用户再次导入相同科目 `external_id` 会更新科目。导入章节也必须提供稳定且唯一的 `external_id`，重排或改名时仍会保留原章节 ID、标注、白板和讨论；导入不会自动删除本次请求中省略的旧章节。

科目所有者也可以直接在 Web 页面修改科目和章节。编辑不会更换章节 ID，因此现有标注、白板和讨论会保留；正文发生变化后，页面会提示重新校对白板与标注位置。对于带 `external_id` 的导入内容，后续再次导入时会以 OpenAPI 数据为准并可能覆盖页面内修改。

`chapters[].content` 的存储格式是 Markdown。Web 端使用 Tiptap 3 的官方 `@tiptap/markdown` 在浏览器内完成 Markdown 与编辑器状态的双向转换，公式使用 `$...$`（行内）或 `$$...$$`（块级）并由 KaTeX 展示；Go 服务端直接存取 Markdown，不做格式转换。

## CLI 课程导入

`lapin-cli` 可以把一个 JSON manifest 引用的本地 Markdown 文档安全转换成上述 OpenAPI 请求。Token 只从环境变量读取，不支持命令行 Token 参数：

```bash
make build-cli

export LAPIN_ACCESS_TOKEN='lpn_替换为你的Token'
# 连接其他 Lapin 实例时设置；默认 http://127.0.0.1:8080
export LAPIN_BASE_URL='https://lapin.example.com'

./bin/lapin-cli course import --manifest ./course/course.json
```

纯 Markdown 课程可使用 `version: 1`。章节通过 `children` 递归嵌套，`content_file` 相对 manifest 所在目录解析：

```json
{
  "version": 1,
  "external_id": "go-basics-2026",
  "title": "Go 入门",
  "description": "从语言基础到 HTTP 服务",
  "tags": ["Go", "后端"],
  "chapters": [
    {
      "external_id": "part-language",
      "title": "第一部分：语言基础",
      "children": [
        {
          "external_id": "chapter-syntax",
          "title": "第一章：基础语法",
          "content_file": "chapters/01-syntax.md"
        }
      ]
    }
  ]
}
```

带本地图片或请求体超过 1 MiB 的课程使用 `version: 2`。CLI 会创建暂存任务，将图片按每批最多 16 张上传、分批提交章节，并在所有批次完整后一次性发布课程；中途失败不会暴露半成品课程。同一课程只允许一个进行中的任务，24 小时无活动会自动失效，也可通过 `POST /openapi/v1/subject-imports/:id/abort` 主动终止：

```json
{
  "version": 2,
  "external_id": "agent-book-2026",
  "title": "深入理解 AI Agent",
  "assets": [
    {"key": "figure-01", "file": "assets/figure-01.png"}
  ],
  "chapters": [
    {
      "external_id": "chapter-01",
      "title": "第 1 章",
      "content_file": "chapters/chapter-01.md"
    }
  ]
}
```

章节 Markdown 用 `lapin-asset://figure-01` 引用 manifest 中的图片。上传成功后 CLI 会将其替换为站内预览地址。服务端只接受 PNG/JPEG，每张不超过 10 MiB；单次导入最多 64 MiB，每用户最多 5,000 张/512 MiB，实例总计最多 20,000 张/2 GiB。文件按 SHA-256 内容寻址存为 `<ASSET_DIR>/<hash前两位>/<完整hash>.<扩展名>`，相同内容只保存一份。

### PDF 转 Markdown bundle

PDF 转换是独立的 `internal/documentconv/pdf` package：通用层提取文字块、字体、坐标和图片，中文技术书籍的“第 N 章 / 图 N-N”识别放在可替换 profile 中。CLI 只负责生成可检查、可修改的 Markdown bundle。需要系统已安装 Poppler 的 `pdftohtml` 和 `pdftocairo`：

```bash
./bin/lapin-cli course prepare-pdf \
  --pdf '/absolute/path/book.pdf' \
  --output /tmp/book-bundle \
  --external-id agent-book-2026 \
  --title '深入理解 AI Agent' \
  --profile zh-technical-book

# 先检查 /tmp/book-bundle/chapters、assets 和 course.json，再导入
./bin/lapin-cli course import --manifest /tmp/book-bundle/course.json
```

PDF 的语义结构并不可靠：转换器会恢复常见段落、标题、列表、连续等宽代码行和内嵌位图，并把带可识别图题的矢量区域渲染为 PNG。多栏表格目前保留为带 `|` 分隔的可读文本，不伪装成 Web 不支持的 GFM 表格；复杂表格、跨栏排版和代码块仍应在导入前人工检查生成的 Markdown。

中文技术书籍使用 `--profile zh-technical-book`（默认），识别“第 N 章”和“图 N-N”；英文或其他按大字号二级标题分章的书籍可使用 `--profile generic-book`。

#### 两种提取引擎

`--engine layout`（默认）是上面描述的离线确定性引擎：只依赖 Poppler，不联网，逐页做版面分析。它按字号和「第 N 章」切分，适合书籍。

`--engine llm` 把每页渲染成图片，连同该页文字、全文大纲、上一页尾部和下一页开头一起交给多模态模型，按 `internal/documentconv/pdf/llm_prompt.md` 的规则重建 Markdown。它能恢复版面分析拿不到的东西——GFM 表格、带语言标注的代码围栏、稳定的标题层级——并丢弃页眉页脚。代价是每页一次网关往返、结果不可复现，且**不提取图片**（图位以 `*[图: …]*` 占位并计入 warnings）。接口手册这类表格密集、不按字号分章的文档适合它。

```bash
export LAPIN_LLM_BASE_URL='http://<gateway>/v1'
export LAPIN_LLM_MODEL='<vision-model>'
# 需要鉴权的网关再设 LAPIN_LLM_API_KEY；和 Access Token 一样只从环境变量读取

./bin/lapin-cli course prepare-pdf \
  --pdf '/absolute/path/manual.pdf' \
  --output /tmp/manual-bundle \
  --external-id nvr-openapi \
  --title '录播主机接口手册' \
  --profile generic-book \
  --engine llm --llm-dpi 300 --llm-workers 10
```

`--llm-base-url`、`--llm-model`、`--llm-outline-model` 也可用命令行传入，覆盖同名环境变量。该引擎另外需要 Poppler 的 `pdftotext` 和 `pdftoppm`，单份 PDF 最多 400 页。

结果里的 `warnings` 必须逐条看：它会报告转成空白的页、需要人工补的图位，以及**结尾落在代码围栏内的章节**——某一页的 shell 载荷换行时，模型可能在命令中间关掉围栏，未闭合的围栏会把该章后半段整体渲染成代码。图表转写同样需要复核：柱状图的数值归属会被稳定地认错，改 prompt 无法消除。

重新转换已经导入过的 PDF 时，应传入上一次审核过的 manifest：`--reuse-chapter-tree /path/to/previous/course.json`。CLI 会按章节标题复用既有 external ID 和分组树，并把新识别章节追加为新节点，避免因重新生成 ID 造成重复章节。标题发生变化时先人工更新基准 manifest 的映射。

CLI 会拒绝未知字段、非 UTF-8 文档、目录穿越、逃逸到 manifest 目录外的软链接、超出服务端限制的内容，以及对非本机地址使用明文 HTTP。它不会自动重试 POST 或跟随重定向。远程网络较慢且需要代理时，可使用标准 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY` 环境变量；不要把 Access Token 写入代理配置或日志。

白板由 Vue 组件内部的轻量 React island 挂载 MIT 授权的 Excalidraw。保存时只持久化 Excalidraw elements 和必要的透明背景状态，不保存 camera/session，也暂不接受图片；外层契约包含章节 HashID、正文 SHA-256 版本及稳定参考尺寸。题目内容和透明画布共享同一局部坐标系，窗口变化只调整渲染比例，不会反复缩放并回写数据库坐标。旧版 tldraw 快照会保留并显示明确提示，只有用户选择新建并保存 Excalidraw 白板后才会替换。当前 MVP 以章节为锚点，后续可细化到题目或内容块。

数据库内部所有实体和关联记录均使用 PostgreSQL 自增 `BIGINT`。HTTP API 只返回 HashID 字符串，Web 端不会看到数据库原始 long ID。

## 代码结构

- `internal/httpapi/server.go`：集中列出每个 HTTP 地址及对应 handler；
- `internal/httpapi/handler/`：独立的 HTTP handler package；
- `internal/database/`：按表拆分的 Go 数据访问文件，例如 `users.go`、`chapters.go`、`comments.go`；
- `migrations/migrations.go`：嵌入根目录 `migrations/*.sql`；
- `web/web.go`：嵌入 `web/dist` 并提供 SPA fallback。

## 本地开发

```bash
# 使用本机 127.0.0.1:5433 的 PostgreSQL（先创建一次隔离测试库）
PGPASSWORD=postgres createdb -h 127.0.0.1 -p 5433 -U postgres lapin_test

# 服务端
DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable' \
HASHID_SALT='replace-with-a-private-stable-value' go run ./cmd/lapin

# 或使用项目锁定版本的 Air 监听 Go/迁移 SQL，同时启动 Vite 前端开发服务
make watch
# 打开 http://localhost:5173；退出 make watch 时两个开发进程会一并停止
# 仅 make watch 会为本地开发默认创建 admin@localhost / admin12345678
# 默认账号仅允许 development + loopback HTTP_ADDR + loopback PostgreSQL
# 可通过环境变量成对设置 ADMIN_EMAIL、ADMIN_PASSWORD 覆盖，不要作为 make 命令行变量传入
# 凭据仅交给一次性 bootstrap，不传给 npm、Go build、Air 或 Vite
# PostgreSQL 配置统一写入 DATABASE_URL；make watch 会拒绝 PGPASSWORD 等 PG* 环境变量

# Web（另一个终端）
npm --prefix web install
npm --prefix web run dev
```

## 验证

```bash
make test
make build
# 预先安装固定版本 playwright-cli 0.1.18；Vite 已在 5173 运行时执行真实 Chromium 回归
make test-browser
```

配置项：

| 环境变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `DATABASE_URL` | 是 | - | PostgreSQL 连接串 |
| `HTTP_ADDR` | 否 | `:8080` | Hertz 监听地址 |
| `APP_ENV` | 否 | `development` | 生产环境设置为 `production` |
| `SECURE_COOKIES` | 否 | `false` | HTTPS 部署时设为 `true` |
| `HASHID_SALT` | 否 | 仅开发默认值 | HashID 盐；`APP_ENV=production` 时必须为至少 32 字符的私有稳定值 |
| `TRUSTED_PROXY_CIDRS` | 否 | 空 | 可信反向代理网段，逗号分隔；仅这些来源可提供客户端 IP 转发头 |
| `ADMIN_EMAIL` | 否 | 空 | 首次管理员邮箱；必须与 `ADMIN_PASSWORD` 成对设置 |
| `ADMIN_PASSWORD` | 否 | 空 | 首次管理员密码，至少 12 个 Unicode 字符且 UTF-8 不超过 128 字节；不会覆盖已有管理员密码 |
| `ASSET_DIR` | 否 | `data/assets` | 图片内容存储目录；生产和容器部署应挂载持久卷 |

默认 Compose 仅绑定 `127.0.0.1:8080`，适合本机体验。远程部署请放在 HTTPS 反向代理后面，并设置 `APP_ENV=production`、`SECURE_COOKIES=true` 和反向代理实际来源的 `TRUSTED_PROXY_CIDRS`；数据库密码和 `HASHID_SALT` 也应改为部署环境的私密配置。生产模式会拒绝不安全的 Cookie 配置。
