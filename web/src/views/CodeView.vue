<template>
  <div class="chat-wrap">
    <!-- 会话侧栏：默认折叠成窄条，hover 浮出完整列表；点展开按钮可钉住 -->
    <div class="sessions-wrap" :class="{ collapsed: !sessionsOpen }">
      <!-- 折叠窄条：仅折叠态显示 -->
      <div v-if="!sessionsOpen" class="sessions-rail">
        <button class="rail-btn" :title="t('query.expand') + ' (' + sessions.length + ')'" @click="toggleSessions">
          <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><path d="M5 3 L9 7 L5 11" stroke="currentColor" stroke-width="1.4" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </button>
        <div class="rail-divider" aria-hidden="true"></div>
        <button class="rail-btn" :title="t('query.newSession')" @click="newSession">
          <svg width="13" height="13" viewBox="0 0 14 14" aria-hidden="true"><path d="M7 3 L7 11 M3 7 L11 7" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>
        </button>
      </div>
      <!-- 完整面板：展开态常显；折叠态 hover 浮出 -->
      <aside class="sessions">
        <div class="sessions-head">
          <div>
            <div class="eyebrow">{{ t('query.sessionsEyebrow') }}</div>
            <div class="title">{{ t('query.sessionsTitle') }}</div>
          </div>
          <div class="sessions-head-actions">
            <button class="btn btn-sm new-btn" :title="t('query.newSession')" @click="newSession">{{ t('query.newSession') }}</button>
            <button class="icon-btn" :title="t('query.collapse')" @click="toggleSessions">‹</button>
          </div>
        </div>
        <div class="sessions-list">
          <div
            v-if="currentSid === ''"
            :class="['session-item', 'active', 'session-item-new']"
          >
            <div class="s-title">
              <span v-if="querying" class="loading-glyph" style="font-size:10px">···</span>
              {{ querying ? t('query.thinkingDefault') : t('query.threadNew') }}
            </div>
          </div>
          <div
            v-for="s in sessions" :key="s.id"
            :class="['session-item', { active: s.id === currentSid }]"
            @click="openSession(s.id)"
          >
            <div class="s-title">{{ s.title || `(${t('query.threadUntitled')})` }}</div>
            <div class="s-meta">
              <span>{{ s.turn_count }} {{ s.turn_count === 1 ? t('query.turn') : t('query.turns') }}</span>
              <span class="s-meta-sep">·</span>
              <span>{{ relTime(s.updated_at) }}</span>
            </div>
            <button class="s-del" :title="t('common.delete')" @click.stop="deleteSession(s.id)">×</button>
          </div>
          <div v-if="sessions.length === 0" class="empty-hint" style="padding: 32px 16px">
            {{ t('query.noSessions') }}
          </div>
        </div>
      </aside>
    </div>

    <main class="chat-main">
      <!-- Hero / toolbar -->
      <div class="code-hero">
        <div>
          <div class="eyebrow">CODEGRAPH · {{ space.indexed ? t('code.indexed') : t('code.notIndexed') }}</div>
          <div class="code-title">{{ space.name }}</div>
          <div class="code-path">{{ space.path }} · {{ space.files }} {{ t('code.files') }}</div>
        </div>
        <button class="btn btn-sm sync-btn" :disabled="syncing" @click="sync">
          {{ syncing ? t('code.syncing') : t('code.syncIndex') }}
        </button>
      </div>

      <div ref="msgContainer" class="messages" @click="onMessageClick">
        <div v-if="loadingSession" class="empty-hint">
          <span class="loading-glyph">···</span> {{ t('query.loadingSession') }}
        </div>
        <div v-else-if="messages.length === 0 && !querying" class="chat-empty">
          <div class="ce-glyph">{ }</div>
          <p class="ce-line">{{ t('code.emptyAsk') }}</p>
          <p class="ce-sub">{{ t('code.emptyHint') }}</p>

          <!-- 提问卡片 -->
          <div class="suggest-block">
            <div class="suggest-head">
              <span class="suggest-label">{{ t('query.suggestionsLabel') }}</span>
              <button class="suggest-refresh" :disabled="suggestLoading" @click="loadSuggestions(true)">↻ {{ t('query.suggestionsRefresh') }}</button>
            </div>
            <div class="suggest-grid">
              <template v-if="suggestLoading && !suggestions.length">
                <div v-for="n in 4" :key="'sk' + n" class="suggest-card skeleton"><span class="loading-glyph">···</span></div>
              </template>
              <button v-for="(s, i) in suggestions" :key="i" class="suggest-card" @click="useSuggestion(s)" :title="s">
                <span class="sg-num">{{ String(i + 1).padStart(2, '0') }}</span>
                <span class="sg-text">{{ s }}</span>
                <span class="sg-arrow">→</span>
              </button>
            </div>
          </div>
        </div>

        <div v-for="(m, i) in messages" :key="i" :class="['msg', m.role]">
          <div class="bubble" v-if="m.role === 'user'">{{ m.content }}</div>
          <div v-else class="assistant-wrap">
            <div class="msg-mark">A.</div>

            <div
              v-if="!m.content && querying && i === messages.length - 1"
              class="loading-wrap"
            >
              <button
                type="button"
                class="bubble loading abortable"
                :title="t('query.abortHint')"
                @click="abortStream"
              >
                <span class="thinking-dots" aria-hidden="true"><span></span><span></span><span></span></span>
                <span class="thinking-text thinking-text-default">{{ thinkingMsg || t('query.thinkingDefault') }}</span>
                <span class="thinking-text thinking-text-hover">× {{ t('query.abortHint') }}</span>
                <span class="thinking-shimmer" aria-hidden="true"></span>
              </button>
              <!-- 工具调用滚动列表：累积展示 agent 多轮探索，不刷掉 -->
              <ol v-if="liveTools.length" ref="liveToolsEl" class="live-tools">
                <li v-for="(t2, ti) in liveTools" :key="ti" class="live-tool-item">
                  <span class="ti-arrow">→</span>
                  <span class="ti-text" :title="t2">{{ t2 }}</span>
                </li>
              </ol>
            </div>

            <div v-else class="bubble-host">
              <details v-if="getTools(i).length" class="reason-fold">
                <summary class="reason-fold-summary">
                  <span class="rf-arrow" aria-hidden="true">▸</span>
                  <span class="rf-text">{{ t('code.agentTrace') }} · {{ getTools(i).length }}</span>
                </summary>
                <ol class="reason-fold-list">
                  <li v-for="(t2, ti) in getTools(i)" :key="ti"><span class="ti-arrow">→</span><span class="ti-text" :title="t2">{{ t2 }}</span></li>
                </ol>
              </details>
              <div class="bubble md-content" v-html="renderMd(m.content)"></div>
              <button
                v-if="m.content && !(querying && i === messages.length - 1)"
                class="copy-btn"
                :title="copiedIdx === i ? t('common.copied') : t('common.copy')"
                @click="copyAnswer(i)"
              >{{ copiedIdx === i ? t('common.copied') : t('common.copy') }}</button>
            </div>

            <!-- 右栏：每轮图谱（持久化）+ 最后一轮 follow-ups
                 结构分两层：
                 - .rightcol-fold（details）只负责 sticky 定位 + 高度上限，自身不滚动
                 - .rightcol-scroll 才是真正独立滚动的内容区（flex:1+min-height:0+overflow-y:auto）
                 这样内部滚动完全自洽，不依赖外层还剩多少滚动距离。 -->
            <details
              v-if="hasRightCol(i, m)"
              class="rightcol-fold"
              open
            >
              <summary class="rightcol-fold-summary">
                <span class="rc-arrow" aria-hidden="true">▾</span>
                <span class="rc-label">{{ t('query.rightPanel') }}</span>
                <span v-if="getLinks(i).length" class="rc-count">· {{ getLinks(i).length }}</span>
              </summary>
              <div class="rightcol-scroll">
                <div class="rightcol">
                  <div v-if="getLinks(i).length" class="trace-block">
                    <div class="trace-label">{{ t('query.traceGraph') }}</div>
                    <ul class="graph-chips">
                      <li
                        v-for="(l, li) in getLinks(i)" :key="li"
                        :class="['graph-chip', l.category]"
                        @click="openGraph(l.name)"
                        :title="t('code.openGraph')"
                      >
                        <span class="gc-dot"></span>
                        <span class="gc-name">{{ l.name }}</span>
                        <span v-if="l.kind" class="gc-kind">{{ kindLabel(l.kind) }}</span>
                        <span class="gc-cat">{{ catLabel(l.category) }}</span>
                      </li>
                    </ul>
                  </div>
                  <div v-if="i === lastAssistantIdx && m.content && !querying && followUps.length" class="followup-row">
                    <span class="fu-label">{{ t('query.followUpLabel') }}</span>
                    <button v-for="(f, fi) in followUps" :key="fi" class="followup-chip" @click="useSuggestion(f)" :title="f">
                      <span class="fu-text">{{ f }}</span><span class="fu-arrow">→</span>
                    </button>
                  </div>
                </div>
              </div>
            </details>
          </div>
        </div>
      </div>

      <transition name="status-fade">
        <div v-if="querying" class="chat-status-bar working" aria-live="polite">
          <span class="csb-dots" aria-hidden="true"><span></span><span></span><span></span></span>
          <span class="csb-text">{{ thinkingMsg || t('query.thinkingDefault') }}</span>
          <span class="csb-shimmer" aria-hidden="true"></span>
        </div>
        <div v-else-if="justFinished" class="chat-status-bar done" aria-live="polite">
          <span class="csb-check">✓</span>
          <span class="csb-text">{{ t('query.justDone') }}</span>
        </div>
      </transition>

      <div class="input-bar">
        <textarea v-model="question" rows="2" :placeholder="dynamicPlaceholder" @keydown="onKeydown" :disabled="querying"></textarea>
        <button class="btn btn-primary send-btn" :disabled="querying || !question.trim()" @click="doSend">
          {{ querying ? '…' : t('query.send') }}
        </button>
      </div>
    </main>

    <CodeGraphPanel
      v-if="graphSeed"
      :space="space.name"
      :seed="graphSeed"
      @close="graphSeed = ''"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted, onUnmounted, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { renderMarkdownWithWikilinks, handleCodeCopyClick } from '../markdown'
import type { CodeSpaceInfo } from '../types'
import { useChatState, getSessionState, deleteSessionState, migrateSessionState } from '../composables/useChatState'
import type { SessionState } from '../composables/useChatState'
import { useUpload } from '../composables/useUpload'
import CodeGraphPanel from '../components/CodeGraphPanel.vue'

const { t, locale } = useI18n()

interface SessionMeta { id: string; title: string; turn_count: number; updated_at: string }

const props = defineProps<{ space: CodeSpaceInfo }>()
const emit = defineEmits<{ refresh: [] }>()

const chatState = useChatState('code:' + props.space.name)
const currentSid = computed({ get: () => chatState.currentSid, set: v => { chatState.currentSid = v } })

const ss = computed((): SessionState =>
  getSessionState('code:' + props.space.name, chatState.currentSid)
)
const messages     = computed(() => ss.value.messages)
const currentTitle = computed({ get: () => ss.value.currentTitle, set: v => { ss.value.currentTitle = v } })
const querying     = computed({ get: () => ss.value.querying,     set: v => { ss.value.querying = v } })
const thinkingMsg  = computed({ get: () => ss.value.thinkingMsg,  set: v => { ss.value.thinkingMsg = v } })
const followUps    = computed({ get: () => ss.value.followUps,    set: v => { ss.value.followUps = v } })

const sessions = ref<SessionMeta[]>([])
const question = ref('')
const msgContainer = ref<HTMLElement>()
const copiedIdx = ref<number>(-1)
const syncing = ref(false)
const { startUpload, pollTask } = useUpload()
const graphSeed = ref('') // 打开代码图谱浮层的种子符号

function openGraph(symbol: string) {
  graphSeed.value = symbol
}

const SESSIONS_KEY = 'okb-code-sessions-open'
const sessionsOpen = ref<boolean>(localStorage.getItem(SESSIONS_KEY) === '1')
function toggleSessions() {
  sessionsOpen.value = !sessionsOpen.value
  localStorage.setItem(SESSIONS_KEY, sessionsOpen.value ? '1' : '0')
}

const lastAssistantIdx = computed(() => {
  const msgs = messages.value
  for (let i = msgs.length - 1; i >= 0; i--) if (msgs[i].role === 'assistant') return i
  return -1
})

function getTools(i: number): string[] { return ss.value.traces[i]?.tools || [] }
function getLinks(i: number): Array<{ category: string; name: string; kind?: string }> { return ss.value.traces[i]?.links || [] }
// 图谱节点类别标签（agent 工具按动作分 5 类）
function catLabel(c: string): string {
  switch (c) {
    case 'search':  return t('code.catSearch')
    case 'callers': return t('code.catCallers')
    case 'callees': return t('code.catCallees')
    case 'impact':  return t('code.catImpact')
    case 'file':    return t('code.catFile')
    default:        return t('code.catSymbol')
  }
}
// 符号 kind 标签（codegraph 返回的 kind 字段）
function kindLabel(k: string): string {
  const map: Record<string, string> = {
    function: t('code.kindFunction'),
    method:   t('code.kindMethod'),
    class:    t('code.kindClass'),
    interface: t('code.kindInterface'),
    struct:   t('code.kindStruct'),
    trait:    t('code.kindTrait'),
    enum:     t('code.kindEnum'),
    constant: t('code.kindConstant'),
    const:    t('code.kindConstant'),
    variable: t('code.kindVariable'),
    var:      t('code.kindVariable'),
    field:    t('code.kindField'),
    property: t('code.kindField'),
    type:     t('code.kindType'),
    module:   t('code.kindModule'),
  }
  return map[k.toLowerCase()] || k
}

// 进行中这一轮的工具调用列表（loading 小框里滚动展示，累积不刷掉）
const liveTools = computed(() => (querying.value ? ss.value.trace.tools : []))
const liveToolsEl = ref<HTMLElement>()
watch(() => liveTools.value.length, async () => {
  await nextTick()
  if (liveToolsEl.value) liveToolsEl.value.scrollTop = liveToolsEl.value.scrollHeight
})
function hasRightCol(i: number, m: { content: string }): boolean {
  const isLast = i === lastAssistantIdx.value
  if (m.content && getLinks(i).length) return true
  if (isLast && followUps.value.length) return true
  return false
}

const justFinished = ref(false)
let justFinishedTimer: number | null = null
function flashJustFinished() {
  justFinished.value = true
  if (justFinishedTimer) window.clearTimeout(justFinishedTimer)
  justFinishedTimer = window.setTimeout(() => { justFinished.value = false; justFinishedTimer = null }, 1500)
}

function abortStream() {
  const ctrl = ss.value.abortCtrl
  if (ctrl) try { ctrl.abort() } catch { /* ignore */ }
}

async function copyAnswer(i: number) {
  const m = messages.value[i]
  if (!m) return
  try { await navigator.clipboard.writeText(m.content) } catch {
    const ta = document.createElement('textarea')
    ta.value = m.content; ta.style.position = 'fixed'; ta.style.opacity = '0'
    document.body.appendChild(ta); ta.select()
    try { document.execCommand('copy') } catch { /* ignore */ }
    document.body.removeChild(ta)
  }
  copiedIdx.value = i
  setTimeout(() => { if (copiedIdx.value === i) copiedIdx.value = -1 }, 1500)
}

function renderMd(s: string) { return renderMarkdownWithWikilinks(s) }
function onMessageClick(e: MouseEvent) { handleCodeCopyClick(e) }

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); doSend() }
}

async function scrollToBottom() {
  await nextTick()
  if (msgContainer.value) msgContainer.value.scrollTop = msgContainer.value.scrollHeight
}
async function scrollToAnswerStart(asstIdx: number) {
  await nextTick(); await nextTick()
  await new Promise<void>(r => requestAnimationFrame(() => r()))
  const root = msgContainer.value
  if (!root) return
  const items = root.querySelectorAll<HTMLElement>('.msg')
  const target = items[asstIdx - 1] || items[asstIdx]
  if (!target) return
  const delta = target.getBoundingClientRect().top - root.getBoundingClientRect().top
  root.scrollTo({ top: Math.max(0, root.scrollTop + delta), behavior: 'auto' })
}

async function loadSessions() {
  const res = await api.codeSessions(props.space.name)
  sessions.value = res.ok && res.sessions ? res.sessions : []
}

const loadingSession = ref(false)
async function openSession(sid: string) {
  if (sid === currentSid.value || loadingSession.value) return
  loadingSession.value = true
  currentSid.value = sid
  // 如果该 session 已有 messages，直接显示
  if (ss.value.messages.length > 0) {
    loadingSession.value = false
    await scrollToBottom()
    return
  }
  try {
    const res = await api.codeLoadSession(props.space.name, sid)
    if (sid !== currentSid.value) return
    if (res.ok && res.messages) {
      const msgs = res.messages
      msgs.forEach((m, idx) => {
        ss.value.messages.push({ role: m.role as 'user' | 'assistant', content: m.content })
        if (m.role === 'assistant') {
          ss.value.traces[idx] = { tools: m.tools || [], links: m.graph || [] }
        }
      })
      for (let i = msgs.length - 1; i >= 0; i--) {
        if (msgs[i].role === 'assistant') {
          ss.value.followUps = msgs[i].follow_ups || []
          break
        }
      }
      ss.value.currentTitle = res.title || ''
    }
  } finally {
    loadingSession.value = false
    await scrollToBottom()
  }
}

function newSession() {
  deleteSessionState('code:' + props.space.name, '')
  currentSid.value = ''
}

async function deleteSession(sid: string) {
  if (!confirm(t('query.deleteConfirm'))) return
  await api.codeDeleteSession(props.space.name, sid)
  if (sid === currentSid.value) newSession()
  await loadSessions()
}

function relTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso).getTime()
  if (isNaN(d)) return ''
  const diff = (Date.now() - d) / 1000
  if (diff < 60) return t('query.relJustNow')
  if (diff < 3600) return t('query.relMinAgo', { n: Math.floor(diff / 60) })
  if (diff < 86400) return t('query.relHourAgo', { n: Math.floor(diff / 3600) })
  return t('query.relDayAgo', { n: Math.floor(diff / 86400) })
}

async function doSend() {
  const q = question.value.trim()
  if (!q || querying.value) return

  const curSs = ss.value
  curSs.messages.push({ role: 'user', content: q })
  question.value = ''
  curSs.querying = true
  curSs.thinkingMsg = t('code.queryingCodeGraph')
  curSs.followUps = []
  await scrollToBottom()

  const assistantIdx = curSs.messages.length
  curSs.messages.push({ role: 'assistant', content: '' })
  await scrollToBottom()
  curSs.activeAsstIdx = assistantIdx
  curSs.trace = { tools: [], links: [] }
  curSs.traces[assistantIdx] = curSs.trace

  let answerText = ''
  let charCount = 0
  const abortCtrl = new AbortController()
  curSs.abortCtrl = abortCtrl
  const sendSid = chatState.currentSid

  try {
    const resp = await fetch('/api/code/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: abortCtrl.signal,
      body: JSON.stringify({
        space: props.space.name,
        session_id: currentSid.value || '',
        question: q,
        lang: locale.value,
        best_of: 3,
      }),
    })
    if (!resp.ok || !resp.body) throw new Error(`HTTP ${resp.status}`)
    thinkingMsg.value = ''

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''

    const processBuf = (final = false) => {
      let idx
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const chunk = buf.slice(0, idx); buf = buf.slice(idx + 2); handleSSEChunk(chunk)
      }
      if (final && buf.trim()) { handleSSEChunk(buf); buf = '' }
    }
    const handleSSEChunk = (chunk: string) => {
      const dataLines = chunk.split('\n').filter(l => l.startsWith('data: ')).map(l => l.slice(6))
      if (!dataLines.length) return
      let ev: any
      try { ev = JSON.parse(dataLines.join('\n')) } catch { return }
      handleSSEEvent(ev)
    }
    const handleSSEEvent = async (ev: any) => {
      const sState = getSessionState('code:' + props.space.name, sendSid)
      const amActive = sState.activeAsstIdx === assistantIdx
      const isCurrentSession = chatState.currentSid === sendSid

      if (ev.event === 'start') {
        if (amActive) {
          if (!sendSid && ev.session_id) { migrateSessionState('code:' + props.space.name, '', ev.session_id); chatState.currentSid = ev.session_id }
          if (isCurrentSession) thinkingMsg.value = t('query.generating')
        }
      } else if (ev.event === 'tool') {
        answerText = ''; charCount = 0; sState.streamingChars = 0
        const note = `${ev.name}(${(ev.args || '').slice(0, 80)})`
        if (amActive) sState.trace.tools.push(note)
        if (isCurrentSession) {
          if (ev.name === 'fact_check') thinkingMsg.value = t('code.factCheckRunning')
          else if (ev.name === 'fact_check_passed') thinkingMsg.value = t('code.factCheckPassed')
          else if (ev.name === 'fact_check_failed') thinkingMsg.value = t('code.factCheckRetry')
          else if (ev.name === 'fact_check_source') thinkingMsg.value = t('code.factCheckSource')
          else if (ev.name === 'context_compress') thinkingMsg.value = t('code.contextCompressing')
          else if (ev.name === 'context_compressed') thinkingMsg.value = t('code.contextCompressed')
          else if (ev.name === 'best_of_n') thinkingMsg.value = ev.args || t('code.bestOfNRunning')
          else thinkingMsg.value = t('query.toolRunning', { name: ev.name || '?' })
        }
      } else if (ev.event === 'delta') {
        const piece = ev.text || ''
        if (piece) {
          answerText += piece; charCount += piece.length
          sState.streamingChars = charCount
          if (isCurrentSession) thinkingMsg.value = t('query.generatingProgress', { n: charCount })
        }
      } else if (ev.event === 'done') {
        if (typeof ev.answer === 'string' && ev.answer.trim()) answerText = ev.answer
        if (amActive) {
          if (!sendSid && ev.session_id) { migrateSessionState('code:' + props.space.name, '', ev.session_id); chatState.currentSid = ev.session_id }
          if (ev.title) sState.currentTitle = ev.title
          sState.messages[assistantIdx] = { role: 'assistant', content: answerText }
          if (Array.isArray(ev.graph)) sState.trace.links = ev.graph
          sState.streamingChars = 0
          if (isCurrentSession) flashJustFinished()
        }
        // 无守卫解锁该 session 的 querying
        sState.querying = false
        sState.thinkingMsg = ''
        if (isCurrentSession) await scrollToAnswerStart(assistantIdx)
        loadSessions()
      } else if (ev.event === 'follow_ups') {
        if (Array.isArray(ev.follow_ups) && amActive) {
          sState.followUps = ev.follow_ups.filter((s: any) => typeof s === 'string' && s.trim())
        }
      } else if (ev.event === 'error') {
        answerText += `\n\n> ✦ ${ev.error || t('query.errInference')}`
        if (amActive) sState.messages[assistantIdx] = { role: 'assistant', content: answerText }
      }
    }

    while (true) {
      const { value, done } = await reader.read()
      if (done) { buf += decoder.decode(); processBuf(true); break }
      buf += decoder.decode(value, { stream: true })
      processBuf(false)
    }
  } catch (e: any) {
    const sState = getSessionState('code:' + props.space.name, sendSid)
    const amActive = sState.activeAsstIdx === assistantIdx
    if (e?.name === 'AbortError' || abortCtrl.signal.aborted) {
      answerText = (answerText || '') + `\n\n> _${t('query.aborted')}_`
      if (amActive) sState.messages[assistantIdx] = { role: 'assistant', content: answerText }
    } else {
      answerText = (answerText || '') + `\n\n> ✦ ${t('query.errNetwork', { e: e?.message || e })}`
      if (amActive) sState.messages[assistantIdx] = { role: 'assistant', content: answerText }
    }
  } finally {
    const sState = getSessionState('code:' + props.space.name, sendSid)
    const amActive = sState.activeAsstIdx === assistantIdx
    const isCurrentSession = chatState.currentSid === sendSid
    if (sState.abortCtrl === abortCtrl) sState.abortCtrl = null
    let didFallback = false
    if (amActive && sState.messages[assistantIdx] && sState.messages[assistantIdx].content === '' && answerText) {
      sState.messages[assistantIdx] = { role: 'assistant', content: answerText }; didFallback = true
    }
    if (sState.querying) {
      sState.querying = false; sState.thinkingMsg = ''; sState.streamingChars = 0
      loadSessions(); didFallback = true
    }
    if (didFallback && isCurrentSession) await scrollToAnswerStart(assistantIdx)
  }
}

// ---- 提问卡片 ----
const suggestions = ref<string[]>([])
const suggestLoading = ref(false)
const placeholderIdx = ref(0)
let placeholderTimer: number | null = null
const isEmpty = computed(() => messages.value.length === 0 && !currentSid.value)

const dynamicPlaceholder = computed(() => {
  const fallback = t('code.placeholder')
  if (!isEmpty.value || suggestions.value.length === 0) return fallback
  const s = suggestions.value[placeholderIdx.value % suggestions.value.length]
  return s ? `${s}    ⏎` : fallback
})

async function loadSuggestions(force = false) {
  if (suggestLoading.value && !force) return
  suggestLoading.value = true
  try {
    const res = await api.codeSuggestions(props.space.name, locale.value)
    suggestions.value = res.suggestions || []
    placeholderIdx.value = 0
  } catch { suggestions.value = [] }
  finally { suggestLoading.value = false }
}
function startPlaceholderRotation() {
  stopPlaceholderRotation()
  placeholderTimer = window.setInterval(() => {
    if (suggestions.value.length > 0) placeholderIdx.value = (placeholderIdx.value + 1) % suggestions.value.length
  }, 5000)
}
function stopPlaceholderRotation() { if (placeholderTimer) { window.clearInterval(placeholderTimer); placeholderTimer = null } }

function useSuggestion(text: string) {
  if (querying.value) return
  question.value = text
  doSend()
}

async function sync() {
  syncing.value = true
  try {
    const res = await api.codeSync(props.space.name)
    if (!res.success || !res.task_id) {
      syncing.value = false
      return
    }
    const uiId = startUpload(1, '正在同步 CodeGraph 索引…')
    pollTask(uiId, res.task_id, () => {
      syncing.value = false
      emit('refresh')
    })
  } catch (e: any) {
    syncing.value = false
    console.error('sync error:', e?.message || e)
  }
}

watch(isEmpty, (empty) => {
  if (empty) { loadSuggestions(); startPlaceholderRotation() } else { stopPlaceholderRotation() }
}, { immediate: true })
watch(locale, () => { if (isEmpty.value) loadSuggestions() })

onMounted(loadSessions)

// 实时测量 .messages 可视高度写入 CSS 变量 --rightcol-max-h，
// 供参考面板 max-height 用，保证面板高度不超出展示区。
let rcResizeObserver: ResizeObserver | null = null
onMounted(() => {
  const el = msgContainer.value
  if (!el) return
  const update = () => el.style.setProperty('--rightcol-max-h', `${el.clientHeight - 24}px`)
  update()
  rcResizeObserver = new ResizeObserver(update)
  rcResizeObserver.observe(el)
})
onUnmounted(() => {
  stopPlaceholderRotation()
  if (justFinishedTimer) { window.clearTimeout(justFinishedTimer); justFinishedTimer = null }
  rcResizeObserver?.disconnect()
})
</script>

<style scoped>
.chat-wrap {
  display: flex; gap: 0;
  height: calc(100vh - 200px); min-height: 540px;
  border: 1.5px solid var(--ink); background: var(--paper);
}

/* sessions sidebar — 默认折叠窄条，hover 浮出完整列表 */
.sessions-wrap {
  position: relative;
  width: 260px;
  flex-shrink: 0;
  transition: width 240ms cubic-bezier(0.2, 0.7, 0.2, 1);
}
.sessions-wrap.collapsed { width: 36px; }
.sessions {
  width: 260px; height: 100%; flex-shrink: 0;
  border-right: 1px solid var(--paper-edge);
  background: var(--paper-2); display: flex; flex-direction: column;
}
/* 折叠态：完整面板浮为浮层，hover wrap 时滑入 */
.sessions-wrap.collapsed .sessions {
  position: absolute; top: 0; left: 0; bottom: 0; z-index: 400;
  opacity: 0; pointer-events: none;
  transform: translateX(-12px);
  transition: opacity 200ms ease, transform 200ms ease;
  box-shadow: 4px 0 28px -10px rgba(0,0,0,0.25);
}
.sessions-wrap.collapsed:hover .sessions {
  opacity: 1; pointer-events: auto; transform: translateX(0);
}
.sessions-rail { position: absolute; inset: 0; width: 36px; display: flex; flex-direction: column; align-items: center; padding: 14px 0; background: var(--paper-2); border-right: 1px solid var(--paper-edge); }
.rail-btn {
  appearance: none; background: transparent; border: 0; width: 28px; height: 28px;
  display: inline-flex; align-items: center; justify-content: center;
  color: var(--ink-3); cursor: pointer; transition: color 130ms ease;
}
.rail-btn:hover { color: var(--vermilion); }
.rail-divider { width: 1px; flex: 1; min-height: 60px; background: var(--paper-edge); margin: 10px 0; }
.sessions-head-actions { display: flex; align-items: center; gap: 6px; }
.icon-btn { appearance: none; background: transparent; border: 0; width: 24px; height: 24px; font-family: var(--font-display); font-size: 16px; color: var(--ink-4); cursor: pointer; }
.icon-btn:hover { color: var(--vermilion); }
.sessions-head { padding: 16px 18px 14px; border-bottom: 1px solid var(--paper-edge); display: flex; justify-content: space-between; align-items: flex-end; gap: 10px; }
.sessions-head .title { font-family: var(--font-display); font-size: 18px; font-weight: 500; margin-top: 2px; }
.new-btn { font-size: 11px; padding: 4px 10px; white-space: nowrap; }
.sessions-list { flex: 1; overflow-y: auto; padding: 6px 0; }
.session-item { position: relative; padding: 12px 36px 12px 18px; cursor: pointer; border-bottom: 1px solid var(--paper-edge); transition: padding-left 150ms ease, background 150ms ease; }
.session-item:hover { background: rgba(255, 252, 244, 0.6); padding-left: 22px; }
.session-item.active { background: var(--ink); color: var(--paper); border-bottom-color: var(--ink); }
.s-title { font-family: var(--font-display); font-size: 14px; font-weight: 500; line-height: 1.3; overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.s-meta { margin-top: 4px; font-family: var(--font-mono); font-size: 10px; color: var(--ink-4); display: flex; gap: 6px; align-items: baseline; }
.session-item.active .s-meta { color: rgba(245, 240, 230, 0.5); }
.s-meta-sep { opacity: 0.5; }
.s-del { position: absolute; right: 8px; top: 12px; background: none; border: none; color: var(--ink-4); cursor: pointer; font-size: 14px; padding: 4px; opacity: 0; transition: opacity 130ms ease, color 130ms ease; }
.session-item:hover .s-del { opacity: 1; }
.s-del:hover { color: var(--vermilion); }
.session-item.active .s-del { color: rgba(245, 240, 230, 0.5); }

/* chat main */
.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; padding: 0 28px; }
.code-hero {
  display: flex; justify-content: space-between; align-items: flex-start; gap: 16px;
  padding: 16px 0 14px; border-bottom: 1px solid var(--paper-edge);
}
.code-hero .eyebrow { font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.12em; color: var(--ink-4); }
.code-title { font-family: var(--font-display); font-size: 22px; font-weight: 500; font-style: italic; margin-top: 4px; }
.code-path { font-family: var(--font-mono); font-size: 11px; color: var(--ink-4); margin-top: 2px; word-break: break-all; }
.sync-btn { white-space: nowrap; flex-shrink: 0; }

.messages { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 26px; padding: 24px 0; }
.empty-hint { color: var(--ink-4); font-family: var(--font-mono); font-size: 12px; }
.chat-empty { margin: auto; text-align: center; }
.ce-glyph { font-family: var(--font-display); font-size: 70px; color: var(--paper-edge); line-height: 1; margin-bottom: 12px; }
.ce-line { font-family: var(--font-display); font-style: italic; font-size: 18px; color: var(--ink-2); margin-bottom: 6px; }
.ce-sub { font-family: var(--font-mono); font-size: 11px; letter-spacing: 0.08em; color: var(--ink-4); max-width: 460px; margin: 0 auto; }

.msg { display: flex; }
.msg.user { justify-content: flex-end; }
.msg.assistant { justify-content: flex-start; }
.msg.user .bubble { max-width: 78%; padding: 12px 18px; background: var(--ink); color: var(--paper); font-family: var(--font-body); font-size: 14px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }

.assistant-wrap { position: relative; max-width: 100%; display: grid; grid-template-columns: 28px 1fr minmax(180px, 260px); gap: 10px; align-items: start; }
@media (max-width: 979px) { .assistant-wrap { grid-template-columns: 28px 1fr; } }
.bubble-host { position: relative; min-width: 0; }
.assistant-wrap .bubble { background: rgba(255, 252, 244, 0.4); border-left: 1.5px solid var(--paper-edge); padding: 4px 16px; font-family: var(--font-body); font-size: 14px; line-height: 1.7; color: var(--ink); word-break: break-word; }
.assistant-wrap .bubble.loading { position: relative; font-family: var(--font-display); font-style: italic; font-size: 14px; color: var(--ink-3); background: var(--paper-2); border-left: 1.5px solid var(--vermilion); padding: 12px 22px 14px 18px; display: inline-flex; align-items: center; gap: 12px; overflow: hidden; animation: loadingPulse 1.6s ease-in-out infinite; min-width: 200px; }
@keyframes loadingPulse { 0%, 100% { opacity: 0.92; } 50% { opacity: 1; } }
.assistant-wrap button.bubble.loading { appearance: none; border-top: 0; border-right: 0; border-bottom: 0; text-align: left; font: inherit; cursor: pointer; transition: border-color 160ms ease, background 160ms ease, transform 130ms ease; }
.assistant-wrap button.bubble.loading:hover { background: var(--paper); border-left-color: var(--vermilion); animation-play-state: paused; transform: translateX(-1px); }
.thinking-dots { display: inline-flex; align-items: flex-end; gap: 4px; height: 14px; }
.thinking-dots span { width: 6px; height: 6px; border-radius: 50%; background: var(--vermilion); display: inline-block; animation: thinkingDotJump 1.1s ease-in-out infinite; }
.thinking-dots span:nth-child(2) { animation-delay: 0.18s; background: var(--ink-2); }
.thinking-dots span:nth-child(3) { animation-delay: 0.36s; background: var(--ink-3); }
@keyframes thinkingDotJump { 0%, 60%, 100% { transform: translateY(0); opacity: 0.4; } 30% { transform: translateY(-6px); opacity: 1; } }
.thinking-text-hover { display: none; color: var(--vermilion); }
.assistant-wrap button.bubble.loading:hover .thinking-text-default { display: none; }
.assistant-wrap button.bubble.loading:hover .thinking-text-hover { display: inline; font-style: normal; letter-spacing: 0.04em; }
.thinking-shimmer { position: absolute; left: 0; bottom: 0; height: 1px; width: 40%; background: linear-gradient(90deg, transparent, var(--vermilion), transparent); animation: shimmerSlide 1.6s linear infinite; }
@keyframes shimmerSlide { 0% { transform: translateX(-100%); } 100% { transform: translateX(350%); } }
.msg-mark { font-family: var(--font-display); font-style: italic; font-size: 18px; color: var(--vermilion); line-height: 1.4; padding-top: 1px; text-align: right; }
.copy-btn { position: absolute; right: 6px; bottom: 4px; appearance: none; background: rgba(245, 240, 230, 0.85); border: 0; font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--ink-4); cursor: pointer; opacity: 0; transition: opacity 130ms ease, color 130ms ease; padding: 3px 7px; pointer-events: none; }
.bubble-host:hover .copy-btn { opacity: 1; pointer-events: auto; }
.copy-btn:hover { color: var(--vermilion); background: var(--paper); }
.loading-glyph { display: inline-block; font-family: var(--font-mono); color: var(--vermilion); margin-right: 6px; animation: typewriter 0.9s steps(4) infinite; }
@keyframes typewriter { 0% { opacity: 0.2; } 50% { opacity: 1; } 100% { opacity: 0.2; } }

/* loading 小框 + 工具滚动列表 */
.loading-wrap { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
.live-tools {
  list-style: none; margin: 0; padding: 8px 12px;
  max-height: 96px; overflow-y: auto;
  background: var(--paper-2); border-left: 1.5px solid var(--paper-edge);
  font-family: var(--font-mono); font-size: 11px; min-width: 240px; max-width: 520px;
  display: flex; flex-direction: column; gap: 3px;
}
.live-tool-item { display: flex; align-items: baseline; gap: 6px; color: var(--ink-3); animation: traceFadeIn 220ms ease; }
.live-tool-item .ti-arrow { color: var(--vermilion); font-family: var(--font-display); flex-shrink: 0; }
.live-tool-item .ti-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }

/* reason fold */
.reason-fold { margin-bottom: 8px; font-family: var(--font-mono); font-size: 11px; color: var(--ink-4); border-left: 1px solid var(--paper-edge); padding-left: 10px; }
.reason-fold[open] { border-left-color: var(--vermilion); }
.reason-fold-summary { list-style: none; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; padding: 2px 0; user-select: none; outline: none; letter-spacing: 0.06em; text-transform: uppercase; }
.reason-fold-summary::-webkit-details-marker { display: none; }
.rf-arrow { display: inline-block; font-size: 10px; color: var(--vermilion); transition: transform 150ms ease; }
.reason-fold[open] .rf-arrow { transform: rotate(90deg); }
.rf-text { font-size: 10px; }
.reason-fold-list { list-style: none; padding: 6px 0 4px; margin: 0; display: flex; flex-direction: column; gap: 3px; }
.reason-fold-list li { display: flex; align-items: baseline; gap: 6px; font-size: 11px; color: var(--ink-3); }
.reason-fold-list .ti-arrow { color: var(--vermilion); font-family: var(--font-display); flex-shrink: 0; }
.reason-fold-list .ti-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }

/* status bar */
.chat-status-bar { position: relative; display: flex; align-items: center; gap: 10px; padding: 8px 14px; font-family: var(--font-mono); font-size: 11px; color: var(--ink-3); border-top: 1px solid var(--paper-edge); overflow: hidden; background: var(--paper-2); }
.chat-status-bar.working { background: linear-gradient(180deg, var(--paper-2), var(--paper)); }
.chat-status-bar.done { color: var(--moss); background: linear-gradient(180deg, rgba(91,111,63,0.08), transparent); }
.csb-dots { display: inline-flex; align-items: flex-end; gap: 3px; height: 10px; }
.csb-dots span { width: 4px; height: 4px; border-radius: 50%; background: var(--vermilion); animation: thinkingDotJump 1.1s ease-in-out infinite; }
.csb-dots span:nth-child(2) { animation-delay: 0.18s; background: var(--ink-2); }
.csb-dots span:nth-child(3) { animation-delay: 0.36s; background: var(--ink-3); }
.csb-text { flex: 1; font-style: italic; font-family: var(--font-display); font-size: 12px; }
.csb-check { font-family: var(--font-display); font-size: 14px; color: var(--moss); }
.csb-shimmer { position: absolute; left: 0; bottom: 0; height: 1px; width: 30%; background: linear-gradient(90deg, transparent, var(--vermilion), transparent); animation: shimmerSlide 1.6s linear infinite; }
.status-fade-enter-active, .status-fade-leave-active { transition: opacity 250ms ease, transform 250ms ease, max-height 250ms ease; max-height: 60px; }
.status-fade-enter-from, .status-fade-leave-to { opacity: 0; transform: translateY(4px); max-height: 0; }

/* input */
.input-bar { display: flex; gap: 12px; align-items: flex-end; padding: 16px 0 18px; border-top: 1.5px solid var(--ink); background: linear-gradient(180deg, transparent, rgba(245,240,230,0.6)); }
.input-bar textarea { flex: 1; resize: none; font-family: var(--font-body); font-size: 14px; line-height: 1.55; border: 1px solid var(--paper-edge); background: rgba(255,252,244,0.5); padding: 10px 14px; min-height: 56px; }
.input-bar textarea:focus { border-color: var(--ink); background: var(--paper); }
.send-btn { white-space: nowrap; height: fit-content; padding: 9px 22px; }

/* trace block (右栏) */
.trace-block { display: flex; flex-direction: column; gap: 8px; }
.trace-label { font-family: var(--font-mono); font-size: 9px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--ink-4); padding-left: 2px; }
.trace-list { list-style: none; padding: 0 0 0 10px; margin: 0; display: flex; flex-direction: column; gap: 4px; border-left: 1px solid var(--paper-edge); max-height: 240px; overflow-y: auto; }
.trace-item { display: flex; align-items: baseline; gap: 6px; font-family: var(--font-mono); font-size: 11px; color: var(--ink-3); animation: traceFadeIn 220ms ease; }
@keyframes traceFadeIn { from { opacity: 0; transform: translateX(-4px); } to { opacity: 1; transform: translateX(0); } }
.trace-item .ti-arrow { color: var(--vermilion); font-family: var(--font-display); flex-shrink: 0; }
.trace-item .ti-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }

/* 右栏探索过程折叠：querying 自动展开滚动框，done 自动折叠 */
.trace-fold { font-family: var(--font-mono); font-size: 11px; color: var(--ink-4); }
.trace-fold-sum { list-style: none; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; padding: 2px 0; user-select: none; outline: none; font-size: 9px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--ink-4); }
.trace-fold-sum::-webkit-details-marker { display: none; }
.trace-fold-sum:hover { color: var(--ink-2); }
.trace-fold .rf-arrow { display: inline-block; font-size: 10px; color: var(--vermilion); transition: transform 150ms ease; }
.trace-fold[open] .rf-arrow { transform: rotate(90deg); }
.trace-fold .trace-list { margin-top: 6px; }

/* 代码图谱节点 chips（点击打开图谱浮层） */
.graph-chips { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.graph-chip {
  display: flex; align-items: center; gap: 7px; padding: 5px 8px; cursor: pointer;
  font-family: var(--font-mono); font-size: 11px; color: var(--ink-2);
  border: 1px solid var(--paper-edge); background: var(--paper);
  transition: border-color 130ms ease, background 130ms ease, transform 130ms ease;
}
.graph-chip:hover { border-color: var(--ink); background: var(--paper-2); transform: translateX(2px); }
.gc-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: #5566a0; }
.graph-chip.search  .gc-dot { background: #8a8a8a; }   /* 搜索：中性灰 */
.graph-chip.callers .gc-dot { background: #d97a3a; }   /* 调用方：暖橙 */
.graph-chip.callees .gc-dot { background: #3a7a8c; }   /* 被调用：冷青 */
.graph-chip.impact  .gc-dot { background: #c8302e; }   /* 影响面：朱红 */
.graph-chip.file    .gc-dot { background: #7a8a4a; }   /* 源文件：苔绿 */
.gc-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.gc-kind { font-size: 9px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--ink-2); flex-shrink: 0; padding: 1px 5px; border: 1px solid var(--paper-edge); background: var(--paper-2); }
.gc-cat { font-size: 9px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--ink-4); flex-shrink: 0; }

/* 右栏折叠外壳 .rightcol-fold — sticky 粘在视口内跟随会话滚动，
   自身 max-height 限制在展示区内 + overflow-y:auto 内部独立滚动。
   单层方案（sticky + max-height + overflow-y:auto 是浏览器最基础支持的组合）。 */
.rightcol-fold {
  grid-column: 2 / 3;
  margin-top: 14px;
  padding-left: 16px;
  border-left: 1px solid var(--paper-edge);
}
.rightcol-fold[open] { border-left-color: var(--vermilion); }
@media (min-width: 980px) {
  .rightcol-fold {
    grid-column: 3 / 4;
    margin-top: 0;
    padding-left: 0;
    align-self: start;
    padding-top: 4px;
    position: sticky;
    top: 12px;
    /* max-height 用 JS 实测的 .messages 可视高度（--rightcol-max-h），
       保证面板不超出展示区；回退 70vh 仅用于 JS 未跑起来的瞬间。 */
    max-height: var(--rightcol-max-h, 70vh);
    overflow-y: auto;
    overscroll-behavior: contain;
  }
}
.rightcol-scroll {
  /* 普通内容容器，不做滚动（滚动交给外层 .rightcol-fold）；
     底部留白，避免最后一行贴着滚动区边缘被切。 */
  padding-bottom: 12px;
}

/* summary 手柄 */
.rightcol-fold-summary {
  list-style: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  margin-bottom: 10px;
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-4);
  user-select: none;
  outline: none;
  transition: color 130ms ease;
}
.rightcol-fold-summary::-webkit-details-marker { display: none; }
.rightcol-fold-summary:hover { color: var(--ink-2); }

.rc-arrow {
  display: inline-block;
  font-size: 10px;
  color: var(--vermilion);
  transition: transform 150ms ease;
}
.rightcol-fold:not([open]) .rc-arrow { transform: rotate(-90deg); }

.rc-label { font-size: 10px; }
.rc-count { font-family: var(--font-display); font-size: 12px; color: var(--ink-3); }

/* .rightcol 退化为纯内容容器 */
.rightcol { display: flex; flex-direction: column; gap: 14px; }
.fu-label { font-family: var(--font-display); font-style: italic; font-size: 12px; color: var(--ink-4); margin-right: 2px; white-space: nowrap; }
.followup-row { display: flex; flex-wrap: wrap; gap: 8px; align-items: baseline; }
@media (min-width: 980px) {
  .followup-row { flex-direction: column; align-items: stretch; }
  .followup-row .fu-label { margin-bottom: 2px; margin-right: 0; padding-left: 2px; }
  .followup-row .followup-chip { width: 100%; justify-content: space-between; }
  .followup-row .fu-text { white-space: normal; max-width: none; line-height: 1.4; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; text-align: left; }
}
.followup-chip { appearance: none; background: var(--paper); border: 1px solid var(--paper-edge); padding: 6px 12px; cursor: pointer; font-family: var(--font-body); font-size: 12.5px; color: var(--ink-2); display: inline-flex; align-items: center; gap: 6px; max-width: 100%; transition: border-color 150ms ease, color 150ms ease, transform 150ms ease, box-shadow 150ms ease; animation: fuPop 280ms cubic-bezier(0.2, 0.7, 0.2, 1) both; }
@keyframes fuPop { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }
.followup-chip:hover { border-color: var(--ink); color: var(--ink); transform: translateY(-1px); box-shadow: 2px 2px 0 0 var(--vermilion); }
.fu-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 320px; }
.fu-arrow { font-family: var(--font-display); color: var(--ink-4); font-size: 12px; }
.followup-chip:hover .fu-arrow { color: var(--vermilion); }

/* suggest cards */
.suggest-block { margin-top: 36px; text-align: left; max-width: 720px; width: 100%; margin-left: auto; margin-right: auto; }
.suggest-head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 12px; padding: 0 4px; }
.suggest-label { font-family: var(--font-display); font-style: italic; font-size: 13px; color: var(--ink-3); }
.suggest-refresh { appearance: none; background: none; border: 0; font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase; color: var(--ink-4); cursor: pointer; padding: 4px 6px; }
.suggest-refresh:hover { color: var(--vermilion); }
.suggest-refresh:disabled { opacity: 0.4; cursor: not-allowed; }
.suggest-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; }
@media (min-width: 980px) { .suggest-grid { grid-template-columns: repeat(4, 1fr); } }
.suggest-card { appearance: none; text-align: left; background: var(--paper); border: 1px solid var(--paper-edge); padding: 14px 14px 12px; cursor: pointer; font-family: var(--font-body); font-size: 13px; color: var(--ink-2); line-height: 1.5; display: flex; flex-direction: column; gap: 6px; min-height: 100px; position: relative; transition: transform 180ms cubic-bezier(0.2, 0.7, 0.2, 1), border-color 180ms ease, box-shadow 180ms ease, color 180ms ease; }
.suggest-card:hover { transform: translateY(-2px); border-color: var(--ink); color: var(--ink); box-shadow: 3px 3px 0 0 var(--vermilion); }
.sg-num { font-family: var(--font-mono); font-size: 10px; color: var(--ink-4); }
.suggest-card:hover .sg-num { color: var(--vermilion); }
.sg-text { flex: 1; font-family: var(--font-display); font-size: 14px; font-weight: 500; color: inherit; display: -webkit-box; -webkit-line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
.sg-arrow { font-family: var(--font-display); font-size: 14px; color: var(--ink-4); align-self: flex-end; transition: color 180ms ease, transform 180ms ease; }
.suggest-card:hover .sg-arrow { color: var(--vermilion); transform: translateX(3px); }
.suggest-card.skeleton { cursor: default; pointer-events: none; align-items: center; justify-content: center; color: var(--ink-4); }
</style>
