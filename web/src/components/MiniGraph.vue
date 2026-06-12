<!--
  MiniGraph — 本轮答案的"知识子图"。

  纯 SVG 渲染，不引 cytoscape（太重，且这里只有 3-12 节点，没必要）。
  布局：
    - 中心节点 = 本轮答案（虚拟，"¶" 字符）
    - 周围节点按答案中提到的 wikilinks，圆形等距分布
    - 中心辐射边（极细虚线）

  颜色策略（关键）：
    早期版本只按 category 三色（concepts/indigo, entities/vermilion, summaries/moss），
    但实际场景下一轮答案常常 12 个节点全在同一个 category（比如全是 entities/人名），
    用户反馈"全红，分不清"。
    现在改为：每个节点用 hash(name) 映射到本类别的色板（每类 4 色变体），
    既保留 category 语义（同色系），又让同类内每个节点视觉可区分。

  hover 防抖：
    早期版本对 .mg-node 用 transform: scale(1.18)，但外层 g 已经有
    transform="translate(x,y)" 定位，CSS 的 transform 会**覆盖**它 → 节点瞬间跳到原点。
    现在改用动态 SVG 属性（circle 的 r 在 hover 时变大 + stroke 变粗），不动 transform。

  交互：
    点击外围节点 emit('open', {category, name})，让 QueryView 跳到 WikiView。
-->
<template>
  <div class="mini-graph" ref="root">
    <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`" class="mg-svg">
      <!-- 边：从中心辐射到每个外围节点 -->
      <line
        v-for="(node, i) in nodes" :key="'e' + i"
        :x1="cx" :y1="cy"
        :x2="node.x" :y2="node.y"
        class="mg-edge"
      />
      <!-- 中心节点 -->
      <g class="mg-center">
        <circle :cx="cx" :cy="cy" :r="14" />
        <text :x="cx" :y="cy + 5" text-anchor="middle">¶</text>
      </g>
      <!-- 外围节点：用 g 的 transform 定位，hover 不动 transform 改 r/stroke -->
      <g
        v-for="(node, i) in nodes" :key="'n' + i"
        class="mg-node"
        @click="$emit('open', { category: node.category, name: node.name })"
        @mouseenter="hoverIdx = i"
        @mouseleave="hoverIdx = -1"
        :transform="`translate(${node.x}, ${node.y})`"
      >
        <circle
          :r="hoverIdx === i ? 9 : 7"
          :fill="node.color"
          :stroke="hoverIdx === i ? 'var(--ink)' : 'var(--paper)'"
          :stroke-width="hoverIdx === i ? 2 : 1.5"
        />
        <title>{{ node.category }}/{{ node.name }}</title>
      </g>
    </svg>

    <!-- 节点列表：图下方文字版（短答案不一定能定位节点，文字列表更可读） -->
    <ul class="mg-list">
      <li
        v-for="(l, i) in linksWithColor" :key="i"
        class="mg-list-item"
        @click="$emit('open', { category: l.category, name: l.name })"
        :title="l.category + '/' + l.name"
        @mouseenter="hoverIdx = i"
        @mouseleave="hoverIdx = -1"
        :class="{ active: hoverIdx === i }"
      >
        <span class="mli-dot" :style="{ background: l.color }"></span>
        <span class="mli-name">{{ l.name }}</span>
        <span class="mli-cat">{{ shortCat(l.category) }}</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  links: Array<{ category: string; name: string }>
}>()
defineEmits<{
  open: [{ category: string; name: string }]
}>()

const { t } = useI18n()

// 鼠标悬停的节点 / 列表项下标。SVG 节点和列表项共用同一个 hoverIdx，
// 让用户从文字列表 hover 时图里对应节点也高亮——双向联动定位
const hoverIdx = ref(-1)

// 画布尺寸：随节点数微缩放
const size = computed(() => {
  const n = props.links.length
  if (n <= 4) return 180
  if (n <= 8) return 220
  return 260
})
const cx = computed(() => size.value / 2)
const cy = computed(() => size.value / 2)
const radius = computed(() => size.value * 0.38)

/**
 * 根据 (category, name) 算稳定颜色。
 *
 * 策略：每个 category 有 4 个色板槽，hash(name) % 4 选槽。
 * - concepts: 靛蓝家族（深 → 中 → 浅 → 偏紫）
 * - entities: 朱红家族（深 → 中 → 偏橙 → 偏粉）
 * - summaries: 苔绿家族（深 → 中 → 偏黄 → 偏蓝）
 * - 其他: 灰墨家族
 *
 * 同 category 仍能从色调看出归属，但同类内每个节点都不一样。
 *
 * hash 算 djb2 简单稳定（同一 name 永远同色，不会跨刷新跳）。
 */
const PALETTE: Record<string, string[]> = {
  concepts:     ['#3a4a7a', '#5566a0', '#7d8cc4', '#9b7fb0'],
  entities:     ['#b34a3a', '#d2654a', '#e08762', '#c95a7c'],
  summaries:    ['#5b6f3f', '#7a8a4a', '#a3a85a', '#5a8c7d'],
  // explorations: agent 写的对比/专题笔记。用紫粉家族区分于上面三类知识源。
  explorations: ['#7a4a8c', '#9c5fa8', '#b378b5', '#a890c4'],
  // 代码问答：symbol（函数/类/方法）用靛蓝，file（源文件）用苔绿
  symbol:       ['#3a4a7a', '#5566a0', '#7d8cc4', '#9b7fb0'],
  file:         ['#5b6f3f', '#7a8a4a', '#a3a85a', '#5a8c7d'],
  default:      ['#4a4a4a', '#6a6a6a', '#8a8a8a', '#5a5a6a'],
}
function hashCode(s: string): number {
  let h = 5381
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) & 0xffffffff
  }
  return Math.abs(h)
}
function colorFor(category: string, name: string): string {
  const palette = PALETTE[category] || PALETTE.default
  return palette[hashCode(name) % palette.length]
}

// 给每个 link 预算颜色，列表和图共用同一份（保持视觉一致）
const linksWithColor = computed(() =>
  props.links.map(l => ({ ...l, color: colorFor(l.category, l.name) })),
)

const nodes = computed(() => {
  const n = linksWithColor.value.length
  if (n === 0) return []
  return linksWithColor.value.map((l, i) => {
    const angle = -Math.PI / 2 + (i * 2 * Math.PI) / n
    return {
      ...l,
      x: cx.value + radius.value * Math.cos(angle),
      y: cy.value + radius.value * Math.sin(angle),
    }
  })
})

function shortCat(c: string): string {
  if (c === 'concepts') return t('wiki.concepts')
  if (c === 'entities') return t('wiki.entities')
  if (c === 'summaries') return t('wiki.summaries')
  if (c === 'explorations') return t('wiki.explorations')
  if (c === 'symbol') return t('code.catSymbol')
  if (c === 'file') return t('code.catFile')
  return c
}
</script>

<style scoped>
.mini-graph {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  user-select: none;
}

.mg-svg {
  display: block;
  margin: 0 auto;
}

/* 边：极细虚线，纸张缝线感 */
.mg-edge {
  stroke: var(--paper-edge);
  stroke-width: 1;
  stroke-dasharray: 2 3;
}

/* 中心节点：黑墨实心圆 + 朱红字 */
.mg-center circle {
  fill: var(--ink);
}
.mg-center text {
  font-family: var(--font-display);
  font-size: 14px;
  fill: var(--vermilion);
  font-style: italic;
  pointer-events: none;
}

/*
 * 外围节点：cursor 提示可点击；
 * hover 不再用 CSS transform: scale —— 那样会覆盖 g 的 translate(x,y)
 * 导致节点瞬间跳到画布原点。改成 SVG 属性绑定（r / stroke）做悬停反馈。
 * circle 的 r 用 transition 平滑过渡，看上去依然有"跳出来"的层次。
 */
.mg-node { cursor: pointer; }
.mg-node circle {
  /* 给 r/stroke 加 transition：rounded SVG 属性大多数浏览器都能 transition */
  transition: r 160ms ease, stroke-width 160ms ease, stroke 160ms ease;
}

/* 节点列表：补充图下方文字版 */
.mg-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.mg-list-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 5px 8px;
  cursor: pointer;
  font-family: var(--font-body);
  font-size: 12px;
  color: var(--ink-2);
  border-bottom: 1px dotted transparent;
  transition: color 130ms ease, border-color 130ms ease, padding-left 130ms ease, background 130ms ease;
}
.mg-list-item:hover, .mg-list-item.active {
  color: var(--ink);
  border-bottom-color: var(--paper-edge);
  padding-left: 10px;
  background: var(--paper-2);
}
.mli-dot {
  width: 8px; height: 8px; border-radius: 50%;
  flex-shrink: 0;
  /* 颜色由模板上的 :style="{ background: l.color }" 注入 */
}
.mli-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mli-cat {
  font-family: var(--font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  color: var(--ink-4);
  text-transform: uppercase;
}
</style>
