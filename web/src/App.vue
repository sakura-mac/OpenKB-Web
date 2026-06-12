<template>
  <div class="app">
    <aside class="sidebar">
      <div class="sidebar-header">
        <div class="masthead">
          <div class="masthead-mark">{{ t('brand.mark') }}</div>
          <div class="masthead-rule"></div>
          <div class="masthead-sub" v-html="brandSubHtml"></div>
        </div>
        <p class="masthead-tagline">{{ t('brand.tagline') }}</p>
      </div>

      <nav class="space-list">
        <div class="space-list-label">{{ t('sidebar.spaces') }} · {{ allSpaces.length }}</div>
        <ol class="space-items">
          <li
            v-for="(s, i) in allSpaces" :key="s.kind + ':' + s.name"
            :class="['space-item', {
              code: s.kind === 'code',
              active: !manageMode && currentSpaceName === s.name && currentKind === s.kind,
              selected: manageMode && selectedSpaces.has(s.name),
              manage: manageMode,
            }]"
            @click="manageMode ? toggleSelect(s.name) : selectAnySpace(s)"
          >
            <span class="space-num">{{ String(i + 1).padStart(2, '0') }}</span>
            <input
              v-if="manageMode" type="checkbox"
              :checked="selectedSpaces.has(s.name)"
              class="space-checkbox" @click.stop="toggleSelect(s.name)"
            />
            <span class="space-kind">{{ s.kind === 'code' ? '⌘' : '¶' }}</span>
            <span class="space-name">{{ s.name }}</span>
            <!--
              空间统计数字：仅 hover 浮现「文档/产物面板」，不可点击。
              点击/选中走外面的 li @click 进入 wiki，本身不再触发任何 action。
            -->
            <span
              class="space-meta"
              @mouseenter="showDocsPopover(s.name, $event)"
              @mouseleave="scheduleHideDocsPopover()"
            >{{ s.kind === 'code' ? `${s.files || 0} files` : t('sidebar.docsConcepts', { docs: s.docs, concepts: s.concepts }) }}</span>
          </li>
          <li v-if="spaces.length === 0" class="empty-hint" style="padding:20px 0">
            {{ t('sidebar.noSpaces') }}
          </li>
        </ol>
      </nav>

      <div class="sidebar-footer">
        <template v-if="!manageMode">
          <button class="btn btn-primary" style="flex:1" @click="openCreate('kb')">{{ t('sidebar.newSpace') }}</button>
          <button class="btn btn-ghost btn-sm" @click="openCreate('code')">⌘ Code</button>
          <button class="btn btn-ghost btn-sm" @click="manageMode = true">{{ t('sidebar.edit') }}</button>
        </template>
        <template v-else>
          <button class="btn btn-danger" style="flex:1" :disabled="selectedSpaces.size === 0" @click="doDeleteSelected">
            {{ t('sidebar.delete') }}{{ selectedSpaces.size > 0 ? ` (${selectedSpaces.size})` : '' }}
          </button>
          <button class="btn btn-ghost btn-sm" @click="manageMode = false; selectedSpaces.clear()">{{ t('sidebar.done') }}</button>
        </template>
      </div>
    </aside>

    <main class="main-area">
      <header class="topbar" v-if="currentSpace || currentCodeSpace">
        <div class="topbar-headline">
          <div class="eyebrow">{{ t('topbar.eyebrow') }}</div>
          <h1 class="display topbar-title">{{ currentKind === 'code' ? currentCodeSpace?.name : currentSpace?.name }}</h1>
        </div>
        <nav class="tabs" aria-label="Sections">
          <button
            v-for="t2 in visibleTabs" :key="t2.key"
            :class="['tab', { active: tab === t2.key }]"
            @click="tab = t2.key"
          >
            <span class="tab-num">{{ t2.num }}</span>
            <span class="tab-label">{{ t(`tabs.${t2.key}`) }}</span>
          </button>
          <!-- 设置入口：齿轮图标。点开抽屉式 SettingsPanel -->
          <button
            class="settings-btn"
            :title="t('settings.title')"
            @click="showSettings = true"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
          </button>
        </nav>
      </header>
      <header class="topbar topbar-empty" v-else>
        <div class="topbar-headline">
          <div class="eyebrow">{{ t('topbar.welcomeEyebrow') }}</div>
          <h1 class="display topbar-title" style="font-style:italic">{{ t('topbar.noSpace') }}</h1>
        </div>
        <nav class="tabs" aria-label="Sections">
          <button
            class="settings-btn"
            :title="t('settings.title')"
            @click="showSettings = true"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
          </button>
        </nav>
      </header>

      <section class="content">
        <div v-if="!currentSpace && !currentCodeSpace" class="empty-state">
          <div class="empty-glyph">¶</div>
          <p class="empty-line">{{ t('empty.pickLine1') }}</p>
          <p class="empty-line">{{ t('empty.pickLine2') }}</p>
        </div>
        <CodeView
          v-else-if="tab === 'code' && currentCodeSpace"
          :key="'code:' + currentCodeSpace.name"
          :space="currentCodeSpace"
          @refresh="refreshSpace"
        />
        <WikiView
          v-else-if="tab === 'wiki' && currentSpace"
          :key="'wiki:' + currentSpace.name"
          :space="currentSpace"
          :initial-page="pendingWikiPage"
          @consumed="pendingWikiPage = null"
          @focus-in-graph="onFocusInGraph"
        />
        <GraphView
          v-else-if="tab === 'graph' && currentSpace"
          :key="'graph:' + currentSpace.name"
          :space="currentSpace"
          :focus-target="pendingGraphFocus"
          @open-wiki="onOpenWiki"
          @consumed="pendingGraphFocus = null"
        />
        <QueryView
          v-else-if="tab === 'query' && currentSpace"
          :key="'query:' + currentSpace.name"
          :space="currentSpace"
          @open-wiki="onOpenWiki"
        />
      </section>
    </main>

    <!--
      文档/产物 popover：浮在侧边栏右侧，hover 在某个空间的统计数字上才出现。
      内部用 DocsView，但它需要的是 SpaceDetail 而不是 SpaceInfo——
      所以 popover 触发时会按需 fetch detail，并保留为 popoverSpace。
    -->
    <div
      v-if="popover.spaceName && popover.detail"
      class="docs-popover"
      :style="{ top: popover.top + 'px', left: popover.left + 'px' }"
      @mouseenter="cancelHideDocsPopover()"
      @mouseleave="scheduleHideDocsPopover()"
    >
      <div class="dp-arrow"></div>
      <div class="dp-eyebrow">{{ t('topbar.eyebrow') }} · {{ popover.spaceName }}</div>
      <DocsView
        :key="'docs-pop:' + popover.spaceName"
        :space="popover.detail"
        @refresh="onPopoverRefresh"
      />
    </div>

    <div v-if="uploadTasks.length" class="task-strip">
      <div v-for="task in uploadTasks" :key="task.id" :class="['task-row', task.status]">
        <span class="task-marker">
          <span v-if="task.status === 'uploading'" class="task-spinner">···</span>
          <span v-else-if="task.status === 'done'" class="task-icon-done">✓</span>
          <span v-else class="task-icon-err">✕</span>
        </span>
        <span class="task-message">{{ task.message }}</span>
      </div>
    </div>

    <SettingsPanel v-if="showSettings" @close="showSettings = false" />

    <!--
      首次启动遮罩：一直 mount，组件内部根据 /api/bootstrap/status 决定可见性。
      ready 时 emit('done') 后我们标记为 ready，组件自身 visible=false 撤掉遮罩。
    -->
    <BootstrapOverlay
      v-if="!bootstrapReady"
      @done="bootstrapReady = true"
      @open-settings="showSettings = true"
    />

    <div v-if="showCreate" class="modal-overlay" @click.self="showCreate = false">
      <div class="modal" style="width:560px">
        <div class="eyebrow" style="margin-bottom:6px">{{ t('create.formNo') }}</div>
        <h3>{{ t('create.title') }}</h3>

        <div class="form-group">
          <label>{{ t('create.nameLabel') }}</label>
          <input
            v-model="newName"
            type="text"
            :placeholder="t('create.namePlaceholder')"
            @keydown.enter="doCreate"
          />
        </div>
        <div class="form-group">
          <label>{{ t('create.pathLabel') }}</label>
          <div style="display:flex; gap:8px; align-items:flex-end">
            <input v-model="newPath" type="text" :placeholder="createKind === 'code' ? '选择代码仓库目录（必选）' : t('create.pathPlaceholder')" readonly style="flex:1" />
            <button class="btn btn-sm" @click="toggleCreateBrowser()" :title="t('common.browse')">{{ t('common.browse') }}</button>
          </div>
        </div>

        <div v-if="showCreateBrowser" style="margin: 8px 0 16px">
          <div class="browser-toolbar">
            <button class="btn btn-ghost btn-sm" @click="createBrowseParent()">{{ t('common.up') }}</button>
            <span class="browser-path" :title="createBrowserPath">{{ createBrowserPath }}</span>
            <button class="btn btn-sm btn-primary" @click="selectCreateDir()">{{ t('common.use') }}</button>
          </div>
          <div class="browser-list">
            <div
              v-for="item in createBrowserItems" :key="item.name"
              class="browser-item"
              @click="createBrowseTo(createBrowserPath + '/' + item.name)"
            >
              <span class="bi-name">{{ item.name }}</span>
              <span class="bi-arrow">→</span>
            </div>
            <div v-if="createBrowserItems.length === 0" class="empty-hint" style="padding:18px 0">{{ t('common.empty') }}</div>
          </div>
        </div>

        <p v-if="createError" class="error-text">{{ createError }}</p>

        <div class="modal-actions">
          <button class="btn btn-ghost" @click="showCreate = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="creating" @click="doCreate">
            {{ creating ? t('common.creating') : (createKind === 'code' ? '创建代码空间' : t('common.create')) }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from './api'
import { useUpload } from './composables/useUpload'
import type { SpaceInfo, SpaceDetail, CodeSpaceInfo } from './types'
import WikiView from './views/WikiView.vue'
import DocsView from './views/DocsView.vue'
import GraphView from './views/GraphView.vue'
import QueryView from './views/QueryView.vue'
import CodeView from './views/CodeView.vue'
import SettingsPanel from './views/SettingsPanel.vue'
import BootstrapOverlay from './views/BootstrapOverlay.vue'

const { t } = useI18n()

// brand.sub 用 \n 表示换行（zh "知识工坊" 一行；en "Knowledge\nWorkshop" 两行）
const brandSubHtml = computed(() => t('brand.sub').replace(/\n/g, '<br/>'))

const spaces = ref<SpaceInfo[]>([])
const codeSpaces = ref<CodeSpaceInfo[]>([])
const allSpaces = computed(() => [
  ...spaces.value.map(s => ({ ...s, kind: 'kb' as const })),
  ...codeSpaces.value.map(s => ({ ...s, docs: 0, concepts: 0 })),
])
const currentSpace = ref<SpaceDetail | null>(null)
const currentCodeSpace = ref<CodeSpaceInfo | null>(null)
const currentKind = ref<'kb' | 'code'>('kb')
const currentSpaceName = computed(() => currentKind.value === 'code' ? currentCodeSpace.value?.name : currentSpace.value?.name)
// tab 删除 docs：文档/产物 移到侧边栏 hover popover；主区只剩 知识/图谱/问答 三个章节。
const tab = ref<'wiki' | 'graph' | 'query' | 'code'>('wiki')
const tabs = [
  { key: 'wiki',  num: 'I'   },
  { key: 'graph', num: 'II'  },
  { key: 'query', num: 'III' },
  { key: 'code',  num: '⌘'   },
] as const
const visibleTabs = computed(() => currentKind.value === 'code'
  ? tabs.filter(t => t.key === 'code')
  : tabs.filter(t => t.key !== 'code'),
)

const manageMode = ref(false)
const selectedSpaces = ref<Set<string>>(new Set())
const showCreate = ref(false)
const showSettings = ref(false)
// 首次启动初始化引导：BootstrapOverlay 内部轮询 /api/bootstrap/status，
// ready 时 emit('done') → 这里置 true 撤掉遮罩。
const bootstrapReady = ref(false)
const createKind = ref<'kb' | 'code'>('kb')
const newName = ref('')
const newPath = ref('')
const createError = ref('')
const creating = ref(false)
const showCreateBrowser = ref(false)
const createBrowserPath = ref('')
const createBrowserItems = ref<{ name: string; is_dir: boolean; size: number }[]>([])

const pendingWikiPage = ref<{ category: string; name: string } | null>(null)
const pendingGraphFocus = ref<{ category: string; name: string } | null>(null)

function onOpenWiki(payload: { category: string; name: string }) {
  pendingWikiPage.value = payload
  tab.value = 'wiki'
}
function onFocusInGraph(payload: { category: string; name: string }) {
  pendingGraphFocus.value = payload
  tab.value = 'graph'
}

const { tasks: uploadTasks, startUpload, finishUpload } = useUpload()

async function loadSpaces() {
  spaces.value = await api.listSpaces()
  codeSpaces.value = await api.listCodeSpaces()
}
async function selectSpace(name: string) {
  const detail = await api.spaceDetail(name)
  if ('error' in detail) return
  currentSpace.value = detail as SpaceDetail
  currentCodeSpace.value = null
  currentKind.value = 'kb'
  if (tab.value === 'code') tab.value = 'wiki'
}
async function selectCodeSpace(name: string) {
  const detail = await api.codeSpaceDetail(name)
  if ('error' in detail) return
  currentCodeSpace.value = detail as CodeSpaceInfo
  currentSpace.value = null
  currentKind.value = 'code'
  tab.value = 'code'
}
function selectAnySpace(s: SpaceInfo | (CodeSpaceInfo & { docs: number; concepts: number })) {
  if (s.kind === 'code') selectCodeSpace(s.name)
  else selectSpace(s.name)
}
async function refreshSpace() {
  if (currentKind.value === 'code' && currentCodeSpace.value) await selectCodeSpace(currentCodeSpace.value.name)
  else if (currentSpace.value) await selectSpace(currentSpace.value.name)
}

let refreshTimer: number | null = null
watch(
  () => uploadTasks.value.some(t => t.status === 'uploading'),
  (hasRunning) => {
    if (hasRunning && !refreshTimer) {
      refreshTimer = window.setInterval(() => { refreshSpace(); loadSpaces(); refreshPopover() }, 5000)
    } else if (!hasRunning && refreshTimer) {
      window.clearInterval(refreshTimer)
      refreshTimer = null
    }
  },
)

// ============================================================
// Sidebar hover popover：文档/产物面板
// ============================================================
//
// 设计意图：用户点击空间名只表达「我要看这个 KB 的知识」，会进 wiki tab。
// 文档管理（上传/删除/URL 抓取/Deck）是「副功能」，放在侧边栏数字 hover
// 弹出的浮层里——既不占主区空间，又不跟知识阅读流混在一起。
//
// hover 进入数字 → 200ms 防抖后 fetch detail 并显示 popover
// 离开数字、离开 popover 自身 → 250ms 后隐藏（给鼠标移动的容差）
// 点击空间名（li 主体）→ 立即关闭 popover 并切到 wiki tab
const popover = reactive({
  spaceName: '' as string,
  detail: null as SpaceDetail | null,
  top: 0,
  left: 0,
})
let popoverShowTimer: number | null = null
let popoverHideTimer: number | null = null

async function showDocsPopover(name: string, ev: MouseEvent) {
  cancelHideDocsPopover()
  if (popover.spaceName === name && popover.detail) return
  if (popoverShowTimer) window.clearTimeout(popoverShowTimer)

  // 立刻定位（用 li 元素的位置，对齐到右侧；不等 fetch 完）
  const li = (ev.currentTarget as HTMLElement)?.closest('li') as HTMLElement | null
  if (li) {
    const rect = li.getBoundingClientRect()
    popover.top = Math.max(8, rect.top - 8)
    popover.left = rect.right + 8
  }

  popoverShowTimer = window.setTimeout(async () => {
    try {
      const detail = await api.spaceDetail(name)
      if (!('error' in detail)) {
        popover.spaceName = name
        popover.detail = detail as SpaceDetail
      }
    } catch { /* keep silent */ }
  }, 200)
}

function scheduleHideDocsPopover() {
  if (popoverShowTimer) {
    window.clearTimeout(popoverShowTimer)
    popoverShowTimer = null
  }
  if (popoverHideTimer) window.clearTimeout(popoverHideTimer)
  popoverHideTimer = window.setTimeout(() => {
    popover.spaceName = ''
    popover.detail = null
  }, 250)
}

function cancelHideDocsPopover() {
  if (popoverHideTimer) {
    window.clearTimeout(popoverHideTimer)
    popoverHideTimer = null
  }
}

async function refreshPopover() {
  if (!popover.spaceName) return
  try {
    const detail = await api.spaceDetail(popover.spaceName)
    if (!('error' in detail)) popover.detail = detail as SpaceDetail
  } catch { /* ignore */ }
}

// popover 中的 DocsView 触发 refresh：同步刷新 popover 自己 + 主区
async function onPopoverRefresh() {
  await refreshPopover()
  await loadSpaces()
  if (currentSpace.value && popover.spaceName === currentSpace.value.name) {
    await refreshSpace()
  }
}

function openCreate(kind: 'kb' | 'code') {
  createKind.value = kind
  newName.value = ''
  newPath.value = ''
  createError.value = ''
  showCreate.value = true
}

async function toggleCreateBrowser() {
  showCreateBrowser.value = !showCreateBrowser.value
  if (showCreateBrowser.value) await createBrowseTo('')
}
async function createBrowseTo(path: string) {
  const res = await api.browse(path)
  if (res.path) {
    createBrowserPath.value = res.path
    createBrowserItems.value = (res.items || []).filter(i => i.is_dir)
  }
}
function createBrowseParent() {
  const parts = createBrowserPath.value.split('/')
  parts.pop()
  createBrowseTo(parts.join('/') || '/')
}
function selectCreateDir() {
  newPath.value = createBrowserPath.value
  showCreateBrowser.value = false
}

async function doCreate() {
  createError.value = ''
  const name = newName.value.trim()
  if (!name) { createError.value = t('create.errEmpty'); return }
  if (!/^[a-zA-Z0-9_-]+$/.test(name)) { createError.value = t('create.errInvalidName'); return }
  if (createKind.value === 'code' && !newPath.value) { createError.value = '请选择代码仓库目录'; return }
  creating.value = true
  const res = createKind.value === 'code'
    ? await api.createCodeSpace(name, newPath.value)
    : await api.createSpace(name, newPath.value || undefined)
  creating.value = false
  if (!res.success) {
    createError.value = res.error || t('create.initFail')
    return
  }

  showCreate.value = false
  newName.value = ''
  newPath.value = ''
  showCreateBrowser.value = false

  const taskId = startUpload(1, t('create.initialising', { name }))

  const pollStatus = () => {
    let count = 0
    const tick = () => {
      if (count++ >= 120) {
        finishUpload(taskId, false, t('create.timeoutMsg'))
        return
      }
      setTimeout(async () => {
        try {
          if (createKind.value === 'code') {
            const ts = await api.getTask((res as any).task_id)
            if (ts.status === 'done') {
              finishUpload(taskId, true, '代码索引完成')
              await loadSpaces()
              await selectCodeSpace(name)
              return
            }
            if (ts.status === 'error') {
              finishUpload(taskId, false, ts.message || '代码索引失败')
              await loadSpaces()
              return
            }
          } else {
            const st = await api.spaceStatus(name)
            if (st.status === 'ready') {
              finishUpload(taskId, true, t('create.ready', { name }))
              await loadSpaces()
              if (!currentSpace.value) await selectSpace(name)
              return
            }
            if (st.status === 'error') {
              finishUpload(taskId, false, st.error || t('create.initFail'))
              await loadSpaces()
              return
            }
          }
        } catch { /* keep retrying */ }
        tick()
      }, 1000)
    }
    tick()
  }
  pollStatus()
}

function toggleSelect(name: string) {
  const s = selectedSpaces.value
  if (s.has(name)) s.delete(name)
  else s.add(name)
  selectedSpaces.value = new Set(s)
}

async function doDeleteSelected() {
  const names = [...selectedSpaces.value]
  if (names.length === 0) return
  const msg = names.length === 1
    ? t('create.deleteConfirmOne', { name: names[0] })
    : t('create.deleteConfirmMany', { count: names.length, names: names.join('、') })
  if (!confirm(msg + t('create.deleteWarn'))) return

  for (const name of names) {
    const code = codeSpaces.value.find(s => s.name === name)
    if (code) await api.deleteCodeSpace(name)
    else await api.deleteSpace(name)
  }

  if (currentSpace.value && selectedSpaces.value.has(currentSpace.value.name)) currentSpace.value = null
  if (currentCodeSpace.value && selectedSpaces.value.has(currentCodeSpace.value.name)) currentCodeSpace.value = null
  selectedSpaces.value.clear()
  manageMode.value = false
  await loadSpaces()
}

onMounted(loadSpaces)
</script>

<style scoped>
/* =================================================================
 * Sidebar — TOC of a printed book
 * ================================================================= */

.sidebar {
  width: 280px;
  flex-shrink: 0;
  background: var(--paper-2);
  border-right: 1.5px solid var(--ink);
  display: flex; flex-direction: column;
  position: relative;
}
/* a vertical "spine" hint */
.sidebar::after {
  content: "";
  position: absolute;
  top: 0; bottom: 0; right: -1px;
  width: 4px;
  background: linear-gradient(to right, transparent, rgba(0,0,0,0.08));
  pointer-events: none;
}

.sidebar-header {
  padding: 28px 24px 22px;
  border-bottom: 1px solid var(--paper-edge);
}

.masthead {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 12px;
  align-items: center;
}
.masthead-mark {
  font-family: var(--font-display);
  font-weight: 600;
  font-size: 38px;
  line-height: 1;
  letter-spacing: -0.04em;
  color: var(--ink);
  font-variation-settings: "opsz" 144, "SOFT" 0, "WONK" 1;
}
.masthead-rule {
  display: none;
}
.masthead-sub {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 14px;
  line-height: 1.05;
  color: var(--ink-2);
  font-variation-settings: "opsz" 36, "SOFT" 50;
}
.masthead-tagline {
  margin-top: 14px;
  font-family: var(--font-display);
  font-style: italic;
  font-size: 12px;
  color: var(--ink-3);
  line-height: 1.4;
  font-variation-settings: "opsz" 14;
}

.lang-switch,
.lang-btn,
.lang-sep {
  /* 按 IP/浏览器自动选语言后，主动入口已移除；保留类名占位但设为隐藏。
     不直接删类是因为 Tab 顺序/历史 build 的 CSS 选择器命中可能引入兼容问题。 */
  display: none !important;
}

.space-list {
  flex: 1;
  overflow-y: auto;
  padding: 22px 18px 16px;
}
.space-list-label {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-4);
  margin-bottom: 10px;
  padding-left: 6px;
}
.space-items { list-style: none; padding: 0; margin: 0; }

.space-item {
  display: grid;
  grid-template-columns: 24px 1fr auto;
  align-items: baseline;
  gap: 10px;
  padding: 10px 8px 11px;
  cursor: pointer;
  border-bottom: 1px solid var(--paper-edge);
  transition: padding 150ms ease, background 150ms ease;
  position: relative;
}
.space-item:hover { background: rgba(255, 252, 244, 0.6); padding-left: 12px; }
.space-item.active {
  background: var(--ink);
  color: var(--paper);
  border-bottom-color: var(--ink);
}
.space-item.active::before {
  content: "▶";
  position: absolute;
  left: -20px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--vermilion);
  font-size: 11px;
}
.space-item.selected { background: rgba(200, 48, 46, 0.08); border-bottom-color: var(--vermilion); }
.space-item.manage { padding-left: 4px; grid-template-columns: 18px 24px 1fr auto; }

.space-num {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--ink-4);
  letter-spacing: 0.05em;
}
.space-item.active .space-num { color: rgba(245, 240, 230, 0.5); }

.space-name {
  font-family: var(--font-display);
  font-size: 17px;
  font-weight: 500;
  letter-spacing: -0.01em;
  font-variation-settings: "opsz" 24, "SOFT" 30;
  line-height: 1.1;
}
.space-meta {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--ink-4);
  letter-spacing: 0.04em;
  white-space: nowrap;
  /* 这是个"可悬停徽章"——hover 浮现 docs popover，但不应被误以为是按钮。
     因此 cursor 保持 default，只用极轻的下划线 + 颜色变化暗示可交互。 */
  cursor: default;
  border-bottom: 1px dotted transparent;
  padding: 1px 0;
  transition: color 150ms ease, border-color 150ms ease;
}
.space-item:hover .space-meta { color: var(--ink-3); border-bottom-color: var(--paper-edge); }
.space-meta:hover { color: var(--ink) !important; border-bottom-color: var(--vermilion) !important; }
.space-item.active .space-meta { color: rgba(245, 240, 230, 0.55); }

.space-checkbox {
  width: 14px; height: 14px;
  accent-color: var(--vermilion);
  cursor: pointer;
}

.sidebar-footer {
  padding: 14px 16px 18px;
  border-top: 1.5px solid var(--ink);
  display: flex;
  gap: 8px;
  background: var(--paper);
}

/* =================================================================
 * Main area
 * ================================================================= */

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--paper);
}

.topbar {
  padding: 32px 56px 22px;
  border-bottom: 1.5px solid var(--ink);
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 32px;
  background:
    linear-gradient(180deg, rgba(255,252,244,0.7), transparent);
}
.topbar-empty { padding-bottom: 28px; }
.topbar-headline { min-width: 0; }
.topbar-title {
  font-size: 42px;
  font-weight: 400;
  font-variation-settings: "opsz" 60, "SOFT" 50, "WONK" 0;
  margin-top: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* tabs = chapter index */
.tabs {
  display: flex;
  gap: 0;
  flex-shrink: 0;
}
.tab {
  appearance: none;
  background: none;
  border: 0;
  border-bottom: 2px solid transparent;
  padding: 8px 16px 10px;
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--ink-3);
  cursor: pointer;
  display: inline-flex;
  align-items: baseline;
  gap: 7px;
  transition: color 150ms ease, border-color 150ms ease;
  letter-spacing: 0.01em;
}
.tab:hover { color: var(--ink); }
.tab.active {
  color: var(--ink);
  border-bottom-color: var(--vermilion);
}

/* 齿轮按钮：放在 tab 行尾，和 tab 同行不同色阶。
   默认灰，hover 转朱红。点击打开 SettingsPanel 抽屉。 */
.settings-btn {
  appearance: none;
  background: none;
  border: 0;
  margin-left: 8px;
  padding: 8px 12px;
  color: var(--ink-4);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  border-bottom: 2px solid transparent;
  transition: color 180ms ease, transform 180ms ease;
}
.settings-btn:hover {
  color: var(--vermilion);
  transform: rotate(40deg);
}
.settings-btn svg { display: block; }
.tab-num {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 11px;
  color: var(--ink-4);
  font-variation-settings: "opsz" 14;
}
.tab.active .tab-num { color: var(--vermilion); }
.tab-label { font-weight: 500; }

.content {
  flex: 1;
  overflow-y: auto;
  padding: 36px 56px 80px;
}

/* empty state */
.empty-state {
  margin: 12vh auto 0;
  text-align: center;
  max-width: 520px;
}
.empty-glyph {
  font-family: var(--font-display);
  font-size: 96px;
  color: var(--paper-edge);
  line-height: 1;
  margin-bottom: 12px;
  font-variation-settings: "opsz" 144;
}
.empty-line {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 18px;
  color: var(--ink-3);
  line-height: 1.5;
  font-variation-settings: "opsz" 24, "SOFT" 100;
}

/* =================================================================
 * Task strip — at the foot of the page, like editor's marks
 * ================================================================= */

.task-strip {
  position: fixed;
  bottom: 0; left: 280px; right: 0;
  background: var(--ink);
  color: var(--paper);
  padding: 10px 56px;
  z-index: 1500;
  border-top: 1.5px solid var(--vermilion);
  font-family: var(--font-mono);
  font-size: 12px;
  letter-spacing: 0.01em;
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 35vh;
  overflow-y: auto;
}
.task-row {
  display: grid;
  grid-template-columns: 32px 1fr;
  align-items: baseline;
  gap: 8px;
}
.task-row.done .task-message { color: var(--moss); opacity: 0.85; }
.task-row.error .task-message { color: var(--vermilion); }
.task-marker { text-align: center; }
.task-spinner {
  display: inline-block;
  letter-spacing: 0;
  color: var(--vermilion);
  animation: typewriter 0.9s steps(4) infinite;
}
@keyframes typewriter {
  0%   { opacity: 0.2; }
  25%  { opacity: 0.5; }
  50%  { opacity: 1;   }
  75%  { opacity: 0.5; }
  100% { opacity: 0.2; }
}
.task-icon-done { color: var(--moss); }
.task-icon-err { color: var(--vermilion); }
.task-message { color: rgba(245, 240, 230, 0.85); }

/* =================================================================
 * Browser inside create modal
 * ================================================================= */

.browser-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.browser-path {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-3);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 4px 0;
}
.browser-list {
  max-height: 240px;
  overflow-y: auto;
  border-top: 1px solid var(--paper-edge);
  border-bottom: 1px solid var(--paper-edge);
}
.browser-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 4px;
  cursor: pointer;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--ink-2);
  border-bottom: 1px solid var(--paper-edge);
  transition: padding-left 120ms ease, color 120ms ease;
}
.browser-item:last-child { border-bottom: 0; }
.browser-item:hover { padding-left: 8px; color: var(--ink); }
.browser-item:hover .bi-arrow { color: var(--vermilion); }
.bi-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bi-arrow { color: var(--ink-4); font-family: var(--font-display); }

/* =================================================================
 * Sidebar docs popover — "the back of the index card"
 *
 * 文档/产物面板：从侧边栏数字徽章右侧浮出。
 * 视觉处理为「便签纸」：略带阴影、轻微旋转 0deg（保持稳重），
 * 内里直接复用 DocsView 的版式但收紧一些尺寸。
 * ================================================================= */
.docs-popover {
  position: fixed;
  z-index: 1400;
  width: 560px;
  max-width: calc(100vw - 320px);
  max-height: calc(100vh - 32px);
  background: var(--paper);
  border: 1.5px solid var(--ink);
  box-shadow:
    4px 4px 0 0 var(--ink-2),
    8px 12px 24px -8px rgba(0,0,0,0.25);
  padding: 18px 24px 20px;
  overflow-y: auto;
  font-family: var(--font-body);
  /* 弹出动效：轻微从左滑入（300ms） */
  animation: popoverIn 220ms cubic-bezier(0.2, 0.7, 0.2, 1);
}
@keyframes popoverIn {
  from { opacity: 0; transform: translateX(-12px); }
  to   { opacity: 1; transform: translateX(0); }
}

/* 朱红色小三角"邮戳" */
.docs-popover .dp-arrow {
  position: absolute;
  left: -9px;
  top: 18px;
  width: 0;
  height: 0;
  border-top: 8px solid transparent;
  border-bottom: 8px solid transparent;
  border-right: 8px solid var(--ink);
}
.docs-popover .dp-arrow::after {
  content: "";
  position: absolute;
  left: 2px;
  top: -7px;
  width: 0;
  height: 0;
  border-top: 7px solid transparent;
  border-bottom: 7px solid transparent;
  border-right: 7px solid var(--paper);
}

.docs-popover .dp-eyebrow {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--vermilion);
  margin-bottom: 14px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--paper-edge);
}

/* 给 popover 中的 DocsView 收紧一下版式（标题略小、间距收一点） */
.docs-popover :deep(.docs) { max-width: 100%; }
.docs-popover :deep(.ds-section) { margin-bottom: 28px; }
.docs-popover :deep(.ds-title) { font-size: 22px; }
.docs-popover :deep(.ds-header) { padding-bottom: 8px; margin-bottom: 12px; }
.docs-popover :deep(.doc-row) { padding: 10px 4px 10px 0; }
.docs-popover :deep(.dr-name) { font-size: 14px; }
</style>
