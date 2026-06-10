<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { Power } from '@lucide/vue'
import { Window } from '@wailsio/runtime'
import { api } from '../api/client'
import { useAppStore } from '../stores/app'

const emit = defineEmits<{ 'open-nodes': [] }>()
const store = useAppStore()

const connecting = ref(false)
const error = ref('')
const connectedAt = ref<number | null>(null)
const duration = ref('')
const txTotal = ref('')
const rxTotal = ref('')
const txRate = ref('')
const rxRate = ref('')
let durationTimer: number | undefined
let statsTimer: number | undefined
let prevTx = 0, prevRx = 0, prevStatsAt = 0

const ipDisplay = computed(() => store.vpnIP || '-')
const dnsDisplay = computed(() => store.vpnDNS || '-')
const protocolDisplay = computed(() => store.vpnProtocol || '-')
const durationDisplay = computed(() => duration.value || '-')
const txTotalDisplay = computed(() => txTotal.value || '总上传 -')
const rxTotalDisplay = computed(() => rxTotal.value || '总下载 -')
const txRateDisplay = computed(() => txRate.value || '↑ -')
const rxRateDisplay = computed(() => rxRate.value || '↓ -')

function formatDuration(ms: number): string {
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${m}m ${sec}s`
  if (m > 0) return `${m}m ${sec}s`
  return `${sec}s`
}

function formatRate(bytesPerSec: number): string {
  if (bytesPerSec < 1024) return `${bytesPerSec.toFixed(0)} B/s`
  if (bytesPerSec < 1024 * 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  return `${(bytesPerSec / 1024 / 1024).toFixed(2)} MB/s`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes.toFixed(0)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

async function pollStats() {
  try {
    const s = await api.getVPNStats()
    if (!s.ok) return
    const now = Date.now()
    txTotal.value = '总上传 ' + formatBytes(s.tx_bytes)
    rxTotal.value = '总下载 ' + formatBytes(s.rx_bytes)
    if (prevStatsAt > 0) {
      const dt = (now - prevStatsAt) / 1000
      if (dt > 0) {
        txRate.value = '↑ ' + formatRate((s.tx_bytes - prevTx) / dt)
        rxRate.value = '↓ ' + formatRate((s.rx_bytes - prevRx) / dt)
      }
    }
    prevTx = s.tx_bytes
    prevRx = s.rx_bytes
    prevStatsAt = now
  } catch { /* ignore */ }
}

function startTimers() {
  // Use daemon's connectedAt if available, otherwise fall back to now
  connectedAt.value = store.vpnConnectedAt > 0 ? store.vpnConnectedAt * 1000 : Date.now()
  duration.value = formatDuration(Date.now() - connectedAt.value)
  txTotal.value = ''
  rxTotal.value = ''
  txRate.value = ''
  rxRate.value = ''
  prevStatsAt = 0
  durationTimer = window.setInterval(() => {
    if (connectedAt.value) duration.value = formatDuration(Date.now() - connectedAt.value)
  }, 1000)
  statsTimer = window.setInterval(pollStats, 1000)
}

function stopTimers() {
  connectedAt.value = null
  duration.value = ''
  txTotal.value = ''
  rxTotal.value = ''
  txRate.value = ''
  rxRate.value = ''
  prevStatsAt = 0
  if (durationTimer) { window.clearInterval(durationTimer); durationTimer = undefined }
  if (statsTimer) { window.clearInterval(statsTimer); statsTimer = undefined }
}

watch(() => store.vpnConnected, (v) => {
  if (v) startTimers()
  else stopTimers()
}, { immediate: true })

// When we get a connectedAt from daemon (e.g. app re-opened while VPN already running),
// update the connectedAt so duration reflects actual connection time.
watch(() => store.vpnConnectedAt, (ts) => {
  if (ts > 0 && store.vpnConnected) {
    connectedAt.value = ts * 1000 // convert to ms
  }
})

onUnmounted(() => stopTimers())

const powerLabel = computed(() => {
  if (store.vpnConnected) return '已连接'
  if (store.vpnReconnecting) return '重连中...'
  if (connecting.value) return '连接中...'
  if (store.selectedNodeId) return '点击连接'
  return '未连接'
})

const serverDisplay = computed(() => {
  if (store.vpnConnected) return store.vpnNodeName
  if (store.selectedNodeId) {
    const lat = store.selectedNodeLatency > 0 ? ` ${store.selectedNodeLatency}ms` : ''
    return store.selectedNodeName + lat
  }
  return '点击选择节点'
})

async function handlePowerClick() {
  if (store.vpnConnected) {
    await store.disconnectVPN()
  } else if (store.selectedNodeId) {
    connecting.value = true
    error.value = ''
    try {
      const res = await api.connectVPN(store.selectedNodeId, store.followSplitRoutes)
      if (res.ok) {
        await store.refreshVPNStatus()
      } else {
        error.value = res.summary || '连接失败'
      }
    } finally {
      connecting.value = false
    }
  } else {
    emit('open-nodes')
  }
}

async function onFollowSplitRoutesChange(v: boolean) {
  store.followSplitRoutes = v
  // Apply in real-time if VPN is connected
  if (store.vpnConnected) {
    api.setFollowSplitRoutes(v).catch(() => {})
  }
  try {
    const size = await Window.Size()
    await api.saveWindowState({
      width: size.width,
      height: size.height,
      selected_node_id: store.selectedNodeId ?? undefined,
      selected_node_name: store.selectedNodeName || undefined,
      selected_node_latency: store.selectedNodeLatency || undefined,
      follow_split_routes: v,
    })
  } catch { /* no-op outside Wails */ }
}
</script>

<template>
  <section class="overview">
    <!-- 大圆形电源按钮 -->
    <div class="power-area">
      <div class="power-ring" :class="{ connected: store.vpnConnected }">
        <button
          class="power-btn"
          :class="{ connected: store.vpnConnected, connecting: connecting || store.vpnReconnecting }"
          :disabled="connecting || store.vpnReconnecting"
          @click="handlePowerClick"
        >
          <Power :size="42" />
          <span class="power-label">{{ powerLabel }}</span>
        </button>
      </div>
    </div>

    <!-- 当前服务器 -->
    <div v-if="store.isAuthenticated" class="server-row">
      <span class="server-label">当前服务器</span>
      <button class="server-btn" @click="emit('open-nodes')">
        <span class="server-name">{{ serverDisplay }}</span>
        <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
          <path d="M2 4l4 4 4-4" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/>
        </svg>
      </button>
    </div>

    <!-- 重连提示 -->
    <n-alert v-if="store.vpnReconnecting" type="warning" :bordered="false" class="reconnect-alert">
      WireGuard 隧道断开，正在自动重连...
    </n-alert>

    <!-- 底部信息 pills -->
    <div v-if="store.vpnConnected" class="info-pills">
      <span class="info-pill">IP: {{ ipDisplay }}</span>
      <span class="info-pill">DNS: {{ dnsDisplay }}</span>
      <span class="info-pill">传输协议: {{ protocolDisplay }}</span>
      <span class="info-pill">在线时长: {{ durationDisplay }}</span>
    </div>
    <div v-if="store.vpnConnected" class="info-pills speed-row">
      <span class="info-pill total-pill">{{ txTotalDisplay }}</span>
      <span class="info-pill total-pill">{{ rxTotalDisplay }}</span>
      <span class="info-pill speed-pill">{{ txRateDisplay }}</span>
      <span class="info-pill speed-pill">{{ rxRateDisplay }}</span>
    </div>

    <!-- 路由规则开关 -->
    <div class="toggle-row">
      <n-switch :value="store.followSplitRoutes" size="small" @update:value="onFollowSplitRoutesChange" />
      <span class="toggle-label">遵循路由规则</span>
      <span class="toggle-hint">{{ store.followSplitRoutes ? '按飞连路由分流' : '全部流量走飞连' }}</span>
    </div>

    <n-alert v-if="error" type="error" :bordered="false">{{ error }}</n-alert>
    <n-alert v-if="store.lastError" type="error" :bordered="false">{{ store.lastError }}</n-alert>
  </section>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  gap: 32px;
  padding: 48px 24px;
}

/* 外部虚线环（connected 时有旋转动画） */
.power-ring {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  border: 2px dashed #d0d5e8;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: border-color 0.3s;
}
.power-ring.connected {
  border-color: #4060e0;
  animation: spin 8s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 内圆按钮 */
.power-btn {
  width: 148px;
  height: 148px;
  border-radius: 50%;
  border: none;
  background: #d0d5e8;
  color: white;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  transition: background 0.3s, transform 0.1s;
  animation: none;
}
.power-btn:active { transform: scale(0.97); }
.power-btn.connected {
  background: #4060e0;
  animation: counter-spin 8s linear infinite;
}
@keyframes counter-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(-360deg); }
}

.power-label {
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

/* 服务器行 */
.server-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #666;
}
.server-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  background: none;
  border: none;
  cursor: pointer;
  color: #4060e0;
  font-size: 15px;
  font-weight: 600;
  padding: 0;
}
.server-btn:hover { text-decoration: underline; }

/* 底部 pills */
.info-pills {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}
.info-pill {
  background: #f0f2f8;
  color: #666;
  border-radius: 20px;
  padding: 6px 16px;
  font-size: 13px;
}
.speed-pill {
  font-family: monospace;
  background: #e8f0fe;
  color: #3d5af1;
}
.total-pill {
  font-family: monospace;
  background: #eef7f1;
  color: #287a45;
}

/* 路由规则开关 */
.toggle-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.toggle-label {
  color: #333;
  font-weight: 500;
}
.toggle-hint {
  color: #999;
}
.reconnect-alert {
  width: 100%;
  max-width: 400px;
}
</style>
