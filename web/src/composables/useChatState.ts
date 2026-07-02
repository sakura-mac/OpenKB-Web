// 跨 tab 切换保持 chat 状态：按 space.name 隔离，QueryView 卸载/重建后状态依然存在。
//
// 设计分层：
//   SpaceChatState  — per-space：currentSid、sessionStates
//   SessionState    — per-session：messages、traces、querying、thinkingMsg 等全部状态
//     key = session id（新建会话未拿到 sid 前用空字符串 '' 代表当前新建轮）
//
// 切换 session 时 currentSid 变化，UI 自动切到对应 SessionState，不清除任何数据；
// 切回正在等待的 session 能看到 loading 状态、消息列表、中断按钮。
import { reactive } from 'vue'

export interface ChatMessage { role: 'user' | 'assistant'; content: string }

export interface TraceEntry {
  tools: string[]
  links: Array<{ category: string; name: string; kind?: string }>
}

/** 一个 session 的完整状态（消息 + 运行时）。*/
export interface SessionState {
  messages: ChatMessage[]
  currentTitle: string
  /** 按 assistant 消息下标存的逐轮思考过程 */
  traces: Record<number, TraceEntry>
  // —— 正在进行中的一轮（无活跃轮时全部是初始值）——
  querying: boolean
  thinkingMsg: string
  streamingChars: number
  abortCtrl: AbortController | null
  /** 本轮 assistant 消息在 messages 数组里的下标，无活跃轮 = -1 */
  activeAsstIdx: number
  followUps: string[]
  trace: TraceEntry
}

// 为兼容 CodeView 的 liveTools 读法，SessionState 保留 trace 字段
function makeSessionState(): SessionState {
  return {
    messages: [],
    currentTitle: '',
    traces: {},
    querying: false,
    thinkingMsg: '',
    streamingChars: 0,
    abortCtrl: null,
    activeAsstIdx: -1,
    followUps: [],
    trace: { tools: [], links: [] },
  }
}

interface SpaceChatState {
  currentSid: string
  /** per-session 完整状态。key = sid（新建会话跑完前用 '' 占位）。*/
  sessions: Record<string, SessionState>
}

const states = reactive<Record<string, SpaceChatState>>({})

function makeSpaceState(): SpaceChatState {
  return { currentSid: '', sessions: {} }
}

export function useChatState(spaceName: string): SpaceChatState {
  if (!states[spaceName]) states[spaceName] = makeSpaceState()
  return states[spaceName]
}

/** 获取（或创建）某个 session 的状态。sid='' 代表当前新建会话。*/
export function getSessionState(spaceName: string, sid: string): SessionState {
  const space = useChatState(spaceName)
  if (!space.sessions[sid]) space.sessions[sid] = makeSessionState()
  return space.sessions[sid]
}

/** 删除某个 session 的状态（删除会话时调用）。*/
export function deleteSessionState(spaceName: string, sid: string) {
  const space = states[spaceName]
  if (space) delete space.sessions[sid]
}

/** 把 fromSid 的 session state 迁移到 toSid（新建会话拿到真实 sid 时用）。*/
export function migrateSessionState(spaceName: string, fromSid: string, toSid: string) {
  const space = states[spaceName]
  if (!space || !space.sessions[fromSid]) return
  if (toSid === fromSid) return
  // 把 '' 的 state 挂到真实 sid 下，然后删掉 ''
  space.sessions[toSid] = space.sessions[fromSid]
  delete space.sessions[fromSid]
}

export function resetChatState(spaceName: string) {
  const s = states[spaceName]
  if (s) {
    for (const ss of Object.values(s.sessions)) {
      if (ss.abortCtrl) try { ss.abortCtrl.abort() } catch { /* ignore */ }
    }
  }
  states[spaceName] = makeSpaceState()
}

// ── 向后兼容 ──────────────────────────────────────────────────
// QueryView/CodeView 原来直接读 chatState.messages / chatState.traces /
// chatState.currentTitle 等，现在这些字段都移到 sessions[sid] 里了。
// 通过 getCurrentSession(chatState) 拿到当前 session 的引用，保持原有写法不变。
export function getCurrentSession(space: SpaceChatState): SessionState {
  return getSessionState(
    // spaceName 不直接可得，用 sessions 本身即可（reactive 引用）
    // ——实际上调用方在 Vue computed 里用，空 sid 时也要有默认 state
    Object.keys(states).find(k => states[k] === space) || '',
    space.currentSid,
  )
}
