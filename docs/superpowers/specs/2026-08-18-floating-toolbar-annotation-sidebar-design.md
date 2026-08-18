# 浮动工具栏、可隐藏标注侧边栏与正文内标注标记

日期：2026-08-18
状态：已确认，待实现

## 背景

章节学习页当前的三处问题：

1. 白板开关按钮孤立在正文右上角 `.chapter-document-actions`，标注的创建入口则在右侧 `.annotation-panel` 表单里，两个高频动作分居两处。
2. 右侧标注栏是固定分栏（`.notes-grid` 的第二列），永远占宽，正文和白板都被挤窄，且无法收起。
3. 标注只以 `start_offset` / `end_offset` 字符偏移存在数据库里，正文中看不出哪里被标注过，标注与正文彻底脱节。

顶部还有一条《正文与标注 / 讨论》tab，与新浮动栏的职能重叠。

## 目标

- 白板与标注的入口合并成一条 iOS 风格的浮动药丸工具栏，内容随上下文变形。
- 标注与讨论收进一个可收起的右侧栏，正文默认全宽。
- 正文中被标注的文字带可点击的标记，点击定位到对应标注详情。

## 非目标

- 不改标注的存储格式（仍是渲染文本的 UTF-16 字符偏移）。
- 不新增标注编辑 / 删除接口。
- 不做"标注卡 → 滚动到正文标记"的反向定位（可作为后续增量）。
- 不改白板的窗口化渲染逻辑。

## 约束

- 服务端 `CreateAnnotation` 要求 `note` 非空（`internal/httpapi/handler/interactions.go`），因此不存在"点个颜色就存下的纯高亮"，创建流程必须走到富文本输入。
- HTTP 路由只允许 GET 与 POST，本设计不新增任何路由。
- 白板激活时 `.whiteboard-content-layer` 是 `pointer-events: none`，正文不可选中、标记不可点击，这是既有且期望的行为。

## 组件划分

### 1. `web/src/components/ChapterToolbar.vue`（新）

固定在正文列底部中央的浮动药丸条。圆角全椭、半透明毛玻璃（`backdrop-filter: blur`）、`1px solid #dcddd4` 细边与轻投影，配色沿用现有暖纸色（底 `#fbfaf6`，前景 `#33483e`）。

Props：`mode`、`whiteboardActive`、`whiteboardDisabled`、`whiteboardLoading`、`commentCount`、`annotationCount`、`quote`、`color`、`saving`。
Emits：`toggle-whiteboard`、`open-sidebar(tab)`、`pick-color(color)`、`compose-annotation`、`cancel-selection`、`undo`、`redo`、`clear`、`save-whiteboard`。

`mode` 由父组件按状态推导，三态之间宽度做 200ms 过渡：

| mode | 内容 | 进入条件 |
| --- | --- | --- |
| `reading` | `▣ 白板`、`✎ 标注 N`、`💬 讨论 N` | 默认 |
| `selecting` | 引用摘要、四色点、`写标注 →`、`✕` | 正文存在选区 |
| `whiteboard` | `▣ 白板(已开)` │ `↶ ↷ 🗑 ✓` │ `✎ 标注 N` | 白板已开启 |

`mode` 的推导优先级：有选区 → `selecting`；否则白板已开 → `whiteboard`；否则 `reading`。白板开启时正文不可选中，因此前两者不会同时成立。

白板的撤销 / 重做 / 清空 / 保存从 Excalidraw 自带工具栏搬到这里，`excalidrawBridge.ts` 的 `ToolbarExtension` 只保留拖动把手，Excalidraw 原工具栏只剩绘制工具，避免同一动作两个入口。`ExcalidrawWhiteboard.vue` 通过 `defineExpose` 暴露 `undo` / `redo` / `clear` / `save`，转调既有的 `ExcalidrawBridge` 方法。

`reading` 态点 `✎ 标注` 等于打开侧边栏的标注页；`selecting` 态点 `写标注 →` 展开侧边栏并把引用与颜色带进新建区。

### 2. `web/src/components/AnnotationSidebar.vue`（新）

右侧栏，宽 20rem，收起用 `transform: translateX(100%)`，`transition` 220ms。把手是贴在侧边栏左边缘垂直居中的半圆按钮，收起时贴在屏幕右缘；按钮带 `aria-expanded` 与中文 `title`（`展开标注栏` / `收起标注栏`）。

内部两个 tab：

- **标注**：顶部新建区（引用摘要 → 四色点 → `RichTextEditor` → `保存标注`），下方标注卡列表，卡片支持 `selected` 高亮。
- **讨论**：现有 `comments-panel` 的内容整体迁入（发布框 + 列表 + 空态）。

Props：`open`、`tab`、`annotations`、`comments`、`draft`、`activeAnnotationId`、`user`、`saving`。
Emits：`update:open`、`update:tab`、`update:draft`、`save-annotation`、`post-comment`。

外部把 `activeAnnotationId` 置为某条标注时，侧边栏展开对应 tab、高亮该卡并 `scrollIntoView`。

顶部 `.study-tabs` 整条删除，`.notes-grid` 改为单列，正文与白板永远全宽。窄屏（<900px）侧边栏改为盖在正文之上的抽屉，不再挤压正文列。

### 3. `web/src/annotationMarks.ts`（新）

导出纯函数 `annotationDecorationRanges(doc, annotations)`，返回 `{ id, from, to, color }[]`。

映射规则必须与 `RichTextContent.vue` 发射选区时完全一致：以 `textBetween(0, pos, '', leafText)` 的长度为准，`inlineMath` / `blockMath` 取 `attrs.latex`，长度按 JS 字符串长度（UTF-16 码元）计，与服务端 `utf16.Encode` 的校验口径一致。

映射不成立的标注（章节正文已改、偏移越界、区间为空）直接跳过，不产出装饰、不抛错。

### 4. `web/src/components/RichTextContent.vue`（改）

新增 `annotations` prop 与 `annotation-click` emit。用一个 ProseMirror 插件把 `annotationDecorationRanges` 的结果转成 `Decoration.inline`，带 `data-annotation-id` 与颜色 class；编辑器根节点上做 click 委托，命中最内层 `[data-annotation-id]` 时派发。`annotations` 变化时刷新装饰。

标记样式：对应颜色的底色高亮，右上角小圆点角标，hover 加深。重叠标注装饰天然可叠，点击取最内层。

### 5. `web/src/components/DashboardView.vue`（改）

新增状态 `sidebarOpen`、`sidebarTab`、`activeAnnotationId`；删除 `activeTab` / `.study-tabs` / `.annotation-panel` / `.chapter-document-actions`；把标注与讨论的渲染交给 `AnnotationSidebar`，把白板与标注入口交给 `ChapterToolbar`。切换章节时收起选区态并清空 `activeAnnotationId`。

## 数据流

```
正文选区 → RichTextContent selection → ExcalidrawWhiteboard 透传 → DashboardView.captureSelection
   → ChapterToolbar 进入 selecting 态 → 选色 → 写标注
   → AnnotationSidebar 展开、引用与颜色预填、输入框聚焦 → 保存
   → api.createAnnotation → 重拉 annotations → 装饰重算 → 正文出现标记
   → 点标记 → activeAnnotationId → 侧边栏展开标注页并高亮该卡
```

## 错误处理

- 创建标注失败：沿用既有 `showError`，草稿与选区保留，不清空输入。
- 偏移映射失败：跳过装饰，标注卡照常显示在侧边栏。
- 白板加载失败：`ChapterToolbar` 的白板按钮显示重试态，复用现有 `whiteboardLoadError` 分支。

## 测试

先写测试再实现。

- `annotationMarks.spec.ts`：普通段落偏移映射、含数学节点、越界跳过、区间为空跳过、重叠标注。
- `ChapterToolbar.spec.ts`：三种 mode 的按钮集合、白板未加载完的禁用态、各按钮事件。
- `AnnotationSidebar.spec.ts`：把手收展与 `aria-expanded`、tab 切换、引用与颜色预填、保存事件、`activeAnnotationId` 高亮。
- `RichTextContent.spec.ts`：按 annotations 渲染标记、点击派发 id、annotations 变化后刷新。
- `DashboardView.spec.ts`：更新现有 11 个用例（顶部 tab 已删除），新增「选中文字后浮动栏进入 selecting 并可写标注」「点正文标记后侧边栏展开并高亮对应卡」两条串联用例。
- 浏览器：按 CLAUDE.md 跑注册 → 学习的完整路径，确认浮动栏、侧边栏、标记在真实渲染下的表现。

## 验收

```sh
cd web && npm test && npm run typecheck && npm run build
TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable' go test -race -coverpkg=./... ./...
go vet ./...
go build ./cmd/lapin
```

注意：上面的 Go 测试会清空 `lapin_test`，而 `make watch` 用的是同一个库，跑完需要重建管理员并重新导入课程。
