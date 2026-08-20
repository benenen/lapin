# Flutter 端章节标注设计（按 quote 锚定）

移动端读者可以在章节正文里看到标注高亮、点开详情，并选中文字新建标注。锚点是引用文字本身，字符偏移只作消歧提示。

## 为什么按 quote 锚定

偏移一致性 spike 已经证实：Flutter 与 Web 的 Markdown 解析器把同一篇正文展平成的字符串长度不同（第 1 章 27,586 对 27,612 个字符，共 26 处分歧），且没有任何解析器配置能让两者对齐。前 31% 完全相同，因此抽查会误判为一致。

结论：跨端不能共享「渲染文本上的偏移」。两端共享的唯一确定物是章节的 Markdown 源。

## 服务端契约（既有，不改）

- `GET /api/v1/chapters/:id/annotations` — 返回整章标注，**不按用户过滤**，每条带 `author_name`。
- `POST /api/v1/chapters/:id/annotations` — 需要会话与 CSRF 头。

`CreateAnnotation` 的三道校验都针对**原始 Markdown**（`chapter.Content`），不是渲染后的文本：

1. `end_offset ≤ len(utf16(chapter.Content))`
2. `end_offset - start_offset == len(utf16(quote))`
3. `strings.Contains(chapter.Content, quote)`

另有：`note` 非空且 ≤ 2000 字符，`quote` ≤ 1000 字符，`color` 属于允许集合（默认 `yellow`）。

**offset 基准取 Markdown 源。** 在 `chapter.content` 里定位 quote，所得 UTF-16 下标直接作为 offset，三道校验确定性通过。

Web 端存的是渲染文本上的偏移（`web/src/components/RichTextContent.vue:63` 用 `textBetween` 算前缀长度），与本端基准不同。这不影响互通：两端都以 quote 定位、offset 只作消歧提示，跨端显示仍然正确，只是提示的精度下降。

## 组件

### `domain/quote_anchor.dart` — 纯锚定逻辑

不依赖 Flutter，全部可单测。

- `locateQuote(source, quote, hint)` — 在 Markdown 源里找 quote，多处命中时取距 `hint` 最近的一处，返回 UTF-16 起点；找不到返回 `null`。
- `matchSelectionInSource(source, selection)` — 把**渲染文本**的选区还原成源里的字面子串。先精确搜；失败则把 selection 里的空白串放宽成 `\s+` 再搜。返回源里匹配到的那一段及其起点。

放宽空白是必需的：章节 1–4 的正文含块内单换行，`flutter_markdown_plus` 默认把软换行折成空格（`builder.dart` 的 `trimText`），所以跨换行的选区精确搜必然失败。存进服务端的 quote 取**源里匹配到的那一段**，因此始终是字面子串。

两种搜法都不中就报错让用户重选，不做猜测性修正。

### `presentation/annotation_spans.dart` — 正文内高亮

注册一个 `md.InlineSyntax`，直接匹配标注的引用文字（不改写 Markdown 源），生成嵌套元素：

```
a(href: "lapin-annotation:<id>") > ann(引用文字)
```

- 外层 `a` 让 `MarkdownBuilder` 建立 link handler，从而给文本 span 挂上 `TapGestureRecognizer`；点击经 `onTapLink` 打开详情。
- 内层 `ann` 从 `styleSheet.styles['ann']` 取高亮底色。`_styles` 是可增长的 map 字面量、`styles` getter 返回其本身，因此可以注入自定义标签的样式。合并顺序让 `ann` 覆盖链接色。

两层都是真正的 `TextSpan`，折行、选中、字距均正常。

**不能用 `MarkdownElementBuilder`**：它只能返回 `Widget`，会被包成原子的 `WidgetSpan`，一句话长的引用无法折行。

同一引用多处出现时，`InlineSyntax` 用匹配位置与存储 offset 的距离决定是否接受本次命中，只高亮意图中的那一处。

### `data/annotation_repository.dart`

`list(chapterId)` 与 `create(...)`。provider 显式依赖 `sessionProvider` —— 与课程列表同因：`go_router` 先构建初始路由再重定向，未登录时的 401 会被 Riverpod 缓存住。

### `presentation/` 其余

- `annotation_detail_sheet.dart` — 引用、笔记、作者、时间。
- `create_annotation_sheet.dart` — 笔记输入 + 颜色选择，字数上限与服务端一致。

### 创建流程

`Markdown` 的 `onSelectionChanged(text, selection, cause)` 给出所在块文本与选区（选区不跨块，因为 `selectable` 按块生成 `SelectableText`——这反而提高了选中文字命中源子串的概率）。取到选中文字后经 `matchSelectionInSource` 还原为源子串与起点，连同笔记、颜色提交。

## 错误处理

- 选区还原不到源子串 → 提示换一段选，不提交。
- 创建返回 400 → 展示服务端的中文消息。
- 401 → 走既有的会话失效路径回登录页。
- 列表加载失败 → 正文照常渲染，只是没有高亮；标注不该挡住阅读。

## 测试

- `quote_anchor` 单测：多处出现取最近、CJK 的 UTF-16 计数、跨软换行的容错匹配、匹配不到返回 null、quote 为空。
- widget 测试：假 repository 下高亮渲染出来，点击打开详情。
- e2e 扩一条：新建标注后正文出现标记。

## 不做

- 编辑与删除标注：服务端没有对应路由（HTTP 只允许 GET/POST，撤销需建模为显式 POST 动作路径），本轮不加服务端。
- 与 Web 端偏移基准对齐：已证实不可行。
- 白板：另一件事。
