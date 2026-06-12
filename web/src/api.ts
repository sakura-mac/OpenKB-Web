import type { SpaceInfo, SpaceDetail, TaskStatus, GraphData, DeckInfo, CodeSpaceInfo } from './types'

async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const opts: RequestInit = { method, headers: { 'Content-Type': 'application/json' } }
  if (body) opts.body = JSON.stringify(body)
  const res = await fetch(path, opts)
  // 优先解析 JSON；非 JSON（如网关 502）时抛出可读错误
  const text = await res.text()
  let data: any
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    throw new Error(`HTTP ${res.status}: ${text.slice(0, 200) || '响应非 JSON'}`)
  }
  if (!res.ok && data && typeof data === 'object' && !('error' in data)) {
    throw new Error(`HTTP ${res.status}`)
  }
  return data as T
}

export const api = {
  listSpaces: () => request<SpaceInfo[]>('/api/spaces'),
  listCodeSpaces: () => request<CodeSpaceInfo[]>('/api/code-spaces'),
  spaceDetail: (name: string) => request<SpaceDetail>(`/api/space/${encodeURIComponent(name)}`),
  codeSpaceDetail: (name: string) => request<CodeSpaceInfo>(`/api/code-space/${encodeURIComponent(name)}`),
  createCodeSpace: (name: string, path: string) =>
    request<{ success?: boolean; error?: string; task_id?: string; space?: CodeSpaceInfo }>('/api/code-spaces/create', 'POST', { name, path }),
  deleteCodeSpace: (name: string) =>
    request<{ success?: boolean }>('/api/code-spaces/delete', 'POST', { name }),
  codeQuery: (space: string, question: string) =>
    request<{ success: boolean; answer?: string; error?: string }>('/api/code/query', 'POST', { space, question }),
  codeSync: (space: string) =>
    request<{ success: boolean; task_id?: string; error?: string }>('/api/code/sync', 'POST', { space }),
  // Code 多轮会话（持久化在 <OKB_HOME>/code-chats/<space>/<id>.json）
  codeSessions: (space: string) =>
    request<{ ok: boolean; sessions?: Array<{ id: string; title: string; turn_count: number; updated_at: string }>; error?: string }>(
      `/api/code/sessions/${encodeURIComponent(space)}`,
    ),
  codeLoadSession: (space: string, sid: string) =>
    request<{ ok: boolean; session_id?: string; title?: string; messages?: Array<{ role: string; content: string; tools?: string[]; graph?: Array<{ category: string; name: string }>; follow_ups?: string[] }>; error?: string }>(
      `/api/code/session/${encodeURIComponent(space)}/${encodeURIComponent(sid)}`,
    ),
  codeDeleteSession: (space: string, sid: string) =>
    fetch(`/api/code/session/${encodeURIComponent(space)}/${encodeURIComponent(sid)}`, { method: 'DELETE' }).then(r => r.json()),
  codeSuggestions: (space: string, lang: string) =>
    request<{ suggestions: string[] }>(
      `/api/code/suggestions/${encodeURIComponent(space)}?lang=${encodeURIComponent(lang)}`,
    ),
  // 代码图谱：按需取某符号的 1 跳邻居（callers/callees）
  codeGraphNeighbors: (space: string, symbol: string) =>
    request<{ nodes: Array<{ id: string; label: string; kind: string; file?: string; line?: number; is_center?: boolean }>; edges: Array<{ source: string; target: string; type: string }> }>(
      `/api/code/graph/${encodeURIComponent(space)}?symbol=${encodeURIComponent(symbol)}`,
    ),
  // 点击图谱节点：取该符号的源码片段
  codeSymbolSource: (space: string, name: string) =>
    request<{ found: boolean; name?: string; kind?: string; qualified?: string; file?: string; start_line?: number; end_line?: number; docstring?: string; code?: string }>(
      `/api/code/symbol/${encodeURIComponent(space)}?name=${encodeURIComponent(name)}`,
    ),
  wikiPage: (space: string, category: string, page: string) =>
    request<{ content?: string; error?: string }>(
      `/api/wiki/${encodeURIComponent(space)}/${encodeURIComponent(category)}/${encodeURIComponent(page)}`,
    ),
  createSpace: (name: string, path?: string) =>
    request<{ success?: boolean; error?: string; status?: string }>('/api/spaces/create', 'POST', { name, path: path || '' }),
  spaceStatus: (name: string) => request<{ status: string; error?: string }>(`/api/space-status/${encodeURIComponent(name)}`),
  deleteSpace: (name: string) => request<{ success?: boolean }>('/api/spaces/delete', 'POST', { name }),
  query: (space: string, question: string) =>
    request<{ success: boolean; answer?: string; error?: string }>('/api/query', 'POST', { space, question }),
  removeDoc: (space: string, doc: string) =>
    request<{ success: boolean; task_id?: string; error?: string }>('/api/remove', 'POST', { space, doc }),
  addDoc: (space: string, path: string) =>
    request<{ success: boolean; error?: string }>('/api/add', 'POST', { space, path }),
  uploadFiles: (space: string, files: File[]) => {
    const form = new FormData()
    files.forEach(f => form.append('files', f))
    return fetch(`/api/upload/${encodeURIComponent(space)}`, { method: 'POST', body: form }).then(r => r.json())
  },
  getTask: (id: string) => request<TaskStatus>(`/api/task/${encodeURIComponent(id)}`),
  graph: (space: string) => request<GraphData>(`/api/graph/${encodeURIComponent(space)}`),
  // Deck: 异步生成 → 返回 task_id；列表/在线浏览/下载/删除
  createDeck: (space: string, name: string, intent: string, critique = false) =>
    request<{ success: boolean; task_id?: string; error?: string }>(
      '/api/deck', 'POST', { space, name, intent, critique },
    ),
  listDecks: (space: string) => request<DeckInfo[]>(`/api/decks/${encodeURIComponent(space)}`),
  deckUrl: (space: string, name: string, download = false) =>
    `/api/deck/${encodeURIComponent(space)}/${encodeURIComponent(name)}${download ? '?download=1' : ''}`,
  deleteDeck: (space: string, name: string) =>
    fetch(`/api/deck/${encodeURIComponent(space)}/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(r => r.json()),

  // Chat（多轮，复用 OpenKB chat session 文件 .openkb/chats/<id>.json）
  chatSessions: (space: string) =>
    request<{ ok: boolean; sessions?: Array<{ id: string; title: string; turn_count: number; updated_at: string }>; error?: string }>(
      `/api/chat/sessions/${encodeURIComponent(space)}`,
    ),
  chatLoadSession: (space: string, sid: string) =>
    request<{ ok: boolean; session_id?: string; title?: string; messages?: Array<{ role: string; content: string }>; error?: string }>(
      `/api/chat/session/${encodeURIComponent(space)}/${encodeURIComponent(sid)}`,
    ),
  chatDeleteSession: (space: string, sid: string) =>
    fetch(`/api/chat/session/${encodeURIComponent(space)}/${encodeURIComponent(sid)}`, { method: 'DELETE' }).then(r => r.json()),
  chatSend: (space: string, message: string, sessionId?: string) =>
    request<{ success: boolean; task_id?: string; error?: string }>(
      '/api/chat/send', 'POST', { space, message, session_id: sessionId || '' },
    ),
  browse: (path?: string) =>
    request<{ path: string; items: { name: string; is_dir: boolean; size: number }[] }>('/api/browse', 'POST', { path: path || '' }),

  // 推荐问题：按当前 locale 让后端基于 wiki concepts/entities 抽样生成
  suggestions: (space: string, lang: string) =>
    request<{ suggestions: string[] }>(
      `/api/suggestions/${encodeURIComponent(space)}?lang=${encodeURIComponent(lang)}`,
    ),

  // 设置（LLM API key 等），持久化在 <OKB_HOME>/config.json
  // GET 返回 mask 后的 key（"sk-***xxxx"），llm_has_key 标识是否已设；
  // POST 任意字段空字符串 = 保持不变；"__CLEAR__" = 显式清空
  getSettings: () =>
    request<{
      okb_home: string
      spaces_root: string
      okb_spec: string
      llm_api_key: string
      llm_has_key: boolean
      llm_base_url: string
      llm_model: string
      llm_language: string
      openkb_ready: boolean
      openkb_bin?: string
    }>('/api/settings'),
  updateSettings: (patch: Record<string, string>) =>
    request<{
      okb_home: string
      spaces_root: string
      llm_has_key: boolean
      llm_base_url: string
      llm_model: string
    }>('/api/settings', 'POST', patch),
  checkSettings: (draft?: Record<string, string>) =>
    request<{ ok: boolean; status?: number; model?: string; error?: string }>(
      '/api/settings/check', 'POST', draft || {},
    ),

  // Bootstrap 进度：启动时轮询（每 1s）直到 ready 或 failed
  bootstrapStatus: () =>
    request<{
      phase: 'pending' | 'checking' | 'download-uv' | 'installing' | 'releasing' | 'ready' | 'failed'
      message: string
      progress: number
      error?: string
      started_at?: string
      ended_at?: string
    }>('/api/bootstrap/status'),
  bootstrapRetry: () =>
    request<{ phase: string; message: string; progress: number }>('/api/bootstrap/retry', 'POST', {}),
}
