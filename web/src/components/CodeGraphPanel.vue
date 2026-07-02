<!--
  CodeGraphPanel — 代码图谱浮层。

  代码图谱不是预设的、也不可能整图渲染（项目几万文件）。
  本组件「按需展开」：以一个种子符号为中心拉 callers/callees 作 1 跳邻居，
  双击任意节点 → 拉它的邻居动态长进同一张图（实时渲染），
  单击节点 → 锁定高亮 + 右栏拉源码（点进有东西看）。

  交互层完全对齐 GraphView：filter/search、hover 高亮+dim、单击锁定/解锁、
  双击 concentric 局部重布局、工具栏(fit/zoom/relayout)、hover info 卡片。
  单/双击用 240ms 防抖分离，避免双击被 tap 解锁"吃掉"。用 cytoscape（项目已依赖）。
-->
<template>
  <Teleport to="body">
  <div class="cgp-mask">
    <div class="cgp-panel">
      <div class="cgp-head">
        <div>
          <div class="cgp-eyebrow">CODE GRAPH</div>
          <div class="cgp-title">{{ seed }}</div>
        </div>
        <!-- 面板布局切换 -->
        <div class="cgp-layout-btns">
          <button
            v-for="layout in layouts" :key="layout.id"
            :class="['layout-btn', { active: panelLayout === layout.id }]"
            :title="layout.title"
            @click="panelLayout = layout.id"
          >{{ layout.icon }}</button>
        </div>
        <button class="cgp-close" @click="$emit('close')">×</button>
      </div>

      <div class="cgp-body">
        <!-- 左：图 -->
        <div class="cgp-graph" v-show="panelLayout !== 'source-only'">
          <div v-if="loading" class="cgp-loading"><span class="loading-glyph">···</span> {{ t('code.graphLoading') }}</div>
          <div ref="cyEl" class="cgp-cy"></div>

          <!-- 左上：过滤 + 搜索（学 GraphView） -->
          <div class="cgp-filter">
            <button
              v-for="f in visibleFilters" :key="f.group"
              :class="['filter-btn', { active: f.visible }]"
              @click="f.visible = !f.visible; applyFilter()"
            >
              <span class="dot" :style="{ background: f.color }"></span>{{ groupLabel(f.group) }}
            </button>
            <div class="filter-search">
              <input v-model="searchQuery" type="text" :placeholder="t('code.searchNode')" @input="applySearch" />
              <button v-if="searchQuery" class="search-clear" @click="searchQuery = ''; applySearch()">×</button>
            </div>
          </div>

          <!-- 右上：工具栏 -->
          <div class="cgp-toolbar">
            <button @click="cyFit()" :title="t('graph.tipFit')">◳</button>
            <button @click="cyZoomIn()" :title="t('graph.tipZoomIn')">+</button>
            <button @click="cyZoomOut()" :title="t('graph.tipZoomOut')">−</button>
            <button @click="cyRelayout()" :title="t('graph.tipRelayout')">↻</button>
          </div>

          <!-- hover/选中节点信息卡 -->
          <div v-if="hoverInfo" class="cgp-info">
            <div class="ci-eyebrow">{{ hoverInfo.kind || 'symbol' }}</div>
            <div class="ci-title">{{ hoverInfo.label }}</div>
            <div v-if="hoverInfo.file" class="ci-meta">{{ hoverInfo.file }}<span v-if="hoverInfo.line">:{{ hoverInfo.line }}</span></div>
            <div class="ci-hint">{{ t('code.graphNodeHint') }}</div>
          </div>

          <div class="cgp-legend">
            <span><i class="lg lg-caller"></i>{{ t('code.caller') }}</span>
            <span><i class="lg lg-callee"></i>{{ t('code.callee') }}</span>
            <span class="cgp-tip">{{ t('code.graphTip') }}</span>
          </div>
        </div>

        <!-- 右：选中符号源码 -->
        <div class="cgp-source" v-show="panelLayout !== 'graph-only'">
          <div v-if="srcLoading" class="cgp-loading"><span class="loading-glyph">···</span></div>
          <template v-else-if="source && source.found">
            <!-- 头部：符号名 + 路径 + 搜索 + 大纲开关（单行紧凑） -->
            <div class="cgp-src-head">
              <div class="cgp-src-topbar">
                <div class="cgp-src-identity">
                  <span class="cgp-src-kind">{{ source.kind }}</span>
                  <span class="cgp-src-name">{{ source.name }}</span>
                  <span class="cgp-src-path">{{ source.file }}:{{ source.start_line }}</span>
                </div>
                <div class="cgp-src-actions">
                  <div class="src-search-wrap" :class="{ expanded: srcSearch }">
                    <span class="src-search-icon">⌕</span>
                    <input
                      v-model="srcSearch"
                      class="src-search-input"
                      type="text"
                      placeholder="搜索…"
                      @input="onSrcSearch"
                      @keydown.enter="jumpSrcMatch(1)"
                      @keydown.shift.enter.prevent="jumpSrcMatch(-1)"
                    />
                    <span v-if="srcSearch" class="src-search-count">{{ srcMatchIdx + 1 }}/{{ srcMatchCount }}</span>
                    <button v-if="srcSearch" class="src-search-nav" @click="jumpSrcMatch(-1)" title="↑">↑</button>
                    <button v-if="srcSearch" class="src-search-nav" @click="jumpSrcMatch(1)" title="↓">↓</button>
                    <button v-if="srcSearch" class="src-search-clear" @click="srcSearch=''; onSrcSearch()">×</button>
                  </div>
                  <button
                    class="src-outline-btn"
                    :class="{ active: showOutline }"
                    @click="showOutline = !showOutline"
                    title="大纲"
                  >≡</button>
                </div>
              </div>
            </div>
            <div class="cgp-src-body">
              <!-- 大纲栏（层级树） -->
              <div v-if="showOutline && outlineTree.length" class="cgp-outline">
                <div class="outline-tree">
              <OutlineNodeView
                v-for="node in outlineTree"
                :key="node.line"
                :node="node"
                :depth="0"
                :start-line="source.start_line || 1"
                :collapsed-set="outlineCollapsed"
                :highlight-lines="outlineHighlightLines"
                :active-line="activeOutlineLine"
                @jump="onOutlineJump"
                @toggle="onOutlineToggle"
              />
                </div>
              </div>
              <!-- 代码区 -->
              <div class="cgp-src-code md-content" ref="srcCodeEl" v-html="renderedSourceWithHL" @click="onSrcClick"></div>
            </div>
          </template>
          <div v-else class="cgp-src-empty">{{ t('code.clickNode') }}</div>
        </div>
      </div>
    </div>
  </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import cytoscape from 'cytoscape'
import type { Core } from 'cytoscape'
import { api } from '../api'
import { renderMarkdownWithWikilinks, handleCodeCopyClick } from '../markdown'

const { t } = useI18n()
const props = defineProps<{ space: string; seed: string }>()
const emit = defineEmits<{ close: [] }>()

// ESC 关闭 + 锁定 body 滚动
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
let prevBodyOverflow = ''
onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  prevBodyOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = prevBodyOverflow
})

const cyEl = ref<HTMLElement>()
const loading = ref(false)
const srcLoading = ref(false)
const source = ref<any>(null)
const hoverInfo = ref<{ label: string; kind: string; file?: string; line?: number } | null>(null)
const searchQuery = ref('')
let cy: Core | null = null
let locked: string | null = null
const expanded = new Set<string>()

// 大纲 + 代码搜索状态
const showOutline = ref(true)  // 默认展开
const srcSearch = ref('')
const srcMatchIdx = ref(0)
const srcMatchCount = ref(0)
const srcCodeEl = ref<HTMLElement>()

// 面板布局：both = 图 + 源码，graph-only = 仅图，source-only = 仅源码
type PanelLayout = 'both' | 'graph-only' | 'source-only'
const panelLayout = ref<PanelLayout>('both')
const layouts: Array<{ id: PanelLayout; icon: string; title: string }> = [
  { id: 'graph-only',  icon: '⬛',  title: '仅图谱' },
  { id: 'both',        icon: '◫',   title: '图谱 + 源码' },
  { id: 'source-only', icon: '▭',   title: '仅源码' },
]

// 切换到 graph-only 时通知 cytoscape resize，否则图会错位
watch(panelLayout, async () => {
  await nextTick()
  cy?.resize().fit()
})

// ---- 大纲解析（层级树）----
interface OutlineNode {
  line: number      // 在 pureCode 里的行号（1-based）
  label: string
  kind: string      // 'class'|'interface'|'struct'|'type'|'function'|'method'|'field'
  icon: string
  indent: number    // 源码缩进级别（用于判断父子）
  children: OutlineNode[]
  collapsed: boolean
}

const outlineCollapsed = ref<Set<number>>(new Set())  // 用行号标记已折叠的节点

// source 变化时重置折叠状态（默认全部展开）
watch(() => source.value, () => {
  srcSearch.value = ''
  srcMatchIdx.value = 0
  srcMatchCount.value = 0
  activeOutlineLine.value = 0
  outlineCollapsed.value = new Set()
})

function getPatterns(ext: string): Array<{ re: RegExp; kind: string; icon: string; group?: number; isContainer?: boolean }> {
  const P = (re: RegExp, kind: string, icon: string, isContainer = false, group = 1) => ({ re, kind, icon, group, isContainer })
  if (ext === 'go') return [
    P(/^type\s+(\w+)\s+(?:struct|interface)/, 'type', '◈', true),
    P(/^func\s+\(([^)]+)\)\s+(\w+)\s*\(/, 'method', '·ƒ', false, 2),
    P(/^func\s+(\w+)\s*\(/, 'function', 'ƒ'),
  ]
  if (['ts','tsx','js','jsx','mjs'].includes(ext)) return [
    P(/^\s*(?:export\s+)?(?:abstract\s+)?class\s+(\w+)/, 'class', '◈', true),
    P(/^\s*(?:export\s+)?interface\s+(\w+)/, 'interface', '◇', true),
    P(/^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)/, 'function', 'ƒ'),
    P(/^\s*(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(/, 'function', 'ƒ'),
    P(/^\s*(?:(?:public|protected|private|static|override|readonly|abstract|async|get|set)\s+)*(\w+)\s*(?:<[^>]*>)?\s*\(/, 'method', '·ƒ'),
  ]
  if (ext === 'php') return [
    P(/^\s*(?:abstract\s+|final\s+)?(?:readonly\s+)?class\s+(\w+)/, 'class', '◈', true),
    P(/^\s*interface\s+(\w+)/, 'interface', '◇', true),
    P(/^\s*(?:(?:public|protected|private|static|abstract|final)\s+)*function\s+(\w+)\s*\(/, 'function', 'ƒ'),
  ]
  if (ext === 'py') return [
    P(/^class\s+(\w+)/, 'class', '◈', true),
    P(/^\s+(?:async\s+)?def\s+(\w+)\s*\(/, 'method', '·ƒ'),
    P(/^(?:async\s+)?def\s+(\w+)\s*\(/, 'function', 'ƒ'),
  ]
  if (['java','kt'].includes(ext)) return [
    P(/^\s*(?:(?:public|protected|private|static|abstract|final|sealed)\s+)*(?:class|enum|record)\s+(\w+)/, 'class', '◈', true),
    P(/^\s*(?:(?:public|protected|private|static|abstract|final|override|suspend)\s+)*(?:fun\s+)?(\w+)\s*\(/, 'method', '·ƒ'),
  ]
  if (['c','cpp','cc','h','hpp'].includes(ext)) return [
    P(/^\s*(?:class|struct|enum)\s+(\w+)/, 'type', '◈', true),
    P(/^(?:[\w:*&<>\s]+)\s+(\w+)\s*\([^;]*\)\s*\{/, 'function', 'ƒ'),
  ]
  if (ext === 'rs') return [
    P(/^\s*(?:pub\s+)?(?:struct|enum|trait)\s+(\w+)/, 'type', '◈', true),
    P(/^\s*(?:pub\s+)?impl(?:\s+\w+)?\s+for\s+(\w+)/, 'impl', '⊕', true),
    P(/^\s*(?:pub\s+)?impl\s+(\w+)/, 'impl', '⊕', true),
    P(/^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)/, 'function', 'ƒ'),
  ]
  return [
    P(/^\s*(?:async\s+)?function\s+(\w+)/, 'function', 'ƒ'),
    P(/^\s*class\s+(\w+)/, 'class', '◈', true),
  ]
}

const IS_CONTAINER = new Set(['class','interface','type','struct','impl','enum','trait'])

const outlineTree = computed((): OutlineNode[] => {
  const s = source.value
  if (!s?.found || !s.code) return []
  const lines = pureCode(s.code).split('\n')
  const ext = (s.file || '').split('.').pop()?.toLowerCase() || ''
  const patterns = getPatterns(ext)
  const SKIP = new Set(['if','for','while','switch','catch','return','new','delete','else','try','finally','do','case','default','break','continue','throw'])

  // 0. 标记注释行：单行注释（// # --）、块注释（/* ... */）、块注释续行
  //    注释行不参与符号/调用扫描，避免注释内容被误识别成符号
  const hashLangs = ['py', 'rb', 'sh', 'bash', 'zsh', 'yaml', 'yml', 'toml']
  const usesHash = hashLangs.includes(ext)
  const isComment: boolean[] = new Array(lines.length).fill(false)
  let inBlock = false
  lines.forEach((lineText, idx) => {
    const trimmed = lineText.trim()
    if (inBlock) {
      isComment[idx] = true
      if (trimmed.includes('*/')) inBlock = false
      return
    }
    if (trimmed === '') return
    // 行内注释符
    if (trimmed.startsWith('//') || trimmed.startsWith('*')) { isComment[idx] = true; return }
    if (usesHash && trimmed.startsWith('#')) { isComment[idx] = true; return }
    // 块注释起始
    if (trimmed.startsWith('/*')) {
      isComment[idx] = true
      if (!trimmed.includes('*/')) inBlock = true
      return
    }
  })

  // 1. 扫出所有符号，记录行号+缩进
  interface Raw { line: number; label: string; kind: string; icon: string; indent: number }
  const raw: Raw[] = []
  lines.forEach((lineText, idx) => {
    if (isComment[idx]) return
    const indent = lineText.search(/\S/)
    for (const p of patterns) {
      const m = lineText.match(p.re)
      const g = p.group ?? 1
      if (m && m[g] && !SKIP.has(m[g])) {
        raw.push({ line: idx + 1, label: m[g], kind: p.kind, icon: p.icon, indent: indent < 0 ? 0 : indent })
        break
      }
    }
  })

  // 2. 构建层级树：用栈维护当前容器链
  const roots: OutlineNode[] = []
  const stack: OutlineNode[] = []  // 栈顶=最深的容器

  for (const item of raw) {
    const node: OutlineNode = { ...item, children: [], collapsed: false }

    // 找到第一个 indent 比自己小的容器作为父节点
    while (stack.length > 0) {
      const top = stack[stack.length - 1]
      if (IS_CONTAINER.has(top.kind) && top.indent < node.indent) break
      stack.pop()
    }

    if (stack.length > 0 && IS_CONTAINER.has(stack[stack.length - 1].kind)) {
      stack[stack.length - 1].children.push(node)
    } else {
      roots.push(node)
    }

    if (IS_CONTAINER.has(node.kind)) stack.push(node)
  }

  // 3. 对函数/方法节点，扫其函数体内的调用行（一层），作为子节点
  // 调用检测：匹配 `identifier(` 且不是定义行、不是常见关键字
  const CALL_SKIP = new Set([
    'if','for','while','switch','catch','return','new','delete','else','try',
    'finally','do','case','default','break','continue','throw','print','echo',
    'isset','empty','unset','list','array','require','include','die','exit',
  ])
  // 按语言调整调用检测正则
  const isPhpLike = ['php'].includes(ext)
  const callRe = isPhpLike
    ? /(?:\$this->|self::|\$\w+->|static::)?(\w+)\s*\(/g
    : /(?:this\.|\w+\.)?(\w+)\s*\(/g

  // 找到函数节点在 raw 里对应的下一个同级节点行，确定函数体范围
  const allNodes: OutlineNode[] = []
  function collectAll(nodes: OutlineNode[]) {
    for (const n of nodes) { allNodes.push(n); collectAll(n.children) }
  }
  collectAll(roots)

  function addCallChildren(fnNode: OutlineNode) {
    if (fnNode.children.length > 0) {
      // 已有子节点（类里的方法），递归处理这些方法
      for (const child of fnNode.children) addCallChildren(child)
      return
    }
    if (!['function','method'].includes(fnNode.kind)) return

    // 找函数体范围：从 fnNode.line 到下一个同缩进节点的行（或末尾）
    const myIdx = allNodes.indexOf(fnNode)
    let bodyEnd = lines.length
    for (let i = myIdx + 1; i < allNodes.length; i++) {
      if (allNodes[i].indent <= fnNode.indent) {
        bodyEnd = allNodes[i].line - 1
        break
      }
    }

    const seen = new Set<string>()
    const calls: OutlineNode[] = []

    for (let i = fnNode.line; i < bodyEnd; i++) {
      if (isComment[i]) continue
      // 剥掉行尾注释，避免注释里的 xxx() 被当成调用
      let lineText = lines[i]
      const lc = lineText.indexOf('//')
      if (lc >= 0) lineText = lineText.slice(0, lc)
      if (usesHash) {
        const hc = lineText.indexOf('#')
        if (hc >= 0) lineText = lineText.slice(0, hc)
      }
      callRe.lastIndex = 0
      let m: RegExpExecArray | null
      while ((m = callRe.exec(lineText)) !== null) {
        const name = m[1]
        if (!name || CALL_SKIP.has(name) || seen.has(name)) continue
        // 跳过全大写常量、数字开头等
        if (/^[A-Z_]+$/.test(name) || /^\d/.test(name)) continue
        seen.add(name)
        calls.push({
          line: i + 1,  // 调用所在行（相对 pureCode）
          label: name,
          kind: 'call',
          icon: '→',
          indent: fnNode.indent + 2,
          children: [],
          collapsed: true,
        })
      }
    }

    if (calls.length > 0) {
      fnNode.children = calls
      fnNode.collapsed = true  // 默认折叠，避免撑开大纲
    }
  }

  for (const root of roots) addCallChildren(root)

  return roots
})

function toggleOutlineNode(node: OutlineNode) {
  node.collapsed = !node.collapsed
}

// 大纲展开时高亮代码里对应的行集合（call 子节点的行号）
const outlineHighlightLines = ref<Set<number>>(new Set())  // 保留供 prop 传递
const activeOutlineLine = ref<number>(0)  // 当前选中的大纲行

function onOutlineToggle(node: OutlineNode) {
  const s = outlineCollapsed.value
  if (s.has(node.line)) s.delete(node.line)
  else s.add(node.line)
  outlineCollapsed.value = new Set(s)
}

function onOutlineJump(line: number, label?: string) {
  activeOutlineLine.value = line
  jumpToLine(line)
}

// outlineHighlightNames 废弃（改用 srcSearch 复用搜索高亮），保留空 ref 不影响 prop
const outlineHighlightNames = ref<Set<string>>(new Set())

// ---- 代码搜索 ----
// 把 renderedSource 里匹配的文本 wrap 成 <mark class="src-hl"> 并滚动定位
const renderedSourceWithHL = computed(() => {
  let base = renderedSource.value
  const jl = jumpHighlightLine.value

  // 跳转行高亮：给目标行包一个 span.jump-hl
  if (jl > 0) {
    base = base.replace(/<code[^>]*>([\s\S]*?)<\/code>/g, (match, inner) => {
      // 按 \n 拆行，给第 jl 行加 wrap
      const lines = inner.split('\n')
      if (jl - 1 < lines.length) {
        lines[jl - 1] = `<span class="jump-hl">${lines[jl - 1]}</span>`
      }
      return match.replace(inner, lines.join('\n'))
    })
  }

  // 搜索高亮
  if (!srcSearch.value.trim()) return base
  const q = srcSearch.value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return base.replace(/<code[^>]*>([\s\S]*?)<\/code>/g, (match, inner) => {
    const highlighted = inner.replace(
      new RegExp(`(${q})`, 'gi'),
      '<mark class="src-hl">$1</mark>'
    )
    return match.replace(inner, highlighted)
  })
})

function onSrcSearch() {
  srcMatchIdx.value = 0
  nextTick(() => {
    const el = srcCodeEl.value
    if (!el) return
    const marks = el.querySelectorAll<HTMLElement>('.src-hl')
    srcMatchCount.value = marks.length
    if (marks.length > 0) {
      marks.forEach((m, i) => m.classList.toggle('src-hl-active', i === 0))
      marks[0].scrollIntoView({ block: 'nearest' })
    }
  })
}

function jumpSrcMatch(dir: 1 | -1) {
  const el = srcCodeEl.value
  if (!el) return
  const marks = el.querySelectorAll<HTMLElement>('.src-hl')
  if (!marks.length) return
  marks[srcMatchIdx.value]?.classList.remove('src-hl-active')
  srcMatchIdx.value = (srcMatchIdx.value + dir + marks.length) % marks.length
  marks[srcMatchIdx.value]?.classList.add('src-hl-active')
  marks[srcMatchIdx.value]?.scrollIntoView({ block: 'nearest' })
}

// ---- 大纲跳转 ----
// 当前高亮行号（用于行高亮动画）
const jumpHighlightLine = ref<number>(0)
let jumpHLTimer: ReturnType<typeof setTimeout> | null = null

function jumpToLine(lineNum: number) {
  // 触发行高亮动画
  if (jumpHLTimer) clearTimeout(jumpHLTimer)
  jumpHighlightLine.value = lineNum
  jumpHLTimer = setTimeout(() => { jumpHighlightLine.value = 0 }, 2200)

  nextTick(() => {
    const el = srcCodeEl.value
    if (!el) return
    const pre = el.querySelector('pre')
    const code = el.querySelector('code')
    if (!code) return
    // highlight.js 渲染后每行是一个 <span class="hljs-ln-n"> 或直接在 <code> 里换行
    const lineEls = code.querySelectorAll('.hljs-ln-n, [data-line-number]')
    if (lineEls.length >= lineNum) {
      lineEls[lineNum - 1].scrollIntoView({ behavior: 'smooth', block: 'center' })
      return
    }
    // 通用方案：TreeWalker 数换行定位
    const walker = document.createTreeWalker(code, NodeFilter.SHOW_TEXT)
    let currentLine = 1
    let node: Text | null
    while ((node = walker.nextNode() as Text)) {
      const text = node.textContent || ''
      const newlines = text.split('\n').length - 1
      if (currentLine + newlines >= lineNum) {
        const lineOffset = lineNum - currentLine
        const charPos = text.split('\n').slice(0, lineOffset).join('\n').length
        try {
          const range = document.createRange()
          range.setStart(node, Math.min(charPos, text.length))
          range.collapse(true)
          const dummy = document.createElement('span')
          dummy.style.cssText = 'position:absolute;pointer-events:none;'
          range.insertNode(dummy)
          dummy.scrollIntoView({ behavior: 'smooth', block: 'center' })
          dummy.remove()
        } catch { /* ignore */ }
        return
      }
      currentLine += newlines
    }
    // 兜底：按比例滚动
    if (pre) {
      const totalLines = (code.textContent || '').split('\n').length
      pre.scrollTop = (lineNum / totalLines) * pre.scrollHeight
    }
  })
}

// 节点 kind → 大类（filter 维度）。kind 太碎，归三类。
function kindGroup(kind?: string): string {
  const k = (kind || '').toLowerCase()
  if (k === 'function' || k === 'method' || k === 'func') return 'fn'
  if (k === 'class' || k === 'interface' || k === 'struct' || k === 'trait' || k === 'enum') return 'type'
  return 'other'
}

const filters = reactive([
  { group: 'fn',    color: '#2d4a6b', visible: true },
  { group: 'type',  color: '#1a1814', visible: true },
  { group: 'other', color: '#5b6f3f', visible: true },
])
// 只显示图里实际出现的分组
const presentGroups = ref<string[]>([])
const visibleFilters = computed(() => filters.filter(f => presentGroups.value.includes(f.group)))
function refreshGroups() {
  if (!cy) return
  const set = new Set<string>()
  cy.nodes().forEach(n => { set.add(kindGroup(n.data('kind'))) })
  presentGroups.value = Array.from(set)
}
function groupLabel(g: string): string {
  return t(g === 'fn' ? 'code.fnGroup' : g === 'type' ? 'code.typeGroup' : 'code.otherGroup')
}

const STYLE: any[] = [
  { selector: 'node', style: {
      label: 'data(label)',
      'font-family': 'IBM Plex Mono, ui-monospace, monospace',
      'font-size': '10px', 'font-weight': 500, color: '#1a1814',
      'text-valign': 'bottom', 'text-margin-y': 5,
      'text-outline-color': '#f5f0e6', 'text-outline-width': 3,
      'text-wrap': 'ellipsis', 'text-max-width': '110px', 'min-zoomed-font-size': 6,
      // 默认（other 类，对应图例"其它"）：苔绿圆角
      'background-color': '#5b6f3f', width: 16, height: 16,
      shape: 'round-rectangle', 'border-width': 1, 'border-color': '#1a1814',
  }},
  // 函数 / 方法（对应图例"函数/方法"）：靛蓝椭圆
  { selector: 'node[kind="function"], node[kind="method"], node[kind="func"]', style: {
      'background-color': '#2d4a6b', width: 20, height: 20, shape: 'ellipse',
  }},
  // 类型（class/interface/struct/trait/enum）：墨色菱形
  { selector: 'node[kind="class"], node[kind="interface"], node[kind="struct"], node[kind="trait"], node[kind="enum"]', style: {
      'background-color': '#1a1814', width: 18, height: 18, shape: 'diamond',
  }},
  // 选中态：墨色细边
  { selector: 'node:selected', style: { 'border-width': 2, 'border-color': '#1a1814' } },
  { selector: 'edge', style: {
      width: 1.6, 'line-color': 'rgba(26,24,20,0.4)',
      'target-arrow-shape': 'triangle', 'target-arrow-color': 'rgba(26,24,20,0.5)',
      'arrow-scale': 1.4, 'curve-style': 'bezier', opacity: 0.85,
  }},
  // caller → 中心（"传入"，调用方）：暖橙
  { selector: 'edge.e-caller', style: { 'line-color': '#d97a3a', 'target-arrow-color': '#d97a3a', width: 1.8 } },
  // 中心 → callee（"传出"，被调用）：冷青
  { selector: 'edge.e-callee', style: { 'line-color': '#3a7a8c', 'target-arrow-color': '#3a7a8c', width: 1.8 } },
  // 高亮态：放大字号 + 提层级，不加额外染色
  { selector: '.highlight', style: { 'font-size': '12px', 'font-weight': 600, 'z-index': 999 } },
  { selector: '.neighbor', style: { opacity: 0.95 } },
  { selector: 'edge.highlight', style: { width: 2.4, opacity: 1 } },
  { selector: '.search-match', style: { 'font-weight': 700, 'z-index': 999 } },
  { selector: '.dimmed', style: { opacity: 0.16 } },
  { selector: 'edge.dimmed', style: { opacity: 0.05 } },
  { selector: '.hidden', style: { display: 'none' } },
]

// 拉某符号邻居并加进图（不负责布局）
async function fetchNeighbors(symbol: string, isSeed = false) {
  if (expanded.has(symbol)) return
  expanded.add(symbol)
  if (isSeed) loading.value = true
  try {
    const res = await api.codeGraphNeighbors(props.space, symbol)
    if (!cy) return
    const toAdd: any[] = []
    // willHave: 加完这批后图中会存在的所有节点 id（已有 + 本批新增）。
    // cytoscape 加边时两端节点必须已存在，否则 cy.add 整批抛错、连线全丢——
    // 这是之前"连接逻辑不对"的根因。加边前用它校验端点。
    const willHave = new Set<string>()
    cy.nodes().forEach(n => { willHave.add(n.id()) })
    for (const n of res.nodes || []) {
      if (cy.getElementById(n.id).length === 0) {
        // 不存 center 字段——切换中心时旧节点会残留 center=1，统一靠 reclassifyEdges 处理边色
        toAdd.push({ group: 'nodes', data: { id: n.id, label: n.label, kind: n.kind, file: n.file, line: n.line } })
      }
      willHave.add(n.id)
    }
    const edgeSeen = new Set<string>()
    for (const e of res.edges || []) {
      const eid = `${e.source}->${e.target}`
      if (edgeSeen.has(eid)) continue
      if (!willHave.has(e.source) || !willHave.has(e.target)) continue // 端点缺失则跳过
      edgeSeen.add(eid)
      const cls = e.type === 'caller' ? 'e-caller' : e.type === 'callee' ? 'e-callee' : ''
      const existing = cy.getElementById(eid)
      if (existing.length > 0) {
        // 边已存在但中心切换后语义可能变了（同一条边相对新旧中心一个是 caller 一个是 callee）。
        // 必须重置 class，否则连线保持旧颜色——这就是"中心转换颜色没跟上"的根因。
        existing.removeClass('e-caller e-callee')
        if (cls) existing.addClass(cls)
        continue
      }
      toAdd.push({ group: 'edges', data: { id: eid, source: e.source, target: e.target }, classes: cls })
    }
    cy.add(toAdd)
    refreshGroups()
  } catch {
    // ignore
  } finally {
    if (isSeed) loading.value = false
  }
}

// 以指定节点为新中心，重新刷所有边的 caller/callee class（不重拉数据）。
// 切换中心后，每条边相对中心的语义角色会变（同一条边对 A 是 callee，对 B 可能是 caller），
// 颜色必须跟着切。靠边的 source/target 与新中心 id 推断方向：
//   - 边的 target == 中心 → 该边是 caller（外部 → 中心）
//   - 边的 source == 中心 → 该边是 callee（中心 → 外部）
//   - 与中心无直接关联的边 → 清空 class，走默认灰
function reclassifyEdges(centerId: string) {
  if (!cy) return
  cy.edges().forEach(e => {
    e.removeClass('e-caller e-callee')
    const s = e.data('source'); const t = e.data('target')
    if (t === centerId) e.addClass('e-caller')
    else if (s === centerId) e.addClass('e-callee')
  })
}

// 单击：聚焦该节点 = 锁定 + concentric + 拉源码 + 刷边色
async function focusNode(node: any) {
  const id = node.id()
  locked = id
  reclassifyEdges(id) // 中心切换 → 同步重打边的 caller/callee 颜色
  relayoutAround(node)
  hoverInfo.value = nodeInfo(node)
  loadSource(id)
}

// 双击：先展开邻居（已展开则跳过 API），再以新中心刷边色 + concentric 聚焦
async function expandNode(symbol: string) {
  await fetchNeighbors(symbol)
  if (!cy) return
  const node = cy.getElementById(symbol)
  if (node && node.length) {
    locked = symbol
    reclassifyEdges(symbol)
    relayoutAround(node)
    hoverInfo.value = nodeInfo(node)
  }
}

// 拉某符号的源码到右栏。
// 正确性靠"用 file+line 精确坐标查后端"——节点自带 codegraph 给的 filePath/startLine，
// 后端按 (file, startLine) 锁定具体符号，不会被同名/前缀符号顶替（如 "g" 误命中 "get_groups"）。
async function loadSource(symbol: string) {
  srcLoading.value = true
  source.value = null
  let file: string | undefined
  let line: number | undefined
  if (cy) {
    const node = cy.getElementById(symbol)
    if (node && node.length) {
      const f = node.data('file'); const l = node.data('line')
      if (typeof f === 'string' && f) file = f
      if (typeof l === 'number' && l > 0) line = l
    }
  }
  try {
    source.value = await api.codeSymbolSource(props.space, symbol, file, line)
  } catch {
    source.value = { found: false }
  } finally {
    srcLoading.value = false
  }
}

// ---- 源码语法高亮（复用项目 markdown 渲染：marked + highlight.js）----
function pureCode(code?: string): string {
  if (!code) return ''
  return code
    .replace(/^\/\/[^\n]*\n/, '')
    .split('\n')
    .map(l => l.replace(/^\d+\t/, ''))
    .join('\n')
    .replace(/\n+$/, '')
}
function langFromFile(file?: string): string {
  const ext = (file || '').split('.').pop()?.toLowerCase() || ''
  const map: Record<string, string> = {
    php: 'php', js: 'javascript', mjs: 'javascript', cjs: 'javascript',
    ts: 'typescript', tsx: 'typescript', jsx: 'javascript',
    go: 'go', py: 'python', rb: 'ruby', java: 'java', kt: 'kotlin',
    c: 'c', h: 'c', cpp: 'cpp', cc: 'cpp', hpp: 'cpp', cs: 'csharp',
    rs: 'rust', swift: 'swift', sh: 'bash', bash: 'bash', zsh: 'bash',
    sql: 'sql', json: 'json', yaml: 'yaml', yml: 'yaml', xml: 'xml',
    html: 'xml', vue: 'xml', css: 'css', scss: 'scss', less: 'less',
    md: 'markdown', lua: 'lua',
  }
  return map[ext] || ''
}
const renderedSource = computed(() => {
  const s = source.value
  if (!s || !s.found) return ''
  const lang = langFromFile(s.file)
  return renderMarkdownWithWikilinks('```' + lang + '\n' + pureCode(s.code) + '\n```')
})
function onSrcClick(e: Event) { handleCodeCopyClick(e) }

// ---- GraphView 同款交互 ----
function highlight(node: any) {
  if (!cy) return
  const hood = node.neighborhood().add(node)
  cy.elements().not('.hidden').addClass('dimmed').removeClass('highlight neighbor')
  hood.removeClass('dimmed')
  hood.edges().addClass('highlight')
  hood.nodes().not(node).addClass('neighbor')
  node.addClass('highlight')
}
function clearHighlight() { cy?.elements().removeClass('dimmed highlight neighbor') }
function unlock() { locked = null; clearHighlight(); hoverInfo.value = null }

// concentric：节点居中、邻居环绕，再 fit 到此子图
function relayoutAround(node: any) {
  if (!cy) return
  const id = node.id()
  const hood = node.neighborhood().add(node).not('.hidden')
  hood.layout({
    name: 'concentric',
    concentric: (n: any) => (n.id() === id ? 2 : 1),
    levelWidth: () => 1,
    minNodeSpacing: 48,
    spacingFactor: 1.35,
    animate: true,
    animationDuration: 520,
    fit: false,
  } as any).run()
  // 不在这里 highlight——避免初始/切换中心时把整片邻域染红，让 caller/callee 语义色保持可读。
  // 高亮只由用户主动 hover/单击触发。
  setTimeout(() => { cy?.animate({ fit: { eles: hood, padding: 70 } }, { duration: 440 }) }, 530)
}

function cyFit() { cy?.fit(undefined, 40) }
function cyZoomIn() { if (cy) cy.zoom({ level: cy.zoom() * 1.3, renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 } }) }
function cyZoomOut() { if (cy) cy.zoom({ level: cy.zoom() * 0.7, renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 } }) }
function cyRelayout() {
  if (!cy) return
  if (locked) {
    const n = cy.getElementById(locked)
    if (n.length) { relayoutAround(n); return }
  }
  unlock()
  cy.layout({ name: 'cose', animate: true, animationDuration: 800, nodeRepulsion: () => 13000, idealEdgeLength: () => 130, gravity: 0.18, numIter: 400, fit: true, padding: 60 } as any).run()
}

// 类型过滤：隐藏 / 显示某分组节点（及其相连边）
function applyFilter() {
  if (!cy) return
  cy.nodes().forEach(n => {
    const f = filters.find(x => x.group === kindGroup(n.data('kind')))
    n.toggleClass('hidden', !(f?.visible ?? true))
  })
  cy.edges().forEach(e => {
    e.toggleClass('hidden', e.source().hasClass('hidden') || e.target().hasClass('hidden'))
  })
}

// 搜索：匹配节点高亮、邻居半亮、其余暗化（学 GraphView）
function applySearch() {
  if (!cy) return
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) { cy.elements().removeClass('dimmed highlight neighbor search-match'); applyFilter(); return }
  applyFilter()
  const visibleNodes = cy.nodes().not('.hidden')
  const matched = visibleNodes.filter(n => String(n.data('label') || '').toLowerCase().includes(q))
  visibleNodes.addClass('dimmed').removeClass('highlight neighbor search-match')
  cy.edges().not('.hidden').addClass('dimmed').removeClass('highlight')
  if (matched.length === 0) return
  matched.removeClass('dimmed').addClass('search-match')
  matched.connectedEdges().not('.hidden').removeClass('dimmed').addClass('highlight')
  matched.neighborhood().nodes().not('.hidden').not(matched).removeClass('dimmed').addClass('neighbor')
}

function nodeInfo(node: any) {
  return { label: String(node.data('label') || node.id()), kind: String(node.data('kind') || ''), file: node.data('file'), line: node.data('line') }
}

onMounted(async () => {
  await nextTick()
  if (!cyEl.value) return
  cyEl.value.addEventListener('contextmenu', e => e.preventDefault())
  cy = cytoscape({
    container: cyEl.value, style: STYLE,
    wheelSensitivity: 0.25, minZoom: 0.2, maxZoom: 3.5,
  })

  // 单/双击防抖分离：240ms 内的第二次 tap 视为双击，避免单击 toggle 把双击"吃掉"
  let clickTimer: ReturnType<typeof setTimeout> | null = null
  cy.on('tap', 'node', (evt) => {
    const node = evt.target
    if (clickTimer) { clearTimeout(clickTimer); clickTimer = null; expandNode(node.id()); return }
    clickTimer = setTimeout(() => { clickTimer = null; focusNode(node) }, 240)
  })
  // 点空白处解锁
  cy.on('tap', (evt) => { if (evt.target === cy) unlock() })
  // hover：未锁定时临时高亮 + info 卡
  cy.on('mouseover', 'node', (evt) => { if (!locked) highlight(evt.target); hoverInfo.value = nodeInfo(evt.target) })
  cy.on('mouseout', 'node', () => { if (!locked) { clearHighlight(); hoverInfo.value = null } })

  await fetchNeighbors(props.seed, true)
  const seedNode = cy.getElementById(props.seed)
  if (seedNode && seedNode.length) {
    locked = props.seed
    reclassifyEdges(props.seed) // 初始以 seed 为中心刷边色
    relayoutAround(seedNode)
    hoverInfo.value = nodeInfo(seedNode)
  }
  loadSource(props.seed)
})

onUnmounted(() => { if (cy) { cy.destroy(); cy = null } })

// ---- 大纲树节点递归组件 ----
// 用 defineComponent 内联，避免新建文件
import { defineComponent, h } from 'vue'
const OutlineNodeView: any = defineComponent({
  name: 'OutlineNodeView',
  props: {
    node: { type: Object as () => OutlineNode, required: true },
    depth: { type: Number, default: 0 },
    startLine: { type: Number, default: 1 },
    collapsedSet: { type: Object as () => Set<number>, required: true },
    highlightLines: { type: Object as () => Set<number>, default: () => new Set() },
    activeLine: { type: Number, default: 0 },
  },
  emits: ['jump', 'toggle'],
  setup(props, { emit }) {
    return () => {
      const n = props.node
      const hasChildren = n.children.length > 0
      const isCollapsed = props.collapsedSet.has(n.line)
      const realLine = n.line + props.startLine - 1
      const isActive = props.activeLine === n.line

      // 图标映射：用短字母徽章替代纯文字符号
      const iconMap: Record<string, { letter: string; cls: string }> = {
        'class': { letter: 'C', cls: 'ol-badge-class' },
        'interface': { letter: 'I', cls: 'ol-badge-interface' },
        'type': { letter: 'T', cls: 'ol-badge-type' },
        'struct': { letter: 'S', cls: 'ol-badge-type' },
        'impl': { letter: 'I', cls: 'ol-badge-type' },
        'enum': { letter: 'E', cls: 'ol-badge-type' },
        'function': { letter: 'ƒ', cls: 'ol-badge-fn' },
        'method': { letter: 'm', cls: 'ol-badge-method' },
        'call': { letter: '→', cls: 'ol-badge-call' },
      }
      const badge = iconMap[n.kind] || { letter: '·', cls: 'ol-badge-other' }

      // 子节点递归（仅展开时渲染）
      const childNodes = hasChildren && !isCollapsed
        ? n.children.map((child: OutlineNode) =>
            h(OutlineNodeView, {
              key: child.line,
              node: child,
              depth: props.depth + 1,
              startLine: props.startLine,
              collapsedSet: props.collapsedSet,
              highlightLines: props.highlightLines,
              activeLine: props.activeLine,
              onJump: (line: number, label: string) => emit('jump', line, label),
              onToggle: (node: OutlineNode) => emit('toggle', node),
            })
          )
        : []

      const indent = props.depth * 16

      const row = h('div', {
        class: ['outline-item', `ol-${n.kind}`, isActive ? 'ol-active' : ''],
        style: { paddingLeft: `${4 + indent}px` },
        onClick: (e: Event) => {
          e.stopPropagation()
          emit('jump', n.line, n.label)
        },
      }, [
        // 缩进参考线（depth > 0 时显示）
        ...(props.depth > 0
          ? [h('span', { class: 'ol-guide', style: { left: `${4 + (props.depth - 1) * 16 + 8}px` } })]
          : []),
        // 折叠箭头
        hasChildren
          ? h('span', {
              class: ['ol-arrow', isCollapsed ? 'ol-arrow-closed' : ''],
              onClick: (e: Event) => { e.stopPropagation(); emit('toggle', n) },
            }, isCollapsed ? '›' : '⌄')
          : h('span', { class: 'ol-arrow-placeholder' }),
        // 类型徽章
        h('span', { class: ['ol-badge', badge.cls] }, badge.letter),
        // 标签名
        h('span', { class: 'ol-label' }, n.label),
        // 行号（右对齐）
        h('span', { class: 'ol-line' }, String(realLine)),
      ])

      return h('div', { class: 'outline-node' }, [row, ...childNodes])
    }
  },
})
</script>

<style scoped>
.cgp-mask {
  position: fixed; inset: 0; z-index: 200;
  background: rgba(20, 18, 15, 0.45);
  display: flex;
}
.cgp-panel {
  width: 100%; height: 100%;
  background: var(--paper);
  display: flex; flex-direction: column;
}
.cgp-head { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; border-bottom: 1.5px solid var(--ink); background: #171411; color: #f5efe5; gap: 12px; }
.cgp-layout-btns { display: flex; gap: 2px; margin-left: auto; }
.layout-btn { appearance: none; background: transparent; border: 1px solid rgba(245,240,230,0.2); color: rgba(245,240,230,0.5); cursor: pointer; width: 28px; height: 28px; font-size: 14px; display: flex; align-items: center; justify-content: center; transition: color 120ms, border-color 120ms, background 120ms; line-height: 1; }
.layout-btn:hover { color: #f5efe5; border-color: rgba(245,240,230,0.6); }
.layout-btn.active { color: #f5efe5; border-color: #f5efe5; background: rgba(245,240,230,0.1); }
.cgp-eyebrow { font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.14em; color: #bfb4a5; }
.cgp-title { font-family: var(--font-display); font-style: italic; font-size: 20px; margin-top: 2px; }
.cgp-close { appearance: none; background: none; border: 0; color: #f5efe5; font-size: 24px; cursor: pointer; line-height: 1; }
.cgp-close:hover { color: var(--vermilion); }
.cgp-body { flex: 1; display: flex; min-height: 0; }
.cgp-graph { flex: 1.4; position: relative; border-right: 1px solid var(--paper-edge); min-width: 0;
  background:
    radial-gradient(at 30% 20%, rgba(200, 48, 46, 0.04) 0, transparent 50%),
    radial-gradient(at 75% 70%, rgba(45, 74, 107, 0.05) 0, transparent 50%),
    var(--paper);
}
.cgp-cy { position: absolute; inset: 0; }
.cgp-loading { position: absolute; top: 12px; left: 12px; z-index: 5; font-family: var(--font-mono); font-size: 12px; color: var(--ink-3); }
.loading-glyph { color: var(--vermilion); animation: typewriter 0.9s steps(4) infinite; }
@keyframes typewriter { 0% { opacity: 0.2; } 50% { opacity: 1; } 100% { opacity: 0.2; } }

/* 左上：过滤 + 搜索（学 GraphView） */
.cgp-filter { position: absolute; top: 12px; left: 12px; display: flex; gap: 6px; align-items: center; flex-wrap: wrap; z-index: 10; background: rgba(245,240,230,0.82); backdrop-filter: blur(6px); padding: 5px 7px; border: 1px solid var(--paper-edge); max-width: 70%; }
.filter-btn { appearance: none; border: 1px solid transparent; background: transparent; padding: 3px 9px; font-family: var(--font-mono); font-size: 11px; letter-spacing: 0.04em; color: var(--ink-3); cursor: pointer; display: flex; align-items: center; gap: 6px; transition: color 130ms ease, border-color 130ms ease; }
.filter-btn:hover { color: var(--ink); }
.filter-btn.active { color: var(--ink); border-color: var(--ink); }
.filter-btn .dot { width: 8px; height: 8px; display: inline-block; }
.filter-search { position: relative; }
.filter-search input { width: 130px; padding: 3px 22px 3px 8px; border: 0; border-bottom: 1px solid var(--ink-4); background: transparent; color: var(--ink); font-family: var(--font-mono); font-size: 11px; outline: none; }
.filter-search input:focus { border-bottom-color: var(--ink); }
.filter-search input::placeholder { color: var(--ink-4); font-style: italic; }
.search-clear { position: absolute; right: 0; top: 50%; transform: translateY(-50%); background: none; border: none; color: var(--ink-4); cursor: pointer; font-size: 13px; padding: 2px 4px; line-height: 1; }
.search-clear:hover { color: var(--vermilion); }

/* 右上：工具栏 */
.cgp-toolbar { position: absolute; top: 12px; right: 12px; display: flex; z-index: 10; background: rgba(245,240,230,0.82); backdrop-filter: blur(6px); border: 1px solid var(--paper-edge); }
.cgp-toolbar button { width: 30px; height: 30px; appearance: none; background: transparent; border: 0; border-right: 1px solid var(--paper-edge); color: var(--ink-2); cursor: pointer; font-family: var(--font-display); font-size: 15px; line-height: 1; transition: background 130ms ease, color 130ms ease; }
.cgp-toolbar button:last-child { border-right: 0; }
.cgp-toolbar button:hover { background: var(--ink); color: var(--paper); }

/* info 卡（hover/选中节点） */
.cgp-info { position: absolute; top: 52px; right: 12px; background: var(--paper); border: 1.5px solid var(--ink); padding: 12px 16px 10px; z-index: 10; min-width: 180px; max-width: 280px; box-shadow: 4px 4px 0 rgba(26,24,20,0.12); }
.ci-eyebrow { font-family: var(--font-mono); font-size: 9px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--ink-4); margin-bottom: 3px; }
.ci-title { font-family: var(--font-mono); font-size: 14px; font-weight: 600; color: var(--ink); word-break: break-all; line-height: 1.2; }
.ci-meta { font-family: var(--font-mono); font-size: 10px; color: var(--ink-4); margin-top: 4px; word-break: break-all; }
.ci-hint { margin-top: 8px; padding-top: 6px; border-top: 1px solid var(--paper-edge); font-family: var(--font-display); font-style: italic; font-size: 11px; color: var(--ink-4); }

.cgp-legend { position: absolute; bottom: 10px; left: 12px; display: flex; gap: 14px; align-items: center; font-family: var(--font-mono); font-size: 10px; color: var(--ink-3); background: rgba(255,252,244,0.85); padding: 4px 8px; }
.cgp-legend .lg { display: inline-block; width: 14px; height: 3px; margin-right: 5px; vertical-align: middle; }
.lg-caller { background: #d97a3a; }
.lg-callee { background: #3a7a8c; }
.cgp-tip { color: var(--ink-4); }
.cgp-source { flex: 1; display: flex; flex-direction: column; min-width: 0; background: var(--paper-2); }

/* 头部：单行紧凑 */
.cgp-src-head { padding: 0; border-bottom: 1px solid var(--paper-edge); flex-shrink: 0; background: var(--paper); }
.cgp-src-topbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; min-height: 0; }
.cgp-src-identity { display: flex; align-items: baseline; gap: 6px; flex: 1; min-width: 0; overflow: hidden; }
.cgp-src-kind { font-family: var(--font-mono); font-size: 9px; letter-spacing: 0.1em; text-transform: uppercase; color: var(--ink-4); flex-shrink: 0; background: var(--paper-2); padding: 1px 5px; }
.cgp-src-name { font-family: var(--font-mono); font-size: 12px; font-weight: 600; color: var(--ink); flex-shrink: 0; max-width: 40%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cgp-src-path { font-family: var(--font-mono); font-size: 9px; color: var(--ink-4); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
.cgp-src-actions { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }

/* 搜索框：始终展开 */
.src-search-wrap { display: flex; align-items: center; gap: 2px; border: 1px solid var(--paper-edge); background: var(--paper-2); padding: 2px 6px; border-radius: 2px; flex: 1; }
.src-search-icon { color: var(--ink-4); font-size: 13px; cursor: text; flex-shrink: 0; }
.src-search-input { flex: 1; border: none; background: transparent; outline: none; font-family: var(--font-mono); font-size: 11px; color: var(--ink); padding: 2px 0; min-width: 60px; }
.src-search-input::placeholder { color: var(--ink-4); font-style: italic; }
.src-search-count { font-family: var(--font-mono); font-size: 9px; color: var(--ink-4); white-space: nowrap; flex-shrink: 0; }
.src-search-nav { appearance: none; border: none; background: transparent; color: var(--ink-4); cursor: pointer; font-size: 11px; padding: 1px 3px; line-height: 1; }
.src-search-nav:hover { color: var(--ink); }
.src-search-clear { appearance: none; border: none; background: transparent; color: var(--ink-4); cursor: pointer; font-size: 12px; padding: 1px 3px; line-height: 1; }
.src-search-clear:hover { color: var(--vermilion); }
.src-outline-btn { appearance: none; border: 1px solid transparent; background: transparent; color: var(--ink-4); cursor: pointer; font-size: 14px; padding: 2px 6px; line-height: 1; border-radius: 2px; transition: color 120ms, border-color 120ms, background 120ms; }
.src-outline-btn:hover { color: var(--ink); border-color: var(--paper-edge); }
.src-outline-btn.active { color: var(--ink); border-color: var(--ink); background: rgba(26,24,20,0.06); }

/* 搜索高亮（大纲点击也复用） */
.cgp-src-code :deep(.src-hl) { background: rgba(200,180,60,0.35); border-radius: 2px; }
.cgp-src-code :deep(.src-hl-active) { background: rgba(200,100,40,0.45); outline: 1px solid var(--vermilion); }

/* 源码主体：大纲 + 代码横排 */
.cgp-src-body { flex: 1; display: flex; min-height: 0; overflow: hidden; }

/* 大纲栏 */
.cgp-outline { width: 240px; min-width: 180px; flex-shrink: 0; border-right: 1px solid var(--paper-edge); overflow: hidden; background: var(--paper); display: flex; flex-direction: column; }
.cgp-outline :deep(.outline-tree) { flex: 1; overflow-y: auto; overflow-x: hidden; padding: 2px 0; }
.cgp-outline :deep(.outline-node) { position: relative; }
.cgp-outline :deep(.outline-item) {
  position: relative; display: flex; align-items: center; gap: 4px;
  padding: 1px 8px 1px 0; cursor: pointer;
  font-family: var(--font-mono); font-size: 12px; color: var(--ink-2);
  white-space: nowrap; overflow: hidden;
  transition: background 80ms; line-height: 22px; height: 24px;
}
.cgp-outline :deep(.outline-item:hover) { background: rgba(26,24,20,0.05); }
.cgp-outline :deep(.outline-item.ol-active) { background: rgba(45,74,107,0.12); color: var(--ink); }
.cgp-outline :deep(.outline-item.ol-active .ol-line) { color: var(--ink-3); }

/* 缩进参考线 */
.cgp-outline :deep(.ol-guide) { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--paper-edge); pointer-events: none; }

/* 折叠箭头 */
.cgp-outline :deep(.ol-arrow) {
  flex-shrink: 0; width: 18px; height: 18px;
  color: var(--ink-3); cursor: pointer; font-size: 11px;
  transition: transform 120ms, color 80ms, background 80ms;
  user-select: none; display: inline-flex; align-items: center; justify-content: center;
  border-radius: 3px;
}
.cgp-outline :deep(.ol-arrow:hover) { color: var(--ink); background: rgba(26,24,20,0.08); }
.cgp-outline :deep(.ol-arrow-closed) { /* 箭头字符已用 › vs ⌄ 区分 */ }
.cgp-outline :deep(.ol-arrow-placeholder) { flex-shrink: 0; width: 18px; }

/* 类型徽章（小圆角方块 + 字母） */
.cgp-outline :deep(.ol-badge) {
  flex-shrink: 0; width: 16px; height: 16px;
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 10px; font-weight: 700; line-height: 1;
  border-radius: 3px; color: #fff;
}
.cgp-outline :deep(.ol-badge-class)     { background: #3b7dd8; }
.cgp-outline :deep(.ol-badge-interface) { background: #6b9fd8; }
.cgp-outline :deep(.ol-badge-type)      { background: #4a8c5a; }
.cgp-outline :deep(.ol-badge-fn)        { background: #c07820; }
.cgp-outline :deep(.ol-badge-method)    { background: #9060b0; }
.cgp-outline :deep(.ol-badge-call)      { background: transparent; color: var(--ink-4); font-weight: 400; font-size: 11px; }
.cgp-outline :deep(.ol-badge-other)     { background: var(--ink-4); }

/* 标签名 */
.cgp-outline :deep(.ol-label) {
  flex: 1; overflow: hidden; text-overflow: ellipsis;
  color: inherit; padding: 0 2px;
}

/* 行号：右对齐、低调 */
.cgp-outline :deep(.ol-line) {
  margin-left: auto; flex-shrink: 0;
  font-size: 10px; color: var(--ink-4); padding-right: 2px;
  font-variant-numeric: tabular-nums;
  min-width: 32px; text-align: right;
}

/* call 子项样式：稍微降调 */
.cgp-outline :deep(.ol-call) { opacity: 0.75; }
.cgp-outline :deep(.ol-call .ol-label) { font-size: 11px; }
.cgp-outline :deep(.ol-call:hover) { opacity: 1; }

/* 跳转行高亮闪烁 */
.cgp-src-code :deep(.jump-hl) {
  display: inline;
  background: rgba(200, 160, 50, 0.28);
  box-shadow: -4px 0 0 rgba(200, 160, 50, 0.28), 4px 0 0 rgba(200, 160, 50, 0.28);
  animation: jump-flash 2.2s ease-out forwards;
}
@keyframes jump-flash {
  0%   { background: rgba(200, 140, 40, 0.45); box-shadow: -4px 0 0 rgba(200,140,40,0.45), 4px 0 0 rgba(200,140,40,0.45); }
  30%  { background: rgba(200, 160, 50, 0.28); box-shadow: -4px 0 0 rgba(200,160,50,0.28), 4px 0 0 rgba(200,160,50,0.28); }
  100% { background: transparent; box-shadow: none; }
}

.cgp-src-code { flex: 1; overflow: auto; margin: 0; padding: 0; min-width: 0; max-width: none !important; box-sizing: border-box; display: flex; flex-direction: column; }
.cgp-src-code :deep(.code-block) { margin: 0 !important; flex: 1; display: flex !important; flex-direction: column; box-sizing: border-box; width: 100%; }
.cgp-src-code :deep(.code-block-head) { display: none !important; }
.cgp-src-code :deep(.code-header) { display: none !important; }
.cgp-src-code :deep(pre) { margin: 0 !important; flex: 1; box-sizing: border-box; overflow-x: auto; padding: 12px 16px !important; width: 100%; }
.cgp-src-code :deep(code) { white-space: pre; word-break: normal; display: block; }
.cgp-src-empty { display: flex; align-items: center; justify-content: center; flex: 1; color: var(--ink-4); font-family: var(--font-mono); font-size: 11px; font-style: italic; }
</style>
