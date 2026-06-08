<template>
  <div class="graph-wrap">
    <div v-if="loading" class="graph-empty">
      <div class="ge-glyph">···</div>
      <p class="ge-line">{{ t('graph.loading') }}</p>
    </div>
    <div v-else-if="!hasData" class="graph-empty">
      <div class="ge-glyph">¶</div>
      <p class="ge-line">{{ t('graph.empty') }}</p>
    </div>
    <template v-else>
      <div class="graph-filter">
        <button
          v-for="f in filters" :key="f.type"
          :class="['filter-btn', { active: f.visible }]"
          @click="f.visible = !f.visible; applyFilter()"
        >
          <span class="dot" :style="{ background: f.color }"></span>{{ filterLabel(f.type) }}
        </button>
        <div class="filter-search">
          <input v-model="searchQuery" type="text" :placeholder="t('graph.search')" @input="applySearch" />
          <button v-if="searchQuery" class="search-clear" @click="searchQuery = ''; applySearch()">×</button>
        </div>
      </div>
      <div ref="cyContainer" class="cy-container"></div>
      <div class="graph-toolbar">
        <button @click="showAll()" :title="t('graph.tipShowAll')">⊞</button>
        <button @click="cyFit()" :title="t('graph.tipFit')">◳</button>
        <button @click="cyZoomIn()" :title="t('graph.tipZoomIn')">+</button>
        <button @click="cyZoomOut()" :title="t('graph.tipZoomOut')">−</button>
        <button @click="cyRelayout()" :title="t('graph.tipRelayout')">↻</button>
      </div>
      <div v-if="hoverInfo" class="graph-info">
        <div class="gi-eyebrow">{{ hoverInfo.type }}</div>
        <div class="gi-title">{{ hoverInfo.label }}</div>
        <div class="gi-hint">{{ t('graph.hint') }}</div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, reactive, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import cytoscape from 'cytoscape'
import type { Core } from 'cytoscape'
import { api } from '../api'
import type { SpaceDetail, GraphData } from '../types'

const { t } = useI18n()

// type → i18n key 的映射（避免 'entity' + 's' = 'entitys' 的尴尬）
function filterLabel(type: string): string {
  const map: Record<string, string> = {
    concept: 'graph.filterConcepts',
    entity:  'graph.filterEntities',
    doc:     'graph.filterDocs',
  }
  return t(map[type] || type)
}

const props = defineProps<{
  space: SpaceDetail
  focusTarget?: { category: string; name: string } | null
}>()
const cyContainer = ref<HTMLElement>()
const cy = ref<Core | null>(null)
const hoverInfo = ref<{ label: string; type: string } | null>(null)
let locked: string | null = null

const loading = ref(true)
const graphData = ref<GraphData>({ nodes: [], edges: [] })
const hasData = computed(() => graphData.value.nodes.length > 0)

const searchQuery = ref('')

const filters = reactive([
  { type: 'concept', color: '#2d4a6b', visible: true },  // indigo
  { type: 'entity',  color: '#1a1814', visible: true },  // ink
  { type: 'doc',     color: '#c8302e', visible: true },  // vermilion
])

onMounted(async () => {
  // 从后端拉取真实图谱关系（解析 wiki 的 [[wikilink]]）
  try {
    const data = await api.graph(props.space.name)
    graphData.value = { nodes: data.nodes || [], edges: data.edges || [] }
  } catch {
    graphData.value = { nodes: [], edges: [] }
  }
  loading.value = false
  if (hasData.value) {
    nextTick(() => {
      initGraph()
      if (props.focusTarget) {
        focusOnNode(props.focusTarget)
      } else {
        // 默认聚焦：选一个有意义的中心节点（优先 summary，再 doc，再 concept），
        // 给一个"读"的视角而非"乱炖"的全景。cose 已 animate:false 同步出初始位置，
        // 这里立刻接管走 concentric 动画。
        nextTick(() => focusDefault())
      }
    })
  }
})

/**
 * 选默认聚焦节点：优先 summary（因为它是知识入口），其次 doc，再次 concept；
 * 找邻居最多的那个，让 concentric 视图最饱满。
 */
function focusDefault() {
  const g = cy.value
  if (!g) return
  const all = g.nodes()
  if (all.length === 0) return

  const priority = ['summaries', 'doc', 'concept', 'entity']
  let best: any = null
  let bestScore = -1
  all.forEach((n: any) => {
    const id = String(n.data('id') || '')
    const type = String(n.data('nodeType') || '')
    const cat = id.includes('/') ? id.slice(0, id.indexOf('/')) : type
    // priority 越靠前权重越高，邻居越多权重越高
    const pi = priority.indexOf(cat)
    const prioBonus = pi < 0 ? 0 : (priority.length - pi) * 100
    const score = prioBonus + n.degree(false)
    if (score > bestScore) {
      bestScore = score
      best = n
    }
  })
  if (best) relayoutAround(best)
}
onUnmounted(() => {
  cy.value?.destroy(); cy.value = null
})

// 定位到指定节点（从 wiki 跳转过来时使用）：复用 relayoutAround 让目标居中、邻居环绕。
function focusOnNode(target: { category: string; name: string }) {
  const g = cy.value
  if (!g) return
  const id = target.category + '/' + target.name
  const node = g.getElementById(id)
  if (!node || node.length === 0) return
  relayoutAround(node)
  emit('consumed')
}

watch(() => props.focusTarget, t => {
  if (t && cy.value) focusOnNode(t)
})

function initGraph() {
  if (!cyContainer.value) return
  // 禁用浏览器默认右键菜单
  cyContainer.value.addEventListener('contextmenu', e => e.preventDefault())

  // 用后端返回的真实节点与边构建图谱
  const elements: any[] = []
  graphData.value.nodes.forEach(n => elements.push({ data: { id: n.id, label: n.label, nodeType: n.type } }))
  graphData.value.edges.forEach(e => elements.push({ data: { source: e.source, target: e.target } }))

  cy.value = cytoscape({
    container: cyContainer.value, elements,
    style: [
      { selector: 'node', style: {
          label: 'data(label)',
          'font-family': 'IBM Plex Sans, system-ui, sans-serif',
          'font-size': '11px',
          'font-weight': 500,
          color: '#1a1814',
          'text-valign': 'bottom',
          'text-margin-y': 6,
          'text-outline-color': '#f5f0e6',
          'text-outline-width': 3,
          'min-zoomed-font-size': 7,
      }},
      { selector: 'node[nodeType="concept"]', style: {
          'background-color': '#2d4a6b',  // indigo
          width: 22, height: 22,
          'border-width': 1, 'border-color': '#1a1814',
      }},
      { selector: 'node[nodeType="entity"]', style: {
          'background-color': '#1a1814',  // ink
          width: 16, height: 16,
          shape: 'diamond',
          'border-width': 1, 'border-color': '#1a1814',
      }},
      { selector: 'node[nodeType="doc"]', style: {
          'background-color': '#c8302e',  // vermilion
          width: 18, height: 18,
          shape: 'round-rectangle',
          'border-width': 1, 'border-color': '#1a1814',
      }},
      { selector: 'edge', style: {
          width: 1,
          'line-color': 'rgba(26, 24, 20, 0.28)',
          'curve-style': 'bezier',
          opacity: 0.6,
      }},
      { selector: '.highlight', style: {
          'border-width': 2.5,
          'border-color': '#c8302e',
          'font-size': '13px',
          'font-weight': 600,
          'z-index': 999,
      }},
      { selector: '.neighbor', style: {
          'border-color': 'rgba(200, 48, 46, 0.35)',
          'border-width': 1.5,
          opacity: 0.85,
      }},
      { selector: 'edge.highlight', style: {
          'line-color': '#c8302e',
          width: 1.6,
          opacity: 0.85,
      }},
      { selector: '.dimmed', style: { opacity: 0.18 } },
      { selector: 'edge.dimmed', style: { opacity: 0.06 } },
      { selector: '.hidden', style: { display: 'none' } },
    ],
    layout: {
      name: 'cose',
      idealEdgeLength: 140,
      nodeRepulsion: 15000,
      nodeOverlap: 30,
      gravity: 0.2,
      numIter: 400,
      animate: false,    // 同步算完位置，立刻让 focusDefault 接管（concentric 才是用户实际看到的"第一眼"）
      fit: false,        // fit 让 focusDefault 接管
    } as any,
    minZoom: 0.2, maxZoom: 4, wheelSensitivity: 0.3,
  })

  const g = cy.value
  g.on('tap', 'node', e => {
    const node = e.target
    // 再次点击同一节点：取消锁定
    if (locked === node.data('id')) { unlock(); return }
    locked = node.data('id')
    highlight(node)
  })
  // 点击空白处（非节点、非边）：取消锁定
  // cytoscape 标准判断：evt.target === evt.cy 表示点击落在了 core（即背景），不是任何元素
  g.on('tap', evt => {
    if (evt.target === evt.cy) unlock()
  })
  g.on('mouseover', 'node', e => { if (!locked) highlight(e.target); hoverInfo.value = { label: e.target.data('label'), type: e.target.data('nodeType') } })
  g.on('mouseout', 'node', () => { if (!locked) { clearHighlight(); hoverInfo.value = null } })
  // 右键：直接打开对应 wiki 文档
  g.on('cxttap', 'node', e => {
    emitOpenFromNode(e.target)
  })
  // 双击：对该节点 + 一阶邻居跑 concentric 局部布局，让画面专注在这块子图
  g.on('dbltap', 'node', e => {
    relayoutAround(e.target)
  })
}

// 对指定节点做 concentric 布局：节点居中，邻居围一圈，然后 fit 到此子图
function relayoutAround(node: any) {
  const g = cy.value
  if (!g) return
  const id = node.data('id')
  const hood = node.neighborhood().add(node)
  const visibleHood = hood.not('.hidden')

  visibleHood.layout({
    name: 'concentric',
    concentric: (n: any) => (n.id() === id ? 2 : 1),
    levelWidth: () => 1,
    minNodeSpacing: 50,
    spacingFactor: 1.4,
    animate: true,
    animationDuration: 600,
    fit: false,
  } as any).run()

  locked = id
  highlight(node)
  setTimeout(() => {
    g.animate({ fit: { eles: visibleHood, padding: 80 } }, { duration: 500 })
  }, 600)
}

function highlight(node: any) {
  const g = cy.value!
  const hood = node.neighborhood().add(node)
  g.elements().addClass('dimmed').removeClass('highlight neighbor')
  hood.removeClass('dimmed')
  hood.edges().addClass('highlight')
  hood.nodes().not(node).addClass('neighbor')
  node.addClass('highlight')
}
function clearHighlight() { cy.value?.elements().removeClass('dimmed highlight neighbor') }
function unlock() { locked = null; clearHighlight(); hoverInfo.value = null }

function cyFit() { cy.value?.fit(undefined, 40) }
function cyZoomIn() { const g = cy.value; if (g) g.zoom({ level: g.zoom() * 1.3, renderedPosition: { x: g.width() / 2, y: g.height() / 2 } }) }
function cyZoomOut() { const g = cy.value; if (g) g.zoom({ level: g.zoom() * 0.7, renderedPosition: { x: g.width() / 2, y: g.height() / 2 } }) }

// 切到全图：解锁 + 全局 cose 重新布局，让用户看到所有节点的关系网络
function showAll() {
  const g = cy.value
  if (!g) return
  unlock()
  g.elements().removeClass('dimmed highlight neighbor search-match')
  g.layout({
    name: 'cose',
    idealEdgeLength: 160,
    nodeRepulsion: 14000,
    nodeOverlap: 50,
    gravity: 0.18,
    numIter: 500,
    animate: true,
    animationDuration: 800,
    fit: true,
    padding: 60,
  } as any).run()
}

function applyFilter() {
  const g = cy.value
  if (!g) return
  g.nodes().forEach(n => {
    const f = filters.find(x => x.type === n.data('nodeType'))
    n.toggleClass('hidden', !(f?.visible ?? true))
  })
  g.edges().forEach(e => {
    e.toggleClass('hidden', e.source().hasClass('hidden') || e.target().hasClass('hidden'))
  })
}

function applySearch() {
  const g = cy.value
  if (!g) return
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) {
    g.elements().removeClass('dimmed highlight neighbor search-match')
    applyFilter()
    return
  }
  // 先应用类型过滤，确保被过滤的节点保持隐藏
  applyFilter()
  // 在可见节点中搜索匹配
  const visibleNodes = g.nodes().not('.hidden')
  const matched = visibleNodes.filter(n => n.data('label').toLowerCase().includes(q))
  if (matched.length === 0) {
    // 没有匹配：所有可见节点暗化
    visibleNodes.addClass('dimmed').removeClass('highlight neighbor search-match')
    g.edges().not('.hidden').addClass('dimmed').removeClass('highlight')
    return
  }
  // 可见节点全部先暗
  visibleNodes.addClass('dimmed').removeClass('highlight neighbor search-match')
  g.edges().not('.hidden').addClass('dimmed').removeClass('highlight')
  // 匹配节点高亮
  matched.removeClass('dimmed').addClass('highlight search-match')
  // 匹配节点的直接连边（排除隐藏的）高亮
  const connEdges = matched.connectedEdges().not('.hidden')
  connEdges.removeClass('dimmed').addClass('highlight')
  // 直接邻居半亮（排除隐藏的和已匹配的）
  const neighbors = matched.neighborhood().nodes().not('.hidden').not(matched)
  neighbors.removeClass('dimmed').addClass('neighbor')
}

function cyRelayout() {
  const g = cy.value
  if (!g) return

  // 如果有选中节点，只对高亮的节点（选中 + 邻居）重新排列
  if (locked) {
    const lockedNode = g.getElementById(locked)
    if (lockedNode.length) {
      const hood = lockedNode.neighborhood().add(lockedNode)
      const visibleHood = hood.not('.hidden')
      const pos = lockedNode.position()
      visibleHood.layout({
        name: 'cose',
        idealEdgeLength: 120,
        nodeRepulsion: 8000,
        nodeOverlap: 30,
        gravity: 0.3,
        numIter: 300,
        animate: true,
        animationDuration: 800,
        boundingBox: {
          x1: pos.x - 300,
          y1: pos.y - 300,
          x2: pos.x + 300,
          y2: pos.y + 300,
        },
      } as any).run()
      return
    }
  }

  // 没有选中节点时，全局重新布局
  unlock()
  g.layout({
    name: 'cose',
    idealEdgeLength: 180,
    nodeRepulsion: 12000,
    nodeOverlap: 50,
    gravity: 0.15,
    numIter: 400,
    animate: true,
    animationDuration: 1000,
  } as any).run()
}

// 右键菜单操作
const emit = defineEmits<{
  (e: 'openWiki', payload: { category: string; name: string }): void
  (e: 'consumed'): void
}>()

// 节点 id 形如 "summaries/README"，拆出 category 与 slug 让父组件切到 WikiView 并打开
function emitOpenFromNode(node: any) {
  const id = String(node.data('id') || '')
  const slash = id.indexOf('/')
  if (slash < 0) return
  emit('openWiki', { category: id.slice(0, slash), name: id.slice(slash + 1) })
}

</script>

<style scoped>
.graph-wrap {
  position: relative;
  height: calc(100vh - 200px);
  min-height: 540px;
}

.cy-container {
  width: 100%;
  height: 100%;
  background:
    radial-gradient(at 30% 20%, rgba(200, 48, 46, 0.04) 0, transparent 50%),
    radial-gradient(at 75% 70%, rgba(45, 74, 107, 0.05) 0, transparent 50%),
    var(--paper);
  border: 1.5px solid var(--ink);
  /* faint hand-drawn frame inside the canvas */
  box-shadow: inset 0 0 0 1px var(--paper-edge);
}

/* ----- top-left filter & search ------------------------------- */
.graph-filter {
  position: absolute;
  top: 14px; left: 14px;
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
  z-index: 10;
  background: rgba(245, 240, 230, 0.82);
  backdrop-filter: blur(6px);
  padding: 6px 8px;
  border: 1px solid var(--paper-edge);
}
.filter-btn {
  appearance: none;
  border: 1px solid transparent;
  background: transparent;
  padding: 4px 10px;
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--ink-3);
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: color 130ms ease, border-color 130ms ease;
}
.filter-btn:hover { color: var(--ink); }
.filter-btn.active { color: var(--ink); border-color: var(--ink); }
.filter-btn .dot {
  width: 8px; height: 8px;
  display: inline-block;
}

.filter-search { position: relative; }
.filter-search input {
  width: 160px;
  padding: 4px 24px 4px 8px;
  border: 0;
  border-bottom: 1px solid var(--ink-4);
  background: transparent;
  color: var(--ink);
  font-family: var(--font-mono);
  font-size: 11px;
  outline: none;
}
.filter-search input:focus { border-bottom-color: var(--ink); }
.filter-search input::placeholder { color: var(--ink-4); font-style: italic; }
.search-clear {
  position: absolute;
  right: 0; top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--ink-4);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 4px;
  line-height: 1;
}
.search-clear:hover { color: var(--vermilion); }

/* ----- top-right toolbar -------------------------------------- */
.graph-toolbar {
  position: absolute;
  top: 14px; right: 14px;
  display: flex;
  gap: 0;
  z-index: 10;
  background: rgba(245, 240, 230, 0.82);
  backdrop-filter: blur(6px);
  border: 1px solid var(--paper-edge);
}
.graph-toolbar button {
  width: 32px; height: 32px;
  appearance: none;
  background: transparent;
  border: 0;
  border-right: 1px solid var(--paper-edge);
  color: var(--ink-2);
  cursor: pointer;
  font-family: var(--font-display);
  font-size: 16px;
  line-height: 1;
  transition: background 130ms ease, color 130ms ease;
}
.graph-toolbar button:last-child { border-right: 0; }
.graph-toolbar button:hover { background: var(--ink); color: var(--paper); }

/* ----- info card (hover/locked node detail) ------------------- */
.graph-info {
  position: absolute;
  bottom: 18px; right: 18px;
  background: var(--paper);
  border: 1.5px solid var(--ink);
  padding: 14px 18px 12px;
  z-index: 10;
  min-width: 200px;
  box-shadow: 4px 4px 0 rgba(26, 24, 20, 0.12);
}
.gi-eyebrow {
  font-family: var(--font-mono);
  font-size: 9px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-4);
  margin-bottom: 4px;
}
.gi-title {
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: var(--ink);
  font-variation-settings: "opsz" 24, "SOFT" 30;
  line-height: 1.15;
}
.gi-hint {
  margin-top: 8px;
  padding-top: 6px;
  border-top: 1px solid var(--paper-edge);
  font-family: var(--font-display);
  font-style: italic;
  font-size: 11px;
  color: var(--ink-4);
  font-variation-settings: "opsz" 14;
}

/* ----- empty state -------------------------------------------- */
.graph-empty {
  height: calc(100vh - 200px);
  min-height: 540px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border: 1.5px solid var(--ink);
}
.ge-glyph {
  font-family: var(--font-display);
  font-size: 96px;
  color: var(--paper-edge);
  line-height: 1;
  margin-bottom: 18px;
  font-variation-settings: "opsz" 144;
}
.ge-line {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 16px;
  color: var(--ink-3);
  font-variation-settings: "opsz" 24, "SOFT" 80;
}
</style>
