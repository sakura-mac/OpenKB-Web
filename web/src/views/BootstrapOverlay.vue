<!--
  BootstrapOverlay — 首次启动初始化遮罩。

  App.vue 启动时立即 mount 这个组件；它内部每 1s 拉 /api/bootstrap/status：
    - phase=pending/checking/download-uv/installing/releasing → 显示遮罩 + 进度条 + 文案
    - phase=ready → emit('done')，App.vue 撤掉遮罩进入正常 UI
    - phase=failed → 显示错误 + "重试"按钮（调 /api/bootstrap/retry）

  视觉：全屏纸面 + 朱红进度脉冲（与全站墨纸风格一致）。
-->
<template>
  <div v-if="visible" class="bs-overlay">
    <div class="bs-card">
      <div class="bs-eyebrow">{{ t('bootstrap.eyebrow') }}</div>
      <h1 class="bs-title">{{ t('bootstrap.title') }}</h1>

      <div v-if="phase !== 'failed'" class="bs-progress-wrap">
        <div class="bs-progress-track">
          <div
            class="bs-progress-fill"
            :style="{ width: progress + '%' }"
          ></div>
          <span class="bs-progress-shimmer" aria-hidden="true"></span>
        </div>
        <div class="bs-progress-num">{{ progress }}%</div>
      </div>

      <p :class="['bs-message', phase === 'failed' ? 'err' : '']">
        <span v-if="phase !== 'failed'" class="bs-dots" aria-hidden="true">
          <span></span><span></span><span></span>
        </span>
        <span class="bs-message-text">{{ message }}</span>
      </p>

      <div v-if="phase === 'failed'" class="bs-failed-actions">
        <p class="bs-error-detail">{{ error }}</p>
        <p class="bs-error-hint">{{ t('bootstrap.failedHint') }}</p>
        <div class="bs-action-row">
          <button class="btn btn-ghost" @click="openSettings">{{ t('bootstrap.openSettings') }}</button>
          <button class="btn btn-primary" :disabled="retrying" @click="retry">
            {{ retrying ? t('bootstrap.retryingBtn') : t('bootstrap.retryBtn') }}
          </button>
        </div>
      </div>

      <div class="bs-tips" v-if="phase !== 'failed' && elapsed > 30">
        <p>{{ t('bootstrap.slowHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'

const { t } = useI18n()
const emit = defineEmits<{
  done: []
  openSettings: []
}>()

const phase = ref<string>('pending')
const message = ref('')
const progress = ref(0)
const error = ref('')
const startedAt = ref<number>(Date.now())
const retrying = ref(false)

// 1s 间隔轮询。ready/failed 后停。
let timer: number | null = null

const visible = computed(() => phase.value !== 'ready')
const elapsed = computed(() => Math.floor((Date.now() - startedAt.value) / 1000))

async function tick() {
  try {
    const res = await api.bootstrapStatus()
    phase.value = res.phase
    progress.value = res.progress
    message.value = res.message || phaseDefaultMessage(res.phase)
    error.value = res.error || ''
    if (res.phase === 'ready') {
      stop()
      emit('done')
      return
    }
    if (res.phase === 'failed') {
      stop()
      return
    }
  } catch (e) {
    // HTTP 还没起来或网络抖动：继续轮询
    message.value = t('bootstrap.connecting')
  }
}

function phaseDefaultMessage(p: string): string {
  switch (p) {
    case 'pending': return t('bootstrap.phase.pending')
    case 'checking': return t('bootstrap.phase.checking')
    case 'download-uv': return t('bootstrap.phase.downloadUv')
    case 'installing': return t('bootstrap.phase.installing')
    case 'releasing': return t('bootstrap.phase.releasing')
    case 'ready': return t('bootstrap.phase.ready')
    case 'failed': return t('bootstrap.phase.failed')
    default: return p
  }
}

function start() {
  tick() // 立即先拉一次
  timer = window.setInterval(tick, 1000)
}
function stop() {
  if (timer != null) {
    window.clearInterval(timer)
    timer = null
  }
}

async function retry() {
  retrying.value = true
  try {
    await api.bootstrapRetry()
    error.value = ''
    phase.value = 'pending'
    progress.value = 0
    startedAt.value = Date.now()
    start()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    retrying.value = false
  }
}

function openSettings() {
  emit('openSettings')
}

onMounted(start)
onUnmounted(stop)
</script>

<style scoped>
/*
 * 遮罩布满整个视口，挡住 sidebar / topbar / content。z-index 要在 SettingsPanel(100) 之下，
 * 因为 failed 状态下用户可能要打开设置改 OKB_SPEC 然后重试。
 */
.bs-overlay {
  position: fixed;
  inset: 0;
  z-index: 90;
  background: var(--paper);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
  animation: bsFade 240ms ease-out;
}
@keyframes bsFade { from { opacity: 0; } to { opacity: 1; } }

.bs-card {
  max-width: 640px;
  width: 100%;
  text-align: center;
}

.bs-eyebrow {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-4);
  margin-bottom: 8px;
}

.bs-title {
  font-family: var(--font-display);
  font-size: 36px;
  font-weight: 500;
  letter-spacing: -0.015em;
  color: var(--ink);
  margin-bottom: 28px;
  font-variation-settings: "opsz" 60, "SOFT" 30;
}

/* 进度条：墨色细轨道 + 朱红填充 + 顶部流光 */
.bs-progress-wrap {
  display: flex;
  align-items: center;
  gap: 16px;
  margin: 0 auto 18px;
  max-width: 480px;
}
.bs-progress-track {
  flex: 1;
  position: relative;
  height: 4px;
  background: var(--paper-2);
  border-left: 1px solid var(--paper-edge);
  border-right: 1px solid var(--paper-edge);
  overflow: hidden;
}
.bs-progress-fill {
  height: 100%;
  background: var(--vermilion);
  transition: width 480ms cubic-bezier(0.2, 0.7, 0.2, 1);
}
.bs-progress-shimmer {
  position: absolute;
  top: 0;
  left: 0;
  height: 100%;
  width: 30%;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.6), transparent);
  animation: bsShimmer 1.6s linear infinite;
}
@keyframes bsShimmer {
  0%   { transform: translateX(-100%); }
  100% { transform: translateX(350%); }
}
.bs-progress-num {
  font-family: var(--font-mono);
  font-size: 13px;
  color: var(--ink-3);
  min-width: 38px;
  text-align: right;
}

/* 文案行 + 跳点 */
.bs-message {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-family: var(--font-display);
  font-style: italic;
  font-size: 15px;
  color: var(--ink-2);
  font-variation-settings: "opsz" 18, "SOFT" 80;
  margin-bottom: 0;
}
.bs-message.err {
  color: var(--vermilion);
}
.bs-dots {
  display: inline-flex;
  gap: 4px;
}
.bs-dots span {
  width: 5px; height: 5px; border-radius: 50%;
  background: var(--vermilion);
  display: inline-block;
  animation: bsJump 1.1s ease-in-out infinite;
}
.bs-dots span:nth-child(2) { animation-delay: 0.18s; background: var(--ink-3); }
.bs-dots span:nth-child(3) { animation-delay: 0.36s; background: var(--ink-4); }
@keyframes bsJump {
  0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
  30%           { transform: translateY(-4px); opacity: 1; }
}

/* 失败态：错误信息块 + 操作按钮 */
.bs-failed-actions {
  margin-top: 30px;
  text-align: left;
  background: var(--paper-2);
  padding: 18px 20px;
  border-left: 2px solid var(--vermilion);
}
.bs-error-detail {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--ink-2);
  margin: 0 0 10px;
  white-space: pre-wrap;
  word-break: break-all;
}
.bs-error-hint {
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--ink-3);
  margin: 0 0 14px;
  line-height: 1.6;
}
.bs-action-row {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

.bs-tips {
  margin-top: 24px;
  font-family: var(--font-body);
  font-size: 12px;
  color: var(--ink-4);
  line-height: 1.6;
}
</style>
