<!--
  SettingsPanel — 用户配置入口（实时自动保存版）。

  覆盖 LLM API 配置 + 知识库存储路径 + OpenKB spec。
  数据通过 GET /api/settings 拉，POST /api/settings 自动保存（debounce 600ms）。

  约定：
   - api_key 字段从后端拿到的是 mask 形式（sk-***xxxx）；
     用户**不动这个字段**就不发送 → 后端不会改 key（"" = 保持不变）。
     用户**输入新 key**会触发 debounce 自动保存。
     用户想**清除 key** → 点"清除"按钮，传 "__CLEAR__" 给后端。
   - 关闭抽屉只是右滑回去（emit('close')），表单值已经写盘（自动保存）。
   - 测试连接：POST /api/settings/check → 后端跑一次 chat completions 验证。
     输入框里的草稿带过去，可在保存生效前先验证。
-->
<template>
  <div class="settings-overlay" @click.self="$emit('close')">
    <aside class="settings-panel" role="dialog" aria-labelledby="settings-title">
      <header class="sp-head">
        <div>
          <div class="eyebrow">{{ t('settings.eyebrow') }}</div>
          <h2 id="settings-title" class="sp-title">{{ t('settings.title') }}</h2>
        </div>
        <div class="sp-head-right">
          <span class="autosave-indicator" :class="autosaveClass" :title="autosaveTitle">
            <span class="autosave-dot"></span>
            {{ autosaveLabel }}
          </span>
          <button class="sp-close" :title="t('common.close')" @click="$emit('close')">×</button>
        </div>
      </header>

      <div v-if="loading" class="sp-loading">
        <span class="loading-glyph">···</span> {{ t('common.loading') }}
      </div>

      <div v-else class="sp-body">
        <!-- §i. LLM 配置 -->
        <section class="sp-section">
          <h3 class="sp-h3">§i {{ t('settings.llmSection') }}</h3>
          <p class="sp-hint">{{ t('settings.llmHint') }}</p>

          <div class="form-group">
            <label>{{ t('settings.apiKey') }}</label>
            <div class="key-row">
              <input
                v-model="form.llm_api_key"
                type="password"
                :placeholder="form.llm_has_key ? maskedKey : t('settings.apiKeyPlaceholder')"
                autocomplete="off"
                spellcheck="false"
              />
              <button
                v-if="form.llm_has_key && !form.llm_api_key"
                class="btn btn-ghost btn-sm"
                @click="clearKey"
              >{{ t('settings.clearKey') }}</button>
            </div>
            <p class="form-help" v-if="form.llm_has_key && !form.llm_api_key">
              {{ t('settings.keyAlreadySet', { masked: maskedKey }) }}
            </p>
          </div>

          <div class="form-group">
            <label>{{ t('settings.baseUrl') }}</label>
            <input v-model="form.llm_base_url" type="text" :placeholder="t('settings.baseUrlPlaceholder')" />
            <p class="form-help">{{ t('settings.baseUrlHelp') }}</p>
          </div>

          <div class="form-group form-row">
            <div style="flex:2">
              <label>{{ t('settings.model') }}</label>
              <input v-model="form.llm_model" type="text" :placeholder="t('settings.modelPlaceholder')" />
            </div>
            <div style="flex:1">
              <label>{{ t('settings.language') }}</label>
              <select v-model="form.llm_language">
                <option value="zh">中文 (zh)</option>
                <option value="en">English (en)</option>
              </select>
            </div>
          </div>

          <div class="check-row">
            <button class="btn btn-sm" :disabled="checking" @click="doCheck">
              {{ checking ? t('settings.checkingBtn') : t('settings.checkBtn') }}
            </button>
            <span v-if="checkMsg" :class="['check-msg', checkOk ? 'ok' : 'err']">{{ checkMsg }}</span>
          </div>
        </section>

        <!-- §ii. 路径 / 高级 -->
        <section class="sp-section">
          <h3 class="sp-h3">§ii {{ t('settings.pathsSection') }}</h3>
          <div class="form-group">
            <label>{{ t('settings.okbHome') }}</label>
            <code class="readonly-path">{{ raw.okb_home }}</code>
            <p class="form-help">{{ t('settings.okbHomeHelp') }}</p>
          </div>
          <div class="form-group">
            <label>{{ t('settings.spacesRoot') }}</label>
            <input v-model="form.spaces_root" type="text" :placeholder="raw.spaces_root" />
            <p class="form-help">{{ t('settings.spacesRootHelp') }}</p>
          </div>
          <div class="form-group">
            <label>{{ t('settings.okbSpec') }}</label>
            <input v-model="form.okb_spec" type="text" :placeholder="raw.okb_spec" />
            <p class="form-help">{{ t('settings.okbSpecHelp') }}</p>
          </div>
        </section>

        <!-- §iii. 状态 -->
        <section class="sp-section sp-status">
          <h3 class="sp-h3">§iii {{ t('settings.statusSection') }}</h3>
          <div class="status-grid">
            <div>{{ t('settings.openkbReady') }}</div>
            <div>
              <span :class="['status-dot', raw.openkb_ready ? 'ok' : 'warn']"></span>
              {{ raw.openkb_ready ? t('settings.statusReady') : t('settings.statusInit') }}
            </div>
            <div>{{ t('settings.openkbBin') }}</div>
            <div><code class="readonly-path">{{ raw.openkb_bin || '—' }}</code></div>
          </div>
        </section>
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'

const { t } = useI18n()
defineEmits<{ close: [] }>()

const loading = ref(true)
const checking = ref(false)
const checkMsg = ref('')
const checkOk = ref(false)

// 自动保存状态：idle 干净 | dirty 有未保存改动 | saving 上传中 | saved 刚保存完 | error 失败
type AutosaveState = 'idle' | 'dirty' | 'saving' | 'saved' | 'error'
const autosaveState = ref<AutosaveState>('idle')
const autosaveError = ref('')

// raw 是 GET /api/settings 原样返回（含 mask 后的 key、只读字段）
const raw = reactive({
  okb_home: '',
  spaces_root: '',
  okb_spec: '',
  llm_api_key: '',
  llm_has_key: false,
  llm_base_url: '',
  llm_model: '',
  llm_language: '',
  openkb_ready: false,
  openkb_bin: '' as string | undefined,
})

// form 是用户编辑中的值。debounce 后自动 POST 给后端。
const form = reactive({
  llm_api_key: '',
  llm_base_url: '',
  llm_model: '',
  llm_language: 'zh',
  spaces_root: '',
  okb_spec: '',
  llm_has_key: false,  // mirror raw 给模板用
})

const maskedKey = computed(() => raw.llm_api_key || '')

const autosaveClass = computed(() => `as-${autosaveState.value}`)
const autosaveLabel = computed(() => {
  switch (autosaveState.value) {
    case 'idle': return t('settings.autosave.idle')
    case 'dirty': return t('settings.autosave.dirty')
    case 'saving': return t('settings.autosave.saving')
    case 'saved': return t('settings.autosave.saved')
    case 'error': return t('settings.autosave.error')
  }
})
const autosaveTitle = computed(() =>
  autosaveState.value === 'error' ? autosaveError.value : autosaveLabel.value,
)

async function load() {
  loading.value = true
  try {
    const res = await api.getSettings()
    Object.assign(raw, res)
    form.llm_base_url = res.llm_base_url
    form.llm_model = res.llm_model
    form.llm_language = res.llm_language || 'zh'
    form.spaces_root = ''
    form.okb_spec = ''
    form.llm_api_key = ''
    form.llm_has_key = res.llm_has_key
  } finally {
    loading.value = false
    // load 完才挂 watch，避免初始化触发 dirty
    setupAutosave()
  }
}

function clearKey() {
  if (!confirm(t('settings.clearKeyConfirm'))) return
  form.llm_api_key = '__CLEAR__'
  // watch 会自动触发保存
}

// debounce 600ms 自动保存
let saveTimer: number | null = null
let watchInited = false

function setupAutosave() {
  if (watchInited) return
  watchInited = true
  // 监听 form 6 个字段，任一变了就 mark dirty + 重置 timer
  watch(
    () => [
      form.llm_api_key,
      form.llm_base_url,
      form.llm_model,
      form.llm_language,
      form.spaces_root,
      form.okb_spec,
    ],
    () => {
      autosaveState.value = 'dirty'
      if (saveTimer != null) window.clearTimeout(saveTimer)
      saveTimer = window.setTimeout(doAutoSave, 600)
    },
  )
}

async function doAutoSave() {
  // 只发送有变化（非空）的字段；空 = "保持不变"（后端约定）
  const patch: Record<string, string> = {}
  if (form.llm_api_key) patch.llm_api_key = form.llm_api_key
  if (form.llm_base_url && form.llm_base_url !== raw.llm_base_url) patch.llm_base_url = form.llm_base_url
  if (form.llm_model && form.llm_model !== raw.llm_model) patch.llm_model = form.llm_model
  if (form.llm_language && form.llm_language !== raw.llm_language) patch.llm_language = form.llm_language
  if (form.spaces_root) patch.spaces_root = form.spaces_root
  if (form.okb_spec) patch.okb_spec = form.okb_spec
  if (Object.keys(patch).length === 0) {
    // 实际没差异，回到 idle（比如用户输入又删干净）
    autosaveState.value = 'idle'
    return
  }
  autosaveState.value = 'saving'
  try {
    const res = await api.updateSettings(patch)
    Object.assign(raw, res)
    // api_key/spaces_root/okb_spec 写完清空 input，让 placeholder 显示新的 saved 值
    if (patch.llm_api_key) form.llm_api_key = ''
    if (patch.spaces_root) form.spaces_root = ''
    if (patch.okb_spec) form.okb_spec = ''
    form.llm_has_key = res.llm_has_key
    autosaveState.value = 'saved'
    // 1.4s 后回到 idle，避免一直显示 "已保存" 占视觉
    window.setTimeout(() => {
      if (autosaveState.value === 'saved') autosaveState.value = 'idle'
    }, 1400)
  } catch (e: any) {
    autosaveState.value = 'error'
    autosaveError.value = e?.message || String(e)
  }
}

async function doCheck() {
  checking.value = true
  checkMsg.value = ''
  try {
    // 把当前 form 草稿带给后端：未保存的输入也能直接测试。
    const draft: Record<string, string> = {}
    if (form.llm_api_key && form.llm_api_key !== '__CLEAR__') draft.llm_api_key = form.llm_api_key
    if (form.llm_base_url) draft.llm_base_url = form.llm_base_url
    if (form.llm_model) draft.llm_model = form.llm_model
    const res = await api.checkSettings(draft)
    if (res.ok) {
      checkMsg.value = t('settings.checkPassed', { model: res.model || '' })
      checkOk.value = true
    } else {
      checkMsg.value = t('settings.checkFailed', { error: res.error || 'unknown' })
      checkOk.value = false
    }
  } catch (e: any) {
    checkMsg.value = t('settings.checkFailed', { error: e?.message || e })
    checkOk.value = false
  } finally {
    checking.value = false
  }
}

onMounted(load)
</script>


<style scoped>
.settings-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.32);
  z-index: 100;
  display: flex;
  justify-content: flex-end;
  /* 抽屉式从右滑入 */
  animation: fadeOverlay 200ms ease-out;
}
@keyframes fadeOverlay { from { opacity: 0; } to { opacity: 1; } }

.settings-panel {
  width: 480px;
  max-width: 92vw;
  height: 100vh;
  background: var(--paper);
  border-left: 1.5px solid var(--ink);
  display: flex;
  flex-direction: column;
  animation: slideIn 240ms cubic-bezier(0.2, 0.7, 0.2, 1);
}
@keyframes slideIn { from { transform: translateX(20px); } to { transform: translateX(0); } }

.sp-head {
  padding: 22px 28px 16px;
  border-bottom: 1.5px solid var(--ink);
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 12px;
}
.sp-title {
  font-family: var(--font-display);
  font-size: 26px;
  font-weight: 500;
  letter-spacing: -0.01em;
  margin-top: 4px;
  font-variation-settings: "opsz" 36, "SOFT" 30;
}
.sp-close {
  appearance: none; background: none; border: 0;
  font-family: var(--font-display);
  font-size: 26px;
  color: var(--ink-4);
  cursor: pointer;
  padding: 0 6px;
  transition: color 130ms ease;
}
.sp-close:hover { color: var(--vermilion); }

/* 实时保存状态指示器：右上角小圆点 + 文案 */
.sp-head-right {
  display: flex;
  align-items: center;
  gap: 14px;
}
.autosave-indicator {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--ink-4);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  user-select: none;
  transition: color 200ms ease;
}
.autosave-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ink-4);
  display: inline-block;
  transition: background 200ms ease;
}
.autosave-indicator.as-idle  .autosave-dot { background: var(--ink-4); }
.autosave-indicator.as-dirty .autosave-dot { background: var(--vermilion); animation: pulse 1.2s ease-in-out infinite; }
.autosave-indicator.as-saving .autosave-dot { background: var(--vermilion); animation: pulse 0.6s ease-in-out infinite; }
.autosave-indicator.as-saved { color: var(--moss, #6b8e23); }
.autosave-indicator.as-saved .autosave-dot { background: var(--moss, #6b8e23); }
.autosave-indicator.as-error { color: var(--vermilion); }
.autosave-indicator.as-error .autosave-dot { background: var(--vermilion); }

.sp-loading {
  flex: 1;
  display: flex; align-items: center; justify-content: center;
  font-family: var(--font-display);
  font-style: italic;
  color: var(--ink-3);
}

.sp-body {
  flex: 1;
  overflow-y: auto;
  padding: 22px 28px 12px;
}
.sp-section { margin-bottom: 28px; }
.sp-h3 {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 500;
  margin-bottom: 6px;
  font-variation-settings: "opsz" 20;
}
.sp-hint {
  font-family: var(--font-body);
  font-size: 12px;
  color: var(--ink-4);
  margin-bottom: 14px;
}
.form-group { margin-bottom: 14px; }
.form-group label {
  display: block;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-4);
  margin-bottom: 5px;
}
.form-group input, .form-group select {
  width: 100%;
  padding: 8px 11px;
  border: 1px solid var(--paper-edge);
  background: rgba(255, 252, 244, 0.5);
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--ink);
}
.form-group input:focus, .form-group select:focus {
  outline: none;
  border-color: var(--ink);
  background: var(--paper);
}
.form-row {
  display: flex;
  gap: 12px;
}
.form-help {
  font-family: var(--font-body);
  font-size: 11px;
  color: var(--ink-4);
  margin-top: 4px;
  line-height: 1.5;
}

.key-row { display: flex; gap: 8px; align-items: stretch; }
.key-row input { flex: 1; }

.check-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 6px;
}
.check-msg {
  font-family: var(--font-body);
  font-size: 12px;
}
.check-msg.ok { color: var(--moss); }
.check-msg.err { color: var(--vermilion); }

.readonly-path {
  display: block;
  font-family: var(--font-mono);
  font-size: 11.5px;
  background: var(--paper-2);
  padding: 6px 9px;
  border-left: 2px solid var(--paper-edge);
  word-break: break-all;
  color: var(--ink-2);
}

.sp-status .status-grid {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 8px 16px;
  font-family: var(--font-mono);
  font-size: 11.5px;
  align-items: center;
  color: var(--ink-3);
}
.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;
}
.status-dot.ok { background: var(--moss); }
.status-dot.warn { background: var(--vermilion); animation: pulse 1.6s ease-in-out infinite; }
@keyframes pulse { 0%, 100% { opacity: 0.5; } 50% { opacity: 1; } }
</style>
