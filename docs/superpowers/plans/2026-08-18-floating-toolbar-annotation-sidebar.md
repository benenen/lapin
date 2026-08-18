# 浮动工具栏 / 可隐藏标注侧边栏 / 正文内标注标记 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把白板与标注的入口合并成一条随上下文变形的浮动工具栏，把标注与讨论收进可收起的右侧栏，并让正文中被标注的文字显示可点击的标记。

**Architecture:** 新增三个前端单元——`ChapterToolbar.vue`（浮动栏，纯展示 + 事件）、`AnnotationSidebar.vue`（右侧栏，标注/讨论两 tab）、`annotationMarks.ts`（把标注字符偏移映射成 ProseMirror 装饰的纯函数 + 插件）。`RichTextContent.vue` 接收 `annotations` 并渲染标记，`ExcalidrawWhiteboard.vue` 透传该 prop 并暴露白板动作，`DashboardView.vue` 只负责状态编排。不改服务端、不改路由、不改标注存储格式。

**Tech Stack:** Vue 3 `<script setup>` + TypeScript、PrimeVue 4、Tiptap 3 / ProseMirror、Vitest + @vue/test-utils。

## Global Constraints

- 设计依据：`docs/superpowers/specs/2026-08-18-floating-toolbar-annotation-sidebar-design.md`。
- 服务端 `CreateAnnotation` 要求 `note` 非空，创建标注必须经过富文本输入；本计划不新增、不修改任何 Go 代码与 HTTP 路由。
- 标注偏移的口径：以 `editor.state.doc.textBetween(from, to, '', leafText)` 的 JS 字符串长度计，`inlineMath` / `blockMath` 取 `attrs.latex`，与 `RichTextContent.vue` 现有的 `emitSelection` 完全一致。
- 颜色只有四种：`yellow`、`green`、`blue`、`pink`；非法颜色一律当 `yellow`。
- 所有面向用户的文案用中文。
- 每个任务必须先写失败的测试再写实现。
- 前端验收命令：`cd web && npm test`、`npm run typecheck`、`npm run build`。
- 不要在本计划中运行 Go 测试：`go test` 会清空 `lapin_test`，而 `make watch` 的开发库是同一个，跑完需要重建管理员并重新导入课程。全量验收留到最后一个任务，由人确认后再跑。

## File Structure

| 文件 | 职责 |
| --- | --- |
| `web/src/annotationMarks.ts`（新） | 纯函数 `annotationDecorationRanges` + ProseMirror 插件 `annotationDecorationPlugin`，把标注偏移映射成装饰 |
| `web/src/annotationMarks.spec.ts`（新） | 上述纯函数的单元测试 |
| `web/src/components/ChapterToolbar.vue`（新） | 浮动药丸工具栏，三态展示，只发事件不持状态 |
| `web/src/components/ChapterToolbar.spec.ts`（新） | 工具栏三态与事件测试 |
| `web/src/components/AnnotationSidebar.vue`（新） | 右侧可收起栏，标注 / 讨论两 tab，含新建标注区 |
| `web/src/components/AnnotationSidebar.spec.ts`（新） | 侧边栏收展、tab、预填、保存测试 |
| `web/src/components/RichTextContent.vue`（改） | 新增 `annotations` prop 与 `annotation-click` 事件 |
| `web/src/components/RichTextContent.spec.ts`（改） | 标记渲染与点击派发测试 |
| `web/src/components/ExcalidrawWhiteboard.vue`（改） | 透传 `annotations` / `annotation-click`，`defineExpose` 白板动作 |
| `web/src/excalidrawBridge.ts`（改） | `ToolbarExtension` 去掉撤销/重做/清空/保存，只留拖动把手 |
| `web/src/components/DashboardView.vue`（改） | 状态编排：删顶部 tab 与固定标注栏，接入工具栏与侧边栏 |
| `web/src/components/DashboardView.spec.ts`（改） | 更新受影响用例 + 两条串联用例 |
| `web/src/styles.css`（改） | 浮动栏、侧边栏、标注标记样式；`.notes-grid` 改单列 |

任务顺序：先做无依赖的纯逻辑（Task 1），再做两个独立展示组件（Task 2、3），然后是正文标记（Task 4），最后才是把它们接起来的编排层（Task 5、6）。

---

### Task 1: 标注偏移 → ProseMirror 装饰的映射

**Files:**
- Create: `web/src/annotationMarks.ts`
- Test: `web/src/annotationMarks.spec.ts`

**Interfaces:**
- Consumes: 无
- Produces:
  - `export interface AnnotationRange { id: string; from: number; to: number; color: string }`
  - `export function annotationDecorationRanges(doc: ProseMirrorNode, annotations: readonly AnnotationOffsets[]): AnnotationRange[]`
  - `export interface AnnotationOffsets { id: string; start_offset: number; end_offset: number; color: string }`
  - `export const annotationDecorationPluginKey: PluginKey`
  - `export function annotationDecorationPlugin(getAnnotations: () => readonly AnnotationOffsets[]): Plugin`

- [ ] **Step 1: 写失败的测试**

创建 `web/src/annotationMarks.spec.ts`：

```typescript
import { describe, expect, it } from 'vitest'
import { Editor } from '@tiptap/core'

import { createEditorExtensions } from './editor'
import { annotationDecorationRanges } from './annotationMarks'

function docOf(markdown: string) {
  const editor = new Editor({
    content: markdown,
    contentType: 'markdown',
    editable: false,
    extensions: createEditorExtensions(),
  })
  const doc = editor.state.doc
  editor.destroy()
  return doc
}

function annotation(id: string, start: number, end: number, color = 'yellow') {
  return { id, start_offset: start, end_offset: end, color }
}

describe('annotation decoration ranges', () => {
  it('maps rendered text offsets onto document positions', () => {
    const doc = docOf('上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5)])

    expect(ranges).toHaveLength(1)
    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('上下文工程')
    expect(ranges[0]!.color).toBe('yellow')
  })

  it('keeps offsets aligned across block boundaries', () => {
    const doc = docOf('第一段。\n\n第二段文字。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 4, 7)])

    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('第二段')
  })

  it('counts math nodes by their latex, matching the selection contract', () => {
    const doc = docOf('设 $a+b$ 成立。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 1)])

    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('设')
  })

  it('skips annotations that no longer fit the chapter text', () => {
    const doc = docOf('很短。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 999)])).toEqual([])
    expect(annotationDecorationRanges(doc, [annotation('a', 900, 950)])).toEqual([])
  })

  it('skips empty and reversed ranges', () => {
    const doc = docOf('上下文工程是核心。')

    expect(annotationDecorationRanges(doc, [annotation('a', 3, 3)])).toEqual([])
    expect(annotationDecorationRanges(doc, [annotation('a', 5, 2)])).toEqual([])
  })

  it('keeps overlapping annotations as separate ranges', () => {
    const doc = docOf('上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5), annotation('b', 3, 8, 'blue')])

    expect(ranges.map((range) => range.id)).toEqual(['a', 'b'])
    expect(ranges[1]!.color).toBe('blue')
  })

  it('falls back to yellow for an unknown colour', () => {
    const doc = docOf('上下文工程是核心。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 3, 'orange')])[0]!.color).toBe('yellow')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/annotationMarks.spec.ts`
Expected: FAIL，报 `Failed to resolve import "./annotationMarks"`。

- [ ] **Step 3: 写最小实现**

创建 `web/src/annotationMarks.ts`：

```typescript
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export interface AnnotationOffsets {
  id: string
  start_offset: number
  end_offset: number
  color: string
}

export interface AnnotationRange {
  id: string
  from: number
  to: number
  color: string
}

const ANNOTATION_COLORS = new Set(['yellow', 'green', 'blue', 'pink'])

export const annotationDecorationPluginKey = new PluginKey('lapinAnnotationDecorations')

// The selection contract in RichTextContent measures offsets with textBetween(…, '', leafText),
// so the reverse mapping has to walk the document with exactly the same accounting.
function leafText(node: ProseMirrorNode): string {
  if (node.type.name === 'inlineMath' || node.type.name === 'blockMath') {
    return String(node.attrs.latex ?? '')
  }
  return ''
}

interface OffsetMarker {
  offset: number
  position: number
}

function offsetMarkers(doc: ProseMirrorNode): { markers: OffsetMarker[]; total: number } {
  const markers: OffsetMarker[] = []
  let offset = 0
  doc.descendants((node, position) => {
    if (node.isText) {
      markers.push({ offset, position })
      offset += node.text?.length ?? 0
      return false
    }
    const leaf = leafText(node)
    if (leaf) {
      markers.push({ offset, position })
      offset += leaf.length
      return false
    }
    return true
  })
  return { markers, total: offset }
}

function positionAt(markers: OffsetMarker[], doc: ProseMirrorNode, offset: number): number | null {
  for (let index = markers.length - 1; index >= 0; index--) {
    const marker = markers[index]!
    if (marker.offset > offset) continue
    const node = doc.nodeAt(marker.position)
    if (!node) return null
    const length = node.isText ? node.text?.length ?? 0 : leafText(node).length
    const inner = offset - marker.offset
    if (inner > length) return null
    return marker.position + (node.isText ? inner : inner > 0 ? node.nodeSize : 0)
  }
  return null
}

export function annotationDecorationRanges(doc: ProseMirrorNode, annotations: readonly AnnotationOffsets[]): AnnotationRange[] {
  const { markers, total } = offsetMarkers(doc)
  const ranges: AnnotationRange[] = []
  for (const annotation of annotations) {
    const { start_offset: start, end_offset: end } = annotation
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start || start < 0 || end > total) continue
    const from = positionAt(markers, doc, start)
    const to = positionAt(markers, doc, end)
    if (from === null || to === null || to <= from) continue
    ranges.push({
      id: annotation.id,
      from,
      to,
      color: ANNOTATION_COLORS.has(annotation.color) ? annotation.color : 'yellow',
    })
  }
  return ranges
}

export function annotationDecorationPlugin(getAnnotations: () => readonly AnnotationOffsets[]): Plugin {
  const build = (doc: ProseMirrorNode) => DecorationSet.create(doc, annotationDecorationRanges(doc, getAnnotations()).map((range) => (
    Decoration.inline(range.from, range.to, {
      class: `annotation-mark annotation-mark-${range.color}`,
      'data-annotation-id': range.id,
    })
  )))
  return new Plugin({
    key: annotationDecorationPluginKey,
    state: {
      init: (_config, state) => build(state.doc),
      apply: (transaction, value, _oldState, newState) => (
        transaction.docChanged || transaction.getMeta(annotationDecorationPluginKey) ? build(newState.doc) : value
      ),
    },
    props: {
      decorations(state) {
        return this.getState(state)
      },
    },
  })
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run src/annotationMarks.spec.ts`
Expected: PASS，7 个用例全绿。

- [ ] **Step 5: 类型检查并提交**

```bash
cd web && npm run typecheck
git add web/src/annotationMarks.ts web/src/annotationMarks.spec.ts
git commit -m "feat: map annotation offsets onto ProseMirror decorations"
```

---

### Task 2: 浮动工具栏组件

**Files:**
- Create: `web/src/components/ChapterToolbar.vue`
- Test: `web/src/components/ChapterToolbar.spec.ts`
- Modify: `web/src/styles.css`（在 `.chapter-document-actions` 那一行之后追加浮动栏样式）

**Interfaces:**
- Consumes: 无
- Produces：组件 props
  `{ mode: 'reading' | 'selecting' | 'whiteboard'; annotationCount: number; commentCount: number; quote: string; color: string; whiteboardDisabled: boolean; whiteboardLoading: boolean; whiteboardError: boolean; saving: boolean }`
  与事件
  `toggle-whiteboard`、`retry-whiteboard`、`open-sidebar: ['annotations' | 'comments']`、`pick-color: [string]`、`compose-annotation`、`cancel-selection`、`undo`、`redo`、`clear`、`save-whiteboard`

- [ ] **Step 1: 写失败的测试**

创建 `web/src/components/ChapterToolbar.spec.ts`：

```typescript
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ChapterToolbar from './ChapterToolbar.vue'

function mountToolbar(props: Record<string, unknown> = {}) {
  return mount(ChapterToolbar, {
    props: {
      mode: 'reading',
      annotationCount: 2,
      commentCount: 3,
      quote: '',
      color: 'yellow',
      whiteboardDisabled: false,
      whiteboardLoading: false,
      whiteboardError: false,
      saving: false,
      ...props,
    },
  })
}

describe('ChapterToolbar', () => {
  it('shows the reading actions with their counts', () => {
    const wrapper = mountToolbar()

    const labels = wrapper.findAll('button').map((button) => button.text())
    expect(labels).toEqual(['白板', '标注 2', '讨论 3'])
  })

  it('opens the sidebar on the matching tab', async () => {
    const wrapper = mountToolbar()

    await wrapper.findAll('button')[1]!.trigger('click')
    await wrapper.findAll('button')[2]!.trigger('click')

    expect(wrapper.emitted('open-sidebar')).toEqual([['annotations'], ['comments']])
  })

  it('morphs into the selection actions and keeps the quote visible', async () => {
    const wrapper = mountToolbar({ mode: 'selecting', quote: '上下文工程是核心' })

    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文工程是核心')
    await wrapper.get('button[data-color="blue"]').trigger('click')
    await wrapper.get('button[data-action="compose"]').trigger('click')
    await wrapper.get('button[data-action="cancel"]').trigger('click')

    expect(wrapper.emitted('pick-color')).toEqual([['blue']])
    expect(wrapper.emitted('compose-annotation')).toHaveLength(1)
    expect(wrapper.emitted('cancel-selection')).toHaveLength(1)
  })

  it('marks the chosen colour as active', () => {
    const wrapper = mountToolbar({ mode: 'selecting', quote: '正文', color: 'pink' })

    expect(wrapper.get('button[data-color="pink"]').classes()).toContain('active')
    expect(wrapper.get('button[data-color="yellow"]').classes()).not.toContain('active')
  })

  it('exposes the whiteboard actions only while the whiteboard is open', async () => {
    const reading = mountToolbar()
    expect(reading.find('button[data-action="undo"]').exists()).toBe(false)

    const wrapper = mountToolbar({ mode: 'whiteboard' })
    await wrapper.get('button[data-action="undo"]').trigger('click')
    await wrapper.get('button[data-action="redo"]').trigger('click')
    await wrapper.get('button[data-action="clear"]').trigger('click')
    await wrapper.get('button[data-action="save-whiteboard"]').trigger('click')

    expect(wrapper.emitted('undo')).toHaveLength(1)
    expect(wrapper.emitted('redo')).toHaveLength(1)
    expect(wrapper.emitted('clear')).toHaveLength(1)
    expect(wrapper.emitted('save-whiteboard')).toHaveLength(1)
  })

  it('disables saving the whiteboard while a save is in flight', () => {
    const wrapper = mountToolbar({ mode: 'whiteboard', saving: true })

    expect(wrapper.get('button[data-action="save-whiteboard"]').attributes('disabled')).toBeDefined()
  })

  it('blocks the whiteboard until persisted data has loaded', () => {
    const wrapper = mountToolbar({ whiteboardDisabled: true })

    expect(wrapper.get('button[data-action="whiteboard"]').attributes('disabled')).toBeDefined()
  })

  it('offers a retry instead of a toggle when the whiteboard failed to load', async () => {
    const wrapper = mountToolbar({ whiteboardError: true })

    const retry = wrapper.get('button[data-action="retry-whiteboard"]')
    expect(retry.text()).toBe('重试白板')
    await retry.trigger('click')

    expect(wrapper.emitted('retry-whiteboard')).toHaveLength(1)
    expect(wrapper.find('button[data-action="whiteboard"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/components/ChapterToolbar.spec.ts`
Expected: FAIL，报无法解析 `./ChapterToolbar.vue`。

- [ ] **Step 3: 写最小实现**

创建 `web/src/components/ChapterToolbar.vue`：

```vue
<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  mode: 'reading' | 'selecting' | 'whiteboard'
  annotationCount: number
  commentCount: number
  quote: string
  color: string
  whiteboardDisabled: boolean
  whiteboardLoading: boolean
  whiteboardError: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  'toggle-whiteboard': []
  'retry-whiteboard': []
  'open-sidebar': [tab: 'annotations' | 'comments']
  'pick-color': [color: string]
  'compose-annotation': []
  'cancel-selection': []
  undo: []
  redo: []
  clear: []
  'save-whiteboard': []
}>()

const colors = ['yellow', 'green', 'blue', 'pink']
const selecting = computed(() => props.mode === 'selecting')
const whiteboardOpen = computed(() => props.mode === 'whiteboard')
</script>

<template>
  <div class="chapter-toolbar" :class="`is-${props.mode}`" role="toolbar" aria-label="章节操作">
    <template v-if="selecting">
      <span class="chapter-toolbar-quote">“{{ props.quote }}”</span>
      <div class="chapter-toolbar-colors">
        <button
          v-for="item in colors"
          :key="item"
          type="button"
          :data-color="item"
          :class="[item, { active: props.color === item }]"
          :aria-label="`标注颜色 ${item}`"
          :aria-pressed="props.color === item"
          @click="emit('pick-color', item)"
        />
      </div>
      <button type="button" data-action="compose" class="chapter-toolbar-primary" @click="emit('compose-annotation')">
        <i class="pi pi-pencil" /> 写标注
      </button>
      <button type="button" data-action="cancel" aria-label="取消选区" @click="emit('cancel-selection')">
        <i class="pi pi-times" />
      </button>
    </template>
    <template v-else>
      <button
        v-if="props.whiteboardError"
        type="button"
        data-action="retry-whiteboard"
        @click="emit('retry-whiteboard')"
      >
        <i class="pi pi-refresh" /> 重试白板
      </button>
      <button
        v-else
        type="button"
        data-action="whiteboard"
        :class="{ active: whiteboardOpen }"
        :disabled="props.whiteboardDisabled || props.whiteboardLoading"
        :aria-pressed="whiteboardOpen"
        @click="emit('toggle-whiteboard')"
      >
        <i class="pi" :class="whiteboardOpen ? 'pi-eye-slash' : 'pi-eye'" /> 白板
      </button>
      <template v-if="whiteboardOpen">
        <span class="chapter-toolbar-divider" aria-hidden="true" />
        <button type="button" data-action="undo" aria-label="撤销" @click="emit('undo')"><i class="pi pi-undo" /></button>
        <button type="button" data-action="redo" aria-label="重做" @click="emit('redo')"><i class="pi pi-refresh" /></button>
        <button type="button" data-action="clear" aria-label="清空白板" @click="emit('clear')"><i class="pi pi-trash" /></button>
        <button type="button" data-action="save-whiteboard" aria-label="保存白板" :disabled="props.saving" @click="emit('save-whiteboard')"><i class="pi pi-check" /></button>
        <span class="chapter-toolbar-divider" aria-hidden="true" />
      </template>
      <button type="button" data-action="annotations" @click="emit('open-sidebar', 'annotations')">
        <i class="pi pi-pencil" /> 标注 {{ props.annotationCount }}
      </button>
      <button v-if="!whiteboardOpen" type="button" data-action="comments" @click="emit('open-sidebar', 'comments')">
        <i class="pi pi-comments" /> 讨论 {{ props.commentCount }}
      </button>
    </template>
  </div>
</template>
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run src/components/ChapterToolbar.spec.ts`
Expected: PASS，8 个用例全绿。

- [ ] **Step 5: 加样式**

在 `web/src/styles.css` 的 `.chapter-document-actions` 那行之后插入（该规则本身会在 Task 6 随模板一起删除，此处不要动它）：

```css
.chapter-toolbar { position: sticky; bottom: 1.5rem; z-index: 30; display: flex; align-items: center; justify-content: center; gap: .35rem; width: max-content; max-width: 100%; margin: 1.5rem auto 0; padding: .35rem .5rem; border: 1px solid #dcddd4; border-radius: 999px; background: rgba(251, 250, 246, .86); box-shadow: 0 6px 20px rgba(51, 72, 62, .14); backdrop-filter: blur(12px); transition: width .2s ease; }
.chapter-toolbar button { display: inline-flex; align-items: center; gap: .35rem; padding: .45rem .8rem; border: 0; border-radius: 999px; color: #33483e; background: transparent; cursor: pointer; transition: background .15s ease; }
.chapter-toolbar button:hover:not(:disabled) { background: #eceade; }
.chapter-toolbar button.active { color: #fbfaf6; background: #33483e; }
.chapter-toolbar button:disabled { opacity: .45; cursor: not-allowed; }
.chapter-toolbar-divider { width: 1px; height: 1.4rem; background: #dcddd4; }
.chapter-toolbar-quote { max-width: 16rem; overflow: hidden; padding-left: .5rem; color: #6d664f; white-space: nowrap; text-overflow: ellipsis; }
.chapter-toolbar-colors { display: flex; gap: .35rem; padding: 0 .3rem; }
.chapter-toolbar-colors button { width: 1.25rem; height: 1.25rem; padding: 0; border: 2px solid transparent; border-radius: 50%; }
.chapter-toolbar-colors button.active { border-color: #33483e; }
.chapter-toolbar-colors .yellow { background: #f0d67b; }
.chapter-toolbar-colors .green { background: #9fcfa8; }
.chapter-toolbar-colors .blue { background: #9ebedb; }
.chapter-toolbar-colors .pink { background: #e7aeb6; }
```

- [ ] **Step 6: 类型检查并提交**

```bash
cd web && npm run typecheck && npx vitest run src/components/ChapterToolbar.spec.ts
git add web/src/components/ChapterToolbar.vue web/src/components/ChapterToolbar.spec.ts web/src/styles.css
git commit -m "feat: add the floating chapter toolbar"
```

---

### Task 3: 可收起的标注 / 讨论侧边栏

**Files:**
- Create: `web/src/components/AnnotationSidebar.vue`
- Test: `web/src/components/AnnotationSidebar.spec.ts`
- Modify: `web/src/styles.css`（在 `.annotation-card p` 那行之后追加侧边栏样式）

**Interfaces:**
- Consumes: 无（`RichTextEditor.vue` / `RichTextContent.vue` 已存在）
- Produces：组件 props
  `{ open: boolean; tab: 'annotations' | 'comments'; annotations: Annotation[]; comments: Comment[]; draft: { quote: string; note: string; color: string }; activeAnnotationId: string; commentBody: string; userName: string }`
  与事件
  `update:open: [boolean]`、`update:tab: ['annotations' | 'comments']`、`update:draft: [draft]`、`update:commentBody: [string]`、`save-annotation`、`post-comment`

- [ ] **Step 1: 写失败的测试**

创建 `web/src/components/AnnotationSidebar.spec.ts`：

```typescript
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AnnotationSidebar from './AnnotationSidebar.vue'
import type { Annotation, Comment } from '../types'

const annotation = (id: string, note: string): Annotation => ({
  id,
  chapter_id: 'chapter-a',
  user_id: 'user-a',
  author_name: '学习者',
  start_offset: 0,
  end_offset: 4,
  quote: '上下文',
  note,
  color: 'yellow',
  created_at: '2026-08-18T00:00:00Z',
})

const comment = (id: string, body: string): Comment => ({
  id,
  chapter_id: 'chapter-a',
  user_id: 'user-a',
  author_name: '学习者',
  body,
  created_at: '2026-08-18T00:00:00Z',
})

function mountSidebar(props: Record<string, unknown> = {}) {
  return mount(AnnotationSidebar, {
    props: {
      open: true,
      tab: 'annotations',
      annotations: [annotation('note-1', '第一条标注')],
      comments: [comment('comment-1', '第一条讨论')],
      draft: { quote: '', note: '', color: 'yellow' },
      activeAnnotationId: '',
      commentBody: '',
      userName: '学习者',
      ...props,
    },
    global: {
      stubs: {
        Avatar: { template: '<span />' },
        Button: {
          props: ['label', 'disabled'],
          template: `<button :disabled="disabled" @click="$emit('click')">{{ label }}</button>`,
        },
        RichTextEditor: {
          props: ['modelValue'],
          template: `<textarea class="rich-editor" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
        },
        RichTextContent: { props: ['content'], template: '<div class="rich-content">{{ content }}</div>' },
      },
    },
  })
}

describe('AnnotationSidebar', () => {
  it('toggles with the handle on its left edge', async () => {
    const wrapper = mountSidebar()

    const handle = wrapper.get('.annotation-sidebar-handle')
    expect(handle.attributes('aria-expanded')).toBe('true')
    await handle.trigger('click')

    expect(wrapper.emitted('update:open')).toEqual([[false]])
  })

  it('reports being collapsed to assistive technology', () => {
    const wrapper = mountSidebar({ open: false })

    expect(wrapper.get('.annotation-sidebar').classes()).toContain('is-collapsed')
    expect(wrapper.get('.annotation-sidebar-handle').attributes('aria-expanded')).toBe('false')
  })

  it('switches between the annotation and discussion tabs', async () => {
    const wrapper = mountSidebar()

    await wrapper.get('button[data-tab="comments"]').trigger('click')

    expect(wrapper.emitted('update:tab')).toEqual([['comments']])
  })

  it('shows the captured quote and colour in the composer', () => {
    const wrapper = mountSidebar({ draft: { quote: '上下文工程', note: '', color: 'blue' } })

    expect(wrapper.get('.annotation-composer blockquote').text()).toContain('上下文工程')
    expect(wrapper.get('.annotation-composer button[data-color="blue"]').classes()).toContain('active')
  })

  it('keeps saving disabled until the note has content', async () => {
    const wrapper = mountSidebar()
    const save = wrapper.findAll('button').find((button) => button.text() === '保存标注')!
    expect(save.attributes('disabled')).toBeDefined()

    await wrapper.setProps({ draft: { quote: '上下文', note: '想法', color: 'yellow' } })
    const enabled = wrapper.findAll('button').find((button) => button.text() === '保存标注')!
    await enabled.trigger('click')

    expect(wrapper.emitted('save-annotation')).toHaveLength(1)
  })

  it('publishes note edits through update:draft', async () => {
    const wrapper = mountSidebar({ draft: { quote: '上下文', note: '', color: 'yellow' } })

    await wrapper.get('.annotation-composer .rich-editor').setValue('新的想法')

    expect(wrapper.emitted('update:draft')).toEqual([[{ quote: '上下文', note: '新的想法', color: 'yellow' }]])
  })

  it('highlights the annotation opened from the chapter text', () => {
    const wrapper = mountSidebar({ activeAnnotationId: 'note-1' })

    expect(wrapper.get('.annotation-card').classes()).toContain('is-active')
  })

  it('renders the discussion tab with its composer and list', async () => {
    const wrapper = mountSidebar({ tab: 'comments' })

    expect(wrapper.text()).toContain('第一条讨论')
    expect(wrapper.find('.annotation-composer').exists()).toBe(false)
    await wrapper.get('.comment-compose .rich-editor').setValue('新讨论')

    expect(wrapper.emitted('update:commentBody')).toEqual([['新讨论']])
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/components/AnnotationSidebar.spec.ts`
Expected: FAIL，报无法解析 `./AnnotationSidebar.vue`。

- [ ] **Step 3: 写最小实现**

创建 `web/src/components/AnnotationSidebar.vue`：

```vue
<script setup lang="ts">
import { computed, nextTick, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'

import type { Annotation, Comment } from '../types'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

interface AnnotationDraft {
  quote: string
  note: string
  color: string
}

const props = defineProps<{
  open: boolean
  tab: 'annotations' | 'comments'
  annotations: Annotation[]
  comments: Comment[]
  draft: AnnotationDraft
  activeAnnotationId: string
  commentBody: string
  userName: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'update:tab': [tab: 'annotations' | 'comments']
  'update:draft': [draft: AnnotationDraft]
  'update:commentBody': [body: string]
  'save-annotation': []
  'post-comment': []
}>()

const colors = ['yellow', 'green', 'blue', 'pink']
const canSave = computed(() => props.draft.note.trim().length > 0)

watch(() => props.activeAnnotationId, async (id) => {
  if (!id) return
  await nextTick()
  document.querySelector(`[data-annotation-card="${id}"]`)?.scrollIntoView({ block: 'nearest' })
})

function updateDraft(patch: Partial<AnnotationDraft>) {
  emit('update:draft', { ...props.draft, ...patch })
}
</script>

<template>
  <aside class="annotation-sidebar" :class="{ 'is-collapsed': !props.open }">
    <button
      type="button"
      class="annotation-sidebar-handle"
      :aria-expanded="props.open"
      :title="props.open ? '收起标注栏' : '展开标注栏'"
      :aria-label="props.open ? '收起标注栏' : '展开标注栏'"
      @click="emit('update:open', !props.open)"
    >
      <i class="pi" :class="props.open ? 'pi-chevron-right' : 'pi-chevron-left'" />
    </button>

    <div class="annotation-sidebar-body">
      <div class="annotation-sidebar-tabs" role="tablist">
        <button type="button" data-tab="annotations" :class="{ active: props.tab === 'annotations' }" @click="emit('update:tab', 'annotations')">
          标注 {{ props.annotations.length }}
        </button>
        <button type="button" data-tab="comments" :class="{ active: props.tab === 'comments' }" @click="emit('update:tab', 'comments')">
          讨论 {{ props.comments.length }}
        </button>
      </div>

      <section v-if="props.tab === 'annotations'" class="annotation-sidebar-panel">
        <div class="annotation-composer">
          <h3>新建标注</h3>
          <blockquote v-if="props.draft.quote">“{{ props.draft.quote }}”</blockquote>
          <p v-else class="annotation-empty-quote">先在正文里选中一段文字。</p>
          <RichTextEditor
            class="compact-rich-text-editor"
            :model-value="props.draft.note"
            @update:model-value="updateDraft({ note: $event })"
          />
          <div class="annotation-actions">
            <div class="annotation-colors">
              <button
                v-for="color in colors"
                :key="color"
                type="button"
                :data-color="color"
                :class="[color, { active: props.draft.color === color }]"
                :aria-label="`标注颜色 ${color}`"
                @click="updateDraft({ color })"
              />
            </div>
            <Button label="保存标注" size="small" :disabled="!canSave" @click="emit('save-annotation')" />
          </div>
        </div>
        <div class="annotation-list">
          <div
            v-for="item in props.annotations"
            :key="item.id"
            :data-annotation-card="item.id"
            class="annotation-card"
            :class="[item.color, { 'is-active': item.id === props.activeAnnotationId }]"
          >
            <small>{{ item.author_name }} · {{ new Date(item.created_at).toLocaleString() }}</small>
            <q v-if="item.quote">{{ item.quote }}</q>
            <RichTextContent :content="item.note" />
          </div>
        </div>
      </section>

      <section v-else class="annotation-sidebar-panel comments-panel">
        <div class="comment-compose">
          <Avatar :label="props.userName.slice(0, 1)" shape="circle" />
          <RichTextEditor
            class="compact-rich-text-editor"
            :model-value="props.commentBody"
            @update:model-value="emit('update:commentBody', $event)"
          />
          <Button label="发送" icon="pi pi-send" :disabled="!props.commentBody.trim()" @click="emit('post-comment')" />
        </div>
        <div v-if="props.comments.length === 0" class="empty-comments">还没有讨论，来提出第一个问题吧。</div>
        <div v-for="item in props.comments" :key="item.id" class="comment-item">
          <Avatar :label="item.author_name.slice(0, 1)" shape="circle" />
          <div>
            <strong>{{ item.author_name }}</strong>
            <small>{{ new Date(item.created_at).toLocaleString() }}</small>
            <RichTextContent :content="item.body" />
          </div>
        </div>
      </section>
    </div>
  </aside>
</template>
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run src/components/AnnotationSidebar.spec.ts`
Expected: PASS，8 个用例全绿。

- [ ] **Step 5: 加样式**

在 `web/src/styles.css` 的 `.annotation-card p` 那行之后插入：

```css
.annotation-sidebar { position: sticky; top: 1rem; align-self: start; width: 20rem; max-height: calc(100vh - 2rem); transition: transform .22s ease, width .22s ease; }
.annotation-sidebar.is-collapsed { width: 0; transform: translateX(100%); }
.annotation-sidebar-handle { position: absolute; top: 50%; left: -1.35rem; z-index: 2; display: flex; align-items: center; justify-content: center; width: 1.35rem; height: 3.2rem; padding: 0; border: 1px solid #dcddd4; border-right: 0; border-radius: 8px 0 0 8px; color: #33483e; background: #fbfaf6; transform: translateY(-50%); cursor: pointer; }
.annotation-sidebar-body { display: flex; flex-direction: column; overflow: hidden; max-height: inherit; border: 1px solid #dcddd4; border-radius: 12px; background: #fbfaf6; }
.annotation-sidebar.is-collapsed .annotation-sidebar-body { visibility: hidden; }
.annotation-sidebar-tabs { display: flex; border-bottom: 1px solid #dcddd4; }
.annotation-sidebar-tabs button { flex: 1; padding: .7rem; border: 0; color: #6d664f; background: transparent; cursor: pointer; }
.annotation-sidebar-tabs button.active { color: #33483e; box-shadow: inset 0 -2px 0 #33483e; }
.annotation-sidebar-panel { overflow: auto; padding: 1rem; }
.annotation-composer h3 { margin-top: 0; font-family: Georgia, serif; }
.annotation-composer blockquote { margin: .7rem 0; padding: .7rem; border-left: 3px solid #d5ad52; color: #6d664f; background: #f8f1d9; }
.annotation-empty-quote { color: #899087; }
.annotation-card.is-active { box-shadow: 0 0 0 2px #33483e; }
@media (max-width: 900px) {
  .annotation-sidebar { position: fixed; top: 0; right: 0; z-index: 40; max-height: 100vh; }
}
```

- [ ] **Step 6: 类型检查并提交**

```bash
cd web && npm run typecheck && npx vitest run src/components/AnnotationSidebar.spec.ts
git add web/src/components/AnnotationSidebar.vue web/src/components/AnnotationSidebar.spec.ts web/src/styles.css
git commit -m "feat: add the collapsible annotation sidebar"
```

---

### Task 4: 正文中的标注标记

**Files:**
- Modify: `web/src/components/RichTextContent.vue`
- Modify: `web/src/components/RichTextContent.spec.ts`
- Modify: `web/src/components/ExcalidrawWhiteboard.vue`（模板中的 `<RichTextContent>` 那一行）
- Modify: `web/src/styles.css`

**Interfaces:**
- Consumes: Task 1 的 `annotationDecorationPlugin(getAnnotations)`、`annotationDecorationPluginKey`、`AnnotationOffsets`
- Produces:
  - `RichTextContent` 新增可选 prop `annotations?: AnnotationOffsets[]` 与事件 `annotation-click: [id: string]`
  - `ExcalidrawWhiteboard` 新增可选 prop `annotations?: AnnotationOffsets[]` 与事件 `annotation-click: [id: string]`

- [ ] **Step 1: 写失败的测试**

在 `web/src/components/RichTextContent.spec.ts` 的 `describe` 块内追加：

```typescript
  it('marks annotated text and reports which annotation was clicked', async () => {
    const wrapper = mount(RichTextContent, {
      props: {
        content: '上下文工程是核心。',
        annotations: [{ id: 'note-1', start_offset: 0, end_offset: 5, color: 'blue' }],
      },
      attachTo: document.body,
    })
    await flushPromises()

    const mark = wrapper.get('[data-annotation-id="note-1"]')
    expect(mark.classes()).toContain('annotation-mark-blue')
    expect(mark.text()).toBe('上下文工程')

    await mark.trigger('click')
    expect(wrapper.emitted('annotation-click')).toEqual([['note-1']])
    wrapper.unmount()
  })

  it('refreshes the marks when annotations change', async () => {
    const wrapper = mount(RichTextContent, {
      props: { content: '上下文工程是核心。', annotations: [] },
      attachTo: document.body,
    })
    await flushPromises()
    expect(wrapper.find('[data-annotation-id]').exists()).toBe(false)

    await wrapper.setProps({ annotations: [{ id: 'note-1', start_offset: 0, end_offset: 3, color: 'yellow' }] })
    await flushPromises()

    expect(wrapper.get('[data-annotation-id="note-1"]').text()).toBe('上下文')
    wrapper.unmount()
  })
```

如果该文件还没有引入 `flushPromises`，把首行的 import 改成
`import { flushPromises, mount } from '@vue/test-utils'`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/components/RichTextContent.spec.ts`
Expected: FAIL，找不到 `[data-annotation-id="note-1"]`。

- [ ] **Step 3: 改 `RichTextContent.vue`**

把 `<script setup>` 中的 props / emits / editor 部分改成：

```typescript
import { watch } from 'vue'
import type { Editor } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/vue-3'

import { createEditorExtensions } from '../editor'
import { annotationDecorationPlugin, annotationDecorationPluginKey, type AnnotationOffsets } from '../annotationMarks'

const props = withDefaults(defineProps<{ content: string; annotations?: AnnotationOffsets[] }>(), {
  annotations: () => [],
})
const emit = defineEmits<{
  selection: [selection: { start_offset: number; end_offset: number; quote: string }]
  'annotation-click': [id: string]
}>()

const editor = useEditor({
  content: props.content,
  contentType: 'markdown',
  editable: false,
  extensions: createEditorExtensions(),
  onSelectionUpdate: ({ editor: current }) => emitSelection(current),
  onCreate: ({ editor: current }) => {
    current.registerPlugin(annotationDecorationPlugin(() => props.annotations))
  },
})

watch(
  () => props.annotations,
  () => {
    const current = editor.value
    if (!current) return
    current.view.dispatch(current.state.tr.setMeta(annotationDecorationPluginKey, true))
  },
  { deep: true },
)

function handleClick(event: MouseEvent) {
  const target = (event.target as HTMLElement | null)?.closest('[data-annotation-id]')
  const id = target?.getAttribute('data-annotation-id')
  if (id) emit('annotation-click', id)
}
```

保留原有的 `watch(() => props.content, …)` 与 `emitSelection` 不动，模板改成：

```vue
<template>
  <EditorContent class="chapter-content autosize-rich-text" :editor="editor" @click="handleClick" />
</template>
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run src/components/RichTextContent.spec.ts`
Expected: PASS，含新增的 2 个用例。

- [ ] **Step 5: 让白板层透传标注**

`web/src/components/ExcalidrawWhiteboard.vue` 的 props 加一项、emits 加一项：

```typescript
const props = defineProps<{
  chapterId: string
  content: string
  active: boolean
  annotations?: AnnotationOffsets[]
  modelValue?: PersistedWhiteboardData | null
  saving?: boolean
}>()

const emit = defineEmits<{
  save: [data: WhiteboardData]
  selection: [selection: { start_offset: number; end_offset: number; quote: string }]
  'annotation-click': [id: string]
}>()
```

顶部补 `import type { AnnotationOffsets } from '../annotationMarks'`，模板里那一行改成：

```vue
          <RichTextContent
            v-if="content"
            :content="content"
            :annotations="props.annotations ?? []"
            @selection="emit('selection', $event)"
            @annotation-click="emit('annotation-click', $event)"
          />
```

同时 `defineExpose` 白板动作，供 Task 5 的浮动栏调用（放在 `save` 函数之后）：

```typescript
defineExpose({
  undo: () => bridge?.undo(),
  redo: () => bridge?.redo(),
  clear: () => bridge?.clear(),
  save,
})
```

- [ ] **Step 6: 加标记样式**

在 `web/src/styles.css` 的 `.annotation-card.is-active` 那行之后插入：

```css
.annotation-mark { border-radius: 3px; box-shadow: inset 0 -.5em 0 rgba(240, 214, 123, .55); cursor: pointer; }
.annotation-mark-yellow { box-shadow: inset 0 -.5em 0 rgba(240, 214, 123, .55); }
.annotation-mark-green { box-shadow: inset 0 -.5em 0 rgba(159, 207, 168, .55); }
.annotation-mark-blue { box-shadow: inset 0 -.5em 0 rgba(158, 190, 219, .55); }
.annotation-mark-pink { box-shadow: inset 0 -.5em 0 rgba(231, 174, 182, .55); }
.annotation-mark:hover { box-shadow: inset 0 -1.1em 0 rgba(213, 173, 82, .35); }
```

- [ ] **Step 7: 全量前端测试并提交**

```bash
cd web && npm test && npm run typecheck
git add web/src/components/RichTextContent.vue web/src/components/RichTextContent.spec.ts web/src/components/ExcalidrawWhiteboard.vue web/src/styles.css
git commit -m "feat: mark annotated chapter text and report marker clicks"
```

---

### Task 5: 白板动作移出 Excalidraw 自带工具栏

**Files:**
- Modify: `web/src/excalidrawBridge.ts:118-137`
- Modify: `web/src/excalidrawBridge.spec.ts`

**Interfaces:**
- Consumes: Task 4 中 `ExcalidrawWhiteboard` 暴露的 `undo` / `redo` / `clear` / `save`
- Produces: `ToolbarExtension` 只再渲染拖动把手；`ExcalidrawBridge` 的 `undo` / `redo` / `clear` / `setSaving` / `getDocument` 接口保持不变

- [ ] **Step 1: 写失败的测试**

在 `web/src/excalidrawBridge.spec.ts` 中把「extends the built-in toolbar instead of rendering a second toolbar」那个用例整体替换成：

```typescript
  it('leaves only the drag handle in the built-in toolbar', () => {
    const host = document.createElement('div')
    mountExcalidraw(host, { width: 960, height: 640 })

    const excalidrawElement = reactRoot.render.mock.calls[0]?.[0]
    expect(excalidrawElement?.props.renderTopRightUI).toBeTypeOf('function')

    const extension = excalidrawElement.props.renderTopRightUI()
    expect(extension.props.host).toBe(host)
    expect(extension.props.undo).toBeUndefined()
    expect(extension.props.redo).toBeUndefined()
    expect(extension.props.clear).toBeUndefined()
    expect(extension.props.save).toBeUndefined()
  })
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/excalidrawBridge.spec.ts`
Expected: FAIL，`extension.props.undo` 仍是函数。

- [ ] **Step 3: 精简 `ToolbarExtension`**

`web/src/excalidrawBridge.ts` 中把 `ToolbarExtensionProps` 改成只剩 `host`：

```typescript
interface ToolbarExtensionProps {
  host: HTMLElement
}
```

组件签名改成 `function ToolbarExtension({ host }: ToolbarExtensionProps)`，并把 `return createPortal(...)` 那段改成只渲染把手：

```typescript
  return createPortal(createElement(Fragment, null,
    createElement('div', { key: 'divider', className: 'App-toolbar__divider' }),
    button('drag', '拖动白板工具栏', toolbarIcon('M8 6h.01', 'M8 12h.01', 'M8 18h.01', 'M16 6h.01', 'M16 12h.01', 'M16 18h.01'), () => {}, 'lapin-toolbar-drag-handle'),
  ), target)
```

`renderTopRightUI` 改成：

```typescript
  const renderTopRightUI = () => createElement(ToolbarExtension, { host: element })
```

`triggerHistoryShortcut`、`clear`、`setSaving`、`getDocument` 及导出的 `ExcalidrawBridge` 接口全部保持不动——浮动栏现在通过它们调用。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run src/excalidrawBridge.spec.ts`
Expected: PASS，5 个用例全绿。

- [ ] **Step 5: 提交**

```bash
cd web && npm run typecheck
git add web/src/excalidrawBridge.ts web/src/excalidrawBridge.spec.ts
git commit -m "refactor: move whiteboard history actions out of the excalidraw toolbar"
```

---

### Task 6: 在 DashboardView 中编排

**Files:**
- Modify: `web/src/components/DashboardView.vue`
- Modify: `web/src/components/DashboardView.spec.ts`
- Modify: `web/src/styles.css:123`（`.notes-grid`）与 `:213`（媒体查询里的 `.notes-grid`）

**Interfaces:**
- Consumes: Task 2 的 `ChapterToolbar`、Task 3 的 `AnnotationSidebar`、Task 4 的 `ExcalidrawWhiteboard` 新 prop / 事件 / `defineExpose`
- Produces: 无（终端组件）

- [ ] **Step 1: 写失败的测试**

在 `web/src/components/DashboardView.spec.ts` 中：

1. 把用例「keeps the transparent whiteboard hidden over the chapter until requested」里对 `[role="tablist"]` 的两段断言（切到讨论 tab、再切回正文 tab）删掉，把找按钮的方式改成按 `data-action`：

```typescript
    const showButton = wrapper.get('[data-action="whiteboard"]')
    await showButton.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-action="whiteboard"]').classes()).toContain('active')
```

2. 用例「offers a retry when persisted whiteboards fail to load」中的 `重试加载白板` 改成 `重试白板`，取按钮方式改成 `wrapper.get('[data-action="retry-whiteboard"]')`。

3. 用例「uses rich-text editors for annotations and discussions」改成：

```typescript
  it('uses rich-text editors for annotations and discussions', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('.annotation-composer .rich-editor').exists()).toBe(true)
    await wrapper.get('[data-tab="comments"]').trigger('click')
    expect(wrapper.find('.comment-compose .rich-editor').exists()).toBe(true)
  })
```

4. 其余用例里凡是断言 `不包含 '白板'` 的 tab 文案检查（`expect(tabLabels).not.toContain('白板')`）连同 `tabLabels` 变量一并删除。

5. 追加两条串联用例：

```typescript
  it('turns a chapter selection into a composed annotation', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('selection', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
    })
    await flushPromises()

    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文')
    await wrapper.get('.chapter-toolbar button[data-color="green"]').trigger('click')
    await wrapper.get('.chapter-toolbar button[data-action="compose"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('.annotation-composer blockquote').text()).toContain('上下文')
    expect(wrapper.get('.annotation-composer button[data-color="green"]').classes()).toContain('active')
  })

  it('opens the sidebar on the annotation clicked in the chapter text', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    await wrapper.get('.annotation-sidebar-handle').trigger('click')
    expect(wrapper.get('.annotation-sidebar').classes()).toContain('is-collapsed')

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('annotation-click', 'note-1')
    await flushPromises()

    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).toContain('is-active')
  })
```

这两条要求 `mountDashboard` 的 `api.listAnnotations` 桩返回一条 `id: 'note-1'` 的标注，并且 `ExcalidrawWhiteboard` 的桩要能转发事件；把该文件里 `ExcalidrawWhiteboard` 的 stub 换成：

```typescript
    ExcalidrawWhiteboard: {
      name: 'ExcalidrawWhiteboard',
      props: ['chapterId', 'content', 'active', 'annotations', 'modelValue', 'saving'],
      template: '<div class="whiteboard-stub" />',
    },
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run src/components/DashboardView.spec.ts`
Expected: FAIL，找不到 `.chapter-toolbar` / `.annotation-sidebar`。

- [ ] **Step 3: 改 `DashboardView.vue` 的状态与事件**

删除 `activeTab` 与 `setActiveTab`，新增：

```typescript
const sidebarOpen = ref(true)
const sidebarTab = ref<'annotations' | 'comments'>('annotations')
const activeAnnotationId = ref('')
const whiteboardRef = ref<InstanceType<typeof ExcalidrawWhiteboard> | null>(null)

const toolbarMode = computed<'reading' | 'selecting' | 'whiteboard'>(() => {
  if (annotation.value.quote) return 'selecting'
  if (whiteboardVisible.value) return 'whiteboard'
  return 'reading'
})

function openSidebar(tab: 'annotations' | 'comments') {
  sidebarTab.value = tab
  sidebarOpen.value = true
}

function composeAnnotation() {
  openSidebar('annotations')
}

function cancelSelection() {
  annotation.value = { start_offset: 0, end_offset: 0, quote: '', note: '', color: annotation.value.color }
}

function focusAnnotation(id: string) {
  activeAnnotationId.value = id
  openSidebar('annotations')
}
```

`watch(activeChapterId, …)` 的回调里追加 `activeAnnotationId.value = ''` 和 `cancelSelection()`。

`saveAnnotation` 成功分支里，把重置改为同时清掉高亮：

```typescript
    annotations.value = [created, ...annotations.value]
    annotation.value = { start_offset: 0, end_offset: 0, quote: '', note: '', color: 'yellow' }
    activeAnnotationId.value = created.id
```

- [ ] **Step 4: 改 `DashboardView.vue` 的模板**

把 `<div class="study-tabs" role="tablist">…</div>` 整块删除；把 `<section v-show="activeTab === 'notes'" class="notes-grid">` 改成 `<section class="notes-grid">`；删除其中的 `<div class="chapter-document-actions">…</div>` 与整个 `<aside class="annotation-panel">…</aside>`；删除 `<section v-show="activeTab === 'comments'" class="comments-panel">…</section>` 整块。`notes-grid` 内改成：

```vue
            <section class="notes-grid">
              <div class="chapter-document">
                <ExcalidrawWhiteboard
                  ref="whiteboardRef"
                  :chapter-id="activeChapter.id"
                  :content="activeChapter.content"
                  :active="whiteboardVisible && whiteboardsLoaded"
                  :annotations="annotations"
                  :model-value="ownWhiteboard"
                  :saving="whiteboardSaving"
                  @selection="captureSelection"
                  @annotation-click="focusAnnotation"
                  @save="saveWhiteboard"
                />
                <ChapterToolbar
                  :mode="toolbarMode"
                  :annotation-count="annotations.length"
                  :comment-count="comments.length"
                  :quote="annotation.quote"
                  :color="annotation.color"
                  :whiteboard-disabled="!whiteboardsLoaded"
                  :whiteboard-loading="whiteboardLoading"
                  :whiteboard-error="Boolean(whiteboardLoadError)"
                  :saving="whiteboardSaving"
                  @toggle-whiteboard="whiteboardVisible = !whiteboardVisible"
                  @retry-whiteboard="loadWhiteboards(activeChapter.id)"
                  @open-sidebar="openSidebar"
                  @pick-color="annotation.color = $event"
                  @compose-annotation="composeAnnotation"
                  @cancel-selection="cancelSelection"
                  @undo="whiteboardRef?.undo()"
                  @redo="whiteboardRef?.redo()"
                  @clear="whiteboardRef?.clear()"
                  @save-whiteboard="whiteboardRef?.save()"
                />
              </div>
              <AnnotationSidebar
                v-model:open="sidebarOpen"
                v-model:tab="sidebarTab"
                :annotations="annotations"
                :comments="comments"
                :draft="{ quote: annotation.quote, note: annotation.note, color: annotation.color }"
                :active-annotation-id="activeAnnotationId"
                :comment-body="commentBody"
                :user-name="user.name"
                @update:draft="annotation = { ...annotation, ...$event }"
                @update:comment-body="commentBody = $event"
                @save-annotation="saveAnnotation"
                @post-comment="postComment"
              />
            </section>
```

顶部 import 补上：

```typescript
import AnnotationSidebar from './AnnotationSidebar.vue'
import ChapterToolbar from './ChapterToolbar.vue'
```

- [ ] **Step 5: 改布局样式**

`web/src/styles.css` 第 123 行改成：

```css
.notes-grid { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2rem; align-items: start; }
```

第 213 行附近媒体查询里的 `.notes-grid { grid-template-columns: 1fr; }` 保持不变。

同时删掉随模板一起作废的四条规则（其余 `.annotation-*` 规则仍被侧边栏使用，不要删）：

```css
.annotation-panel { padding: 1.2rem; border: 1px solid #dcddd4; border-radius: 12px; background: #fbfaf6; }
.annotation-panel h3 { margin-top: 0; font-family: Georgia, serif; }
.annotation-panel blockquote { margin: .7rem 0; padding: .7rem; border-left: 3px solid #d5ad52; color: #6d664f; background: #f8f1d9; }
.chapter-document-actions { display: flex; justify-content: flex-end; margin-bottom: .8rem; }
```

- [ ] **Step 6: 运行测试确认通过**

Run: `cd web && npx vitest run src/components/DashboardView.spec.ts`
Expected: PASS，13 个用例全绿。

- [ ] **Step 7: 全量前端验收并提交**

```bash
cd web && npm test && npm run typecheck && npm run build
git add web/src/components/DashboardView.vue web/src/components/DashboardView.spec.ts web/src/styles.css
git commit -m "feat: drive the chapter page from the floating toolbar and annotation sidebar"
```

---

### Task 7: 真实浏览器验收

**Files:**
- 无代码改动；只在发现问题时回到对应任务修复

**Interfaces:**
- Consumes: Task 1-6 的全部产出
- Produces: 无

- [ ] **Step 1: 确认开发环境在跑**

Run: `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:5173/`
Expected: `200`。若不是，先让人起 `make watch`，不要自行重置数据库。

- [ ] **Step 2: 走一遍真实路径**

打开 `http://localhost:5173/subjects/m4gAZ1DV8M`，用 `admin@localhost` / `admin12345678` 登录，然后逐项确认：

1. 浮动栏在正文底部居中，读态显示 `白板` / `标注 N` / `讨论 N`。
2. 选中一段正文 → 浮动栏变形并显示引用；选颜色 → 点 `写标注` → 侧边栏展开、引用与颜色已填、可写笔记并保存。
3. 保存后正文对应文字出现该颜色的标记。
4. 点标记 → 侧边栏展开到标注页，对应卡片高亮。
5. 点侧边栏左缘中部的把手 → 侧边栏收起，正文变全宽；再点 → 展开。
6. 点 `白板` → 浮动栏出现 `↶ ↷ 🗑 ✓`，画一笔、撤销、保存都生效；Excalidraw 自带工具栏只剩绘制工具与拖动把手。
7. 切到 `讨论` tab → 能发一条讨论。
8. 浏览器窗口收窄到 900px 以下 → 侧边栏变成盖在正文上的抽屉，正文不被挤压。

- [ ] **Step 3: 全量验收**

先向人确认可以清空 `lapin_test`（`make watch` 与测试共用该库），得到确认后运行：

```bash
cd web && npm test && npm run typecheck && npm run build
TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable' go test -race -coverpkg=./... ./...
go vet ./...
go build ./cmd/lapin && rm -f lapin
```

跑完按 `docs/superpowers/specs/2026-08-18-floating-toolbar-annotation-sidebar-design.md` 末尾的说明重建管理员并重新导入课程。

- [ ] **Step 4: 提交收尾**

```bash
git add -A
git commit -m "test: verify the floating toolbar and annotation sidebar in a browser"
```
