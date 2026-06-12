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
  <div class="cgp-mask" @click.self="$emit('close')">
    <div class="cgp-panel">
      <div class="cgp-head">
        <div>
          <div class="cgp-eyebrow">CODE GRAPH</div>
          <div class="cgp-title">{{ seed }}</div>
        </div>
        <button class="cgp-close" @click="$emit('close')">×</button>
      </div>

      <div class="cgp-body">
        <!-- 左：图 -->
        <div class="cgp-graph">
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
        <div class="cgp-source">
          <div v-if="srcLoading" class="cgp-loading"><span class="loading-glyph">···</span></div>
          <template v-else-if="source && source.found">
            <div class="cgp-src-head">
              <div class="cgp-src-name">{{ source.qualified || source.name }}</div>
              <div class="cgp-src-meta">{{ source.kind }} · {{ source.file }}:{{ source.start_line }}</div>
            </div>
            <div class="cgp-src-code md-content" v-html="renderedSource" @click="onSrcClick"></div>
          </template>
          <div v-else class="cgp-src-empty">{{ t('code.clickNode') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import cytoscape from 'cytoscape'
import type { Core } from 'cytoscape'
import { api } from '../api'
import { renderMarkdownWithWikilinks, handleCodeCopyClick } from '../markdown'

const { t } = useI18n()
const props = defineProps<{ space: string; seed: string }>()
defineEmits<{ close: [] }>()

const cyEl = ref<HTMLElement>()
const loading = ref(false)
const srcLoading = ref(false)
const source = ref<any>(null)
const hoverInfo = ref<{ label: string; kind: string; file?: string; line?: number } | null>(null)
const searchQuery = ref('')
let cy: Core | null = null
let locked: string | null = null
const expanded = new Set<string>()

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
      'background-color': '#5b6f3f', width: 16, height: 16,
      shape: 'round-rectangle', 'border-width': 1, 'border-color': '#1a1814',
  }},
  { selector: 'node[kind="function"], node[kind="method"], node[kind="func"]', style: {
      'background-color': '#2d4a6b', width: 20, height: 20, shape: 'ellipse',
  }},
  { selector: 'node[kind="class"], node[kind="interface"], node[kind="struct"], node[kind="trait"], node[kind="enum"]', style: {
      'background-color': '#1a1814', width: 18, height: 18, shape: 'diamond',
  }},
  { selector: 'node[?center]', style: {
      'background-color': '#c8302e', width: 30, height: 30,
      'font-size': '12px', 'font-weight': 700, 'border-width': 1.5,
  }},
  { selector: 'node:selected', style: { 'border-width': 2.5, 'border-color': '#c8302e' } },
  { selector: 'edge', style: {
      width: 1, 'line-color': 'rgba(26,24,20,0.28)',
      'target-arrow-shape': 'triangle', 'target-arrow-color': 'rgba(26,24,20,0.4)',
      'arrow-scale': 0.7, 'curve-style': 'bezier', opacity: 0.6,
  }},
  { selector: 'edge[type="caller"]', style: { 'line-color': '#7a8a4a', 'target-arrow-color': '#7a8a4a' } },
  { selector: 'edge[type="callee"]', style: { 'line-color': '#5566a0', 'target-arrow-color': '#5566a0' } },
  { selector: '.highlight', style: { 'border-width': 2.5, 'border-color': '#c8302e', 'font-size': '12px', 'font-weight': 600, 'z-index': 999 } },
  { selector: '.neighbor', style: { 'border-color': 'rgba(200,48,46,0.4)', 'border-width': 1.5, opacity: 0.9 } },
  { selector: 'edge.highlight', style: { 'line-color': '#c8302e', 'target-arrow-color': '#c8302e', width: 1.6, opacity: 0.9 } },
  { selector: '.search-match', style: { 'border-width': 2.5, 'border-color': '#c8302e' } },
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
        toAdd.push({ group: 'nodes', data: { id: n.id, label: n.label, kind: n.kind, file: n.file, line: n.line, center: n.is_center ? 1 : undefined } })
      }
      willHave.add(n.id)
    }
    const edgeSeen = new Set<string>()
    for (const e of res.edges || []) {
      const eid = `${e.source}->${e.target}`
      if (edgeSeen.has(eid) || cy.getElementById(eid).length > 0) continue
      if (!willHave.has(e.source) || !willHave.has(e.target)) continue // 端点缺失则跳过，避免整批失败
      edgeSeen.add(eid)
      toAdd.push({ group: 'edges', data: { id: eid, source: e.source, target: e.target, type: e.type } })
    }
    cy.add(toAdd)
    refreshGroups()
  } catch {
    // ignore
  } finally {
    if (isSeed) loading.value = false
  }
}

// 单击：聚焦该节点 = 锁定高亮 + concentric 围绕它 + 拉源码（不 toggle 解锁，解锁交给点空白）
async function focusNode(node: any) {
  const id = node.id()
  locked = id
  relayoutAround(node)
  hoverInfo.value = nodeInfo(node)
  loadSource(id)
}

// 双击：先展开邻居（已展开则跳过 API），再 concentric 聚焦
async function expandNode(symbol: string) {
  await fetchNeighbors(symbol)
  if (!cy) return
  const node = cy.getElementById(symbol)
  if (node && node.length) { locked = symbol; relayoutAround(node); hoverInfo.value = nodeInfo(node) }
}

async function loadSource(symbol: string) {
  srcLoading.value = true
  source.value = null
  try {
    source.value = await api.codeSymbolSource(props.space, symbol)
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
  highlight(node)
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
  if (seedNode && seedNode.length) { locked = props.seed; relayoutAround(seedNode); hoverInfo.value = nodeInfo(seedNode) }
  loadSource(props.seed)
})

onUnmounted(() => { if (cy) { cy.destroy(); cy = null } })
</script>

<style scoped>
.cgp-mask {
  position: fixed; inset: 0; z-index: 200;
  background: rgba(20, 18, 15, 0.55);
  display: flex; align-items: center; justify-content: center;
  padding: 4vh 4vw;
}
.cgp-panel {
  width: 100%; max-width: 1100px; height: 88vh;
  background: var(--paper); border: 1.5px solid var(--ink);
  box-shadow: 10px 10px 0 rgba(0,0,0,0.18);
  display: flex; flex-direction: column;
}
.cgp-head { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; border-bottom: 1.5px solid var(--ink); background: #171411; color: #f5efe5; }
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
.cgp-legend .lg { display: inline-block; width: 10px; height: 2px; margin-right: 4px; vertical-align: middle; }
.lg-caller { background: #7a8a4a; }
.lg-callee { background: #5566a0; }
.cgp-tip { color: var(--ink-4); }
.cgp-source { flex: 1; display: flex; flex-direction: column; min-width: 0; background: var(--paper-2); }
.cgp-src-head { padding: 12px 16px; border-bottom: 1px solid var(--paper-edge); }
.cgp-src-name { font-family: var(--font-mono); font-size: 13px; color: var(--ink); font-weight: 600; word-break: break-all; }
.cgp-src-meta { font-family: var(--font-mono); font-size: 10px; color: var(--ink-4); margin-top: 3px; word-break: break-all; }
.cgp-src-code { flex: 1; overflow: auto; margin: 0; padding: 14px 16px; }
.cgp-src-code :deep(.code-block) { margin: 0; }
.cgp-src-code :deep(pre) { margin: 0; }
.cgp-src-empty { margin: auto; color: var(--ink-4); font-family: var(--font-mono); font-size: 12px; }
</style>
