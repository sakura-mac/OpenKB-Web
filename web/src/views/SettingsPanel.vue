<!--
  SettingsPanel — 用户配置入口。

  覆盖 LLM API 配置 + 知识库存储路径 + OpenKB spec。
  数据通过 GET /api/settings 拉，POST /api/settings 保存。

  约定：
   - api_key 字段从后端拿到的是 mask 形式（sk-***xxxx）；
     用户**不动这个字段**就提交 → 后端不会改 key（"" = 保持不变）。
     用户**输入新 key**就提交新值。
     用户想**清除 key** → 点"清除"按钮，传 "__CLEAR__" 给后端。
   - 测试连接：POST /api/settings/check → 后端实际跑一次 chat completions 验证。
-->
<template>
  <div class="settings-overlay" @click.self="$emit('close')">
    <aside class="settings-panel" role="dialog" aria-labelledby="settings-title">
      <header class="sp-head">
        <div>
          <div class="eyebrow">{{ t('settings.eyebrow') }}</div>
          <h2 id="settings-title" class="sp-title">{{ t('settings.title') }}</h2>
        </div>
        <button class="sp-close" :title="t('common.close')" @click="$emit('close')">×</button>
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

      <footer class="sp-foot">
        <button class="btn btn-ghost" @click="$emit('close')">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="doSave">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </footer>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'

const { t } = useI18n()
defineEmits<{ close: [] }>()

const loading = ref(true)
const saving = ref(false)
const checking = ref(false)
const checkMsg = ref('')
const checkOk = ref(false)

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

// form 是用户编辑中的值。提交时只发送有变化（非空）的字段；
// 空字符串 = "保持后端不变"（这是后端约定，前端 UI 通过 placeholder 显示原值即可）
const form = reactive({
  llm_api_key: '',
  llm_base_url: '',
  llm_model: '',
  llm_language: 'zh',
  spaces_root: '',
  okb_spec: '',
  // mirror raw 字段，给模板用
  llm_has_key: false,
})

const maskedKey = computed(() => raw.llm_api_key || '')

async function load() {
  loading.value = true
  try {
    const res = await api.getSettings()
    Object.assign(raw, res)
    // form 初始化为空字符串（让 placeholder 显示当前值）
    // 但 base_url/model/language 直接绑现值，便于直接编辑（这些不敏感）
    form.llm_base_url = res.llm_base_url
    form.llm_model = res.llm_model
    form.llm_language = res.llm_language || 'zh'
    form.spaces_root = ''
    form.okb_spec = ''
    form.llm_api_key = ''
    form.llm_has_key = res.llm_has_key
  } finally {
    loading.value = false
  }
}

function clearKey() {
  // 让用户能一键清除 API key（提示框确认后传 __CLEAR__ 给后端）
  if (!confirm(t('settings.clearKeyConfirm'))) return
  // 临时把 form.llm_api_key 设成 sentinel；下次保存或者立即提交都行
  form.llm_api_key = '__CLEAR__'
  doSave()
}

async function doSave() {
  saving.value = true
  checkMsg.value = ''
  try {
    const patch: Record<string, string> = {}
    if (form.llm_api_key) patch.llm_api_key = form.llm_api_key
    if (form.llm_base_url && form.llm_base_url !== raw.llm_base_url) patch.llm_base_url = form.llm_base_url
    if (form.llm_model && form.llm_model !== raw.llm_model) patch.llm_model = form.llm_model
    if (form.llm_language && form.llm_language !== raw.llm_language) patch.llm_language = form.llm_language
    if (form.spaces_root) patch.spaces_root = form.spaces_root
    if (form.okb_spec) patch.okb_spec = form.okb_spec
    if (Object.keys(patch).length === 0) {
      // 没改任何东西，提示用户
      checkMsg.value = t('settings.noChange')
      checkOk.value = true
      return
    }
    const res = await api.updateSettings(patch)
    Object.assign(raw, res)
    form.llm_api_key = ''
    form.llm_has_key = res.llm_has_key
    checkMsg.value = t('settings.saved')
    checkOk.value = true
  } catch (e: any) {
    checkMsg.value = t('settings.saveFailed', { e: e?.message || e })
    checkOk.value = false
  } finally {
    saving.value = false
  }
}

async function doCheck() {
  checking.value = true
  checkMsg.value = ''
  try {
    // 把当前 form 草稿带给后端：未保存的输入也能直接测试。
    // 空字段后端会回退到已保存值（保持兼容：未改任何东西也能测当前配置）。
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

.sp-foot {
  padding: 14px 28px 18px;
  border-top: 1.5px solid var(--ink);
  display: flex;
  gap: 12px;
  justify-content: flex-end;
}
</style>
