<template>
  <div class="wiki">
    <!-- Sub-section index — like a chapter's three-part outline -->
    <nav class="wiki-tabs" aria-label="Wiki sections">
      <button
        v-for="st in subTabs" :key="st.key"
        :class="['wiki-tab', { active: subTab === st.key }]"
        @click="switchTab(st.key)"
      >
        <span class="wt-num">§{{ st.num }}</span>
        <span class="wt-name">{{ t(`wiki.${st.key}`) }}</span>
        <span class="wt-count">{{ st.count }}</span>
      </button>
    </nav>

    <!-- Page view -->
    <article v-if="currentPage" class="page">
      <div class="breadcrumb">
        <a class="back" @click="goBack" :title="history.length ? t('wiki.backHistory', { n: history.length }) : t('wiki.backToList')">
          {{ t('common.back') }}
        </a>
        <span class="sep">/</span>
        <span class="bc-cat">{{ currentPage.category }}</span>
        <span class="sep">/</span>
        <span class="bc-name">{{ currentPage.name }}</span>
        <button class="graph-jump" :title="t('wiki.jumpGraph')" @click="jumpToGraph()">{{ t('wiki.jumpGraphLabel') }}</button>
      </div>

      <div v-if="pageContent" class="md-content" v-html="renderedPage" @click="onPageClick"></div>
      <div v-else class="page-loading">
        <span class="loading-glyph">···</span>
        <span>{{ t('wiki.pageLoading') }}</span>
      </div>
    </article>

    <!-- List view -->
    <template v-else>
      <!-- Summaries -->
      <div v-if="subTab === 'summaries'">
        <p v-if="summaries.length === 0" class="empty-hint">
          {{ t('wiki.noSummaries') }}
        </p>
        <ol v-else class="summary-list">
          <li
            v-for="(s, i) in summaries" :key="s"
            class="summary-card"
            @click="viewPage('summaries', s)"
          >
            <span class="sc-num">{{ String(i + 1).padStart(2, '0') }}</span>
            <span class="sc-title">{{ space.titles?.[s] || s }}</span>
            <span class="sc-meta">{{ t('wiki.readSummary') }}</span>
          </li>
        </ol>
      </div>

      <!-- Concepts -->
      <div v-else-if="subTab === 'concepts'">
        <p v-if="concepts.length === 0" class="empty-hint">{{ t('wiki.noConcepts') }}</p>
        <div v-else class="tag-block">
          <div class="eyebrow tag-block-label">{{ t('wiki.conceptCount', { n: concepts.length }) }}</div>
          <div class="tag-set">
            <span v-for="c in concepts" :key="c" class="tag" @click="viewPage('concepts', c)">{{ c }}</span>
          </div>
        </div>
      </div>

      <!-- Entities -->
      <div v-else-if="subTab === 'entities'">
        <p v-if="entities.length === 0" class="empty-hint">{{ t('wiki.noEntities') }}</p>
        <div v-else class="tag-block">
          <div class="eyebrow tag-block-label">{{ t('wiki.entityCount', { n: entities.length }) }}</div>
          <div class="tag-set">
            <span v-for="e in entities" :key="e" class="tag" @click="viewPage('entities', e)">{{ e }}</span>
          </div>
        </div>
      </div>

      <!-- Explorations: chat agent 在对话里临时综合写的对比/专题笔记 -->
      <div v-else-if="subTab === 'explorations'">
        <p v-if="explorations.length === 0" class="empty-hint">{{ t('wiki.noExplorations') }}</p>
        <ol v-else class="summary-list">
          <li
            v-for="(s, i) in explorations" :key="s"
            class="summary-card"
            @click="viewPage('explorations', s)"
          >
            <span class="sc-num">{{ String(i + 1).padStart(2, '0') }}</span>
            <span class="sc-title">{{ space.titles?.[s] || s }}</span>
            <span class="sc-meta">{{ t('wiki.readExploration') }}</span>
          </li>
        </ol>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { renderMarkdownWithWikilinks, handleCodeCopyClick } from '../markdown'
import type { SpaceDetail } from '../types'

const { t } = useI18n()

const props = defineProps<{
  space: SpaceDetail
  initialPage?: { category: string; name: string } | null
}>()
const emit = defineEmits<{
  consumed: []
  focusInGraph: [{ category: string; name: string }]
}>()

type SubTabKey = 'summaries' | 'concepts' | 'entities' | 'explorations'

const subTab = ref<SubTabKey>('summaries')

// 防御：后端旧版本/某些边界 case 下 summaries/concepts/entities 可能是 null（Go 的 nil slice 序列化为 null）。
// 用 computed 归一化成 [] 后再用，避免 `.length` 抛 "Cannot read properties of null"。
const summaries    = computed<string[]>(() => props.space.summaries    || [])
const concepts     = computed<string[]>(() => props.space.concepts     || [])
const entities     = computed<string[]>(() => props.space.entities     || [])
// explorations: chat agent 在对话里写的对比/专题笔记。仅当数据存在时显示该 tab，
// 否则侧栏多出一个空 tab 既丑又徒增噪音。
const explorations = computed<string[]>(() => props.space.explorations || [])

const subTabs = computed(() => {
  const tabs: Array<{ key: SubTabKey; num: string; count: number }> = [
    { key: 'summaries', num: 'i',   count: summaries.value.length },
    { key: 'concepts',  num: 'ii',  count: concepts.value.length  },
    { key: 'entities',  num: 'iii', count: entities.value.length  },
  ]
  if (explorations.value.length > 0) {
    tabs.push({ key: 'explorations', num: 'iv', count: explorations.value.length })
  }
  return tabs
})

const currentPage = ref<{ category: string; name: string } | null>(null)
const pageContent = ref('')
const history = ref<{ category: string; name: string }[]>([])

function switchTab(t: SubTabKey) {
  subTab.value = t
  currentPage.value = null
  history.value = []
}

function goBack() {
  const prev = history.value.pop()
  if (prev) {
    currentPage.value = prev
    pageContent.value = ''
    api.wikiPage(props.space.name, prev.category, prev.name).then(res => {
      pageContent.value = res.content || (res.error ? `> ⚠️ ${res.error}` : '')
    })
  } else {
    currentPage.value = null
  }
}

const renderedPage = computed(() => {
  if (!pageContent.value) return ''
  // 先剥前置 frontmatter（OpenKB 生成的 .md 头部 ---\n...\n---）
  let text = pageContent.value
  const fmMatch = text.match(/^---\n[\s\S]*?\n---\n?/)
  if (fmMatch) text = text.slice(fmMatch[0].length)
  return renderMarkdownWithWikilinks(text)
})

function onPageClick(e: MouseEvent) {
  // 优先处理代码块复制按钮
  if (handleCodeCopyClick(e)) return
  const t = e.target as HTMLElement
  if (t.classList.contains('wikilink')) {
    e.preventDefault()
    const category = t.dataset.category || 'concepts'
    const name = decodeURIComponent(t.dataset.name || '')
    if (name) viewPage(category, name)
  }
}

async function viewPage(category: string, name: string) {
  if (currentPage.value) {
    const top = history.value[history.value.length - 1]
    const cur = currentPage.value
    if (!top || top.category !== cur.category || top.name !== cur.name) {
      history.value.push({ category: cur.category, name: cur.name })
    }
  }
  currentPage.value = { category, name }
  pageContent.value = ''
  const res = await api.wikiPage(props.space.name, category, name)
  pageContent.value = res.content || (res.error ? `> ⚠️ ${res.error}` : '')
}

const validSubTabs = ['summaries', 'concepts', 'entities', 'explorations'] as const
function openInitial(p: { category: string; name: string }) {
  if ((validSubTabs as readonly string[]).includes(p.category)) {
    subTab.value = p.category as typeof validSubTabs[number]
  }
  history.value = []
  currentPage.value = null
  viewPage(p.category, p.name)
  emit('consumed')
}

onMounted(() => {
  if (props.initialPage) openInitial(props.initialPage)
})

watch(() => props.initialPage, p => { if (p) openInitial(p) })

function jumpToGraph() {
  if (!currentPage.value) return
  emit('focusInGraph', { category: currentPage.value.category, name: currentPage.value.name })
}
</script>

<style scoped>
.wiki { max-width: 980px; }

/* ----- Sub-tabs (chapter outline) ------------------------------ */
.wiki-tabs {
  display: flex;
  margin-bottom: 32px;
  border-bottom: 1px solid var(--paper-edge);
}
.wiki-tab {
  appearance: none;
  background: none;
  border: 0;
  border-bottom: 2px solid transparent;
  padding: 12px 0 14px;
  margin-right: 36px;
  cursor: pointer;
  display: flex;
  align-items: baseline;
  gap: 10px;
  color: var(--ink-3);
  font-family: var(--font-body);
  transition: color 150ms ease, border-color 150ms ease;
}
.wiki-tab:hover { color: var(--ink); }
.wiki-tab.active {
  color: var(--ink);
  border-bottom-color: var(--vermilion);
}
.wt-num {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 12px;
  color: var(--ink-4);
  font-variation-settings: "opsz" 14;
  letter-spacing: 0.02em;
}
.wiki-tab.active .wt-num { color: var(--vermilion); }
.wt-name {
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 500;
  letter-spacing: -0.01em;
  font-variation-settings: "opsz" 24, "SOFT" 30;
}
.wt-count {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  letter-spacing: 0.04em;
}
.wiki-tab.active .wt-count { color: var(--ink-2); }

/* ----- Summaries: numbered list, hand-bound feeling ------------ */
.summary-list { list-style: none; padding: 0; margin: 0; }
.summary-card {
  display: grid;
  grid-template-columns: 36px 1fr auto;
  gap: 16px;
  align-items: baseline;
  padding: 18px 4px 18px 0;
  border-bottom: 1px solid var(--paper-edge);
  cursor: pointer;
  transition: padding-left 150ms ease, color 150ms ease;
  position: relative;
}
.summary-card:hover { padding-left: 8px; }
.summary-card:hover .sc-meta { color: var(--vermilion); opacity: 1; }
.summary-card:hover .sc-title { color: var(--vermilion); }
.sc-num {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  letter-spacing: 0.05em;
}
.sc-title {
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: var(--ink);
  font-variation-settings: "opsz" 36, "SOFT" 40;
  transition: color 150ms ease;
}
.sc-meta {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 12px;
  color: var(--ink-4);
  opacity: 0.7;
  font-variation-settings: "opsz" 14;
  transition: color 150ms ease, opacity 150ms ease;
  white-space: nowrap;
}

/* ----- Tag block (concepts/entities) --------------------------- */
.tag-block { padding-top: 4px; }
.tag-block-label { margin-bottom: 14px; }
.tag-set { display: flex; flex-wrap: wrap; gap: 4px 6px; max-width: 880px; }

/* ----- Page header / breadcrumb -------------------------------- */
.page { max-width: var(--measure); }
.breadcrumb {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 26px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--paper-edge);
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-3);
  letter-spacing: 0.02em;
}
.breadcrumb .back {
  font-family: var(--font-body);
  font-size: 12px;
  color: var(--ink);
  cursor: pointer;
  text-decoration: none;
  border-bottom: 1px solid var(--ink);
  padding-bottom: 1px;
  transition: color 150ms ease, border-color 150ms ease;
}
.breadcrumb .back:hover { color: var(--vermilion); border-color: var(--vermilion); }
.breadcrumb .sep { color: var(--ink-4); }
.breadcrumb .bc-cat { color: var(--ink-4); }
.breadcrumb .bc-name { color: var(--ink); font-weight: 500; }
.graph-jump {
  margin-left: auto;
  appearance: none;
  background: none;
  border: 0;
  border-bottom: 1px dashed var(--ink-4);
  font-family: var(--font-body);
  font-size: 11px;
  color: var(--ink-3);
  cursor: pointer;
  padding: 2px 0;
  transition: color 150ms ease, border-color 150ms ease;
}
.graph-jump:hover { color: var(--vermilion); border-bottom-color: var(--vermilion); }

/* ----- Loading ------------------------------------------------- */
.page-loading {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 60px 0;
  color: var(--ink-4);
  font-family: var(--font-display);
  font-style: italic;
  font-size: 16px;
}
.loading-glyph {
  font-family: var(--font-mono);
  font-size: 14px;
  color: var(--vermilion);
  animation: typewriter 0.9s steps(4) infinite;
}
@keyframes typewriter {
  0%   { opacity: 0.2; }
  50%  { opacity: 1;   }
  100% { opacity: 0.2; }
}
</style>
