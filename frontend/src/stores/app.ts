import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '../api/client'
import type { AppState, CommandResult, ConfigDocument, DaemonStatus, LogChunk, TestReport } from '../types'

export const useAppStore = defineStore('app', () => {
  const appState = ref<AppState | null>(null)
  const config = ref<ConfigDocument | null>(null)
  const configPath = ref('config.json')
  const status = ref<DaemonStatus>({ state: 'unknown' })
  const lastError = ref('')
  const lastMessage = ref('')
  const busy = ref(false)
  const testReport = ref<TestReport | null>(null)
  const logChunk = ref<LogChunk | null>(null)
  let messageTimer: number | undefined

  // VPN / corplink state
  const vpnConnected = ref(false)
  const vpnReconnecting = ref(false)
  const vpnNodeName = ref('')
  const vpnIP = ref('')
  const vpnDNS = ref('')
  const vpnProtocol = ref('')
  const vpnConnectedAt = ref(0)
  const isAuthenticated = ref(false)

  // Selected node — persisted to ~/.ecorplink/gui_state.json via Wails binding
  const selectedNodeId = ref<number | null>(null)
  const selectedNodeName = ref('')
  const selectedNodeLatency = ref(0)

  // Whether to respect server split-routes (true = split-tunnel, default)
  const followSplitRoutes = ref(true)

  const isRunning = computed(() => status.value.state === 'running')
  async function refresh() {
    try {
      appState.value = await api.getAppState()
      status.value = appState.value.status
      configPath.value = appState.value.config_path || configPath.value
      if (lastError.value.startsWith('后端调用失败：GetAppState')) {
        lastError.value = ''
      }
    } catch (error) {
      lastError.value = errorText(error)
    }
  }

  async function loadConfig(path = '') {
    const res = await api.loadConfig(path)
    if (!res.ok || !res.config) {
      lastError.value = res.error || '配置加载失败'
      return
    }
    config.value = res.config
    configPath.value = res.path
  }

  async function saveConfig() {
    if (!config.value) return false
    try {
      const res = await api.saveConfig(configPath.value, config.value)
      lastError.value = res.ok ? '' : (res.errors || []).join('\n')
      if (res.ok && isRunning.value) {
        const reload = await api.reloadConfig()
        if (!reload.ok) {
          lastError.value = reload.summary || reload.details || '配置已保存，但运行时重载失败'
          return false
        }
      }
      return res.ok
    } catch (error) {
      lastError.value = errorText(error)
      return false
    }
  }

  async function toggleDaemon() {
    busy.value = true
    lastError.value = ''
    const stopping = isRunning.value
    lastMessage.value = stopping ? '正在断开...' : '正在连接...'
    try {
      const res = stopping ? await api.stopDaemon() : await api.startDaemon(configPath.value)
      await refresh()
      if (res.ok) {
        lastError.value = ''
        lastMessage.value = !stopping && status.value.state === 'running'
          ? `已连接${status.value.pid ? ` · PID ${status.value.pid}` : ''}`
          : `已断开${res.summary ? `：${res.summary}` : ''}`
        clearMessageLater()
      } else {
        lastMessage.value = ''
        lastError.value = commandErrorText(res)
      }
    } catch (error) {
      lastMessage.value = ''
      lastError.value = errorText(error)
    } finally {
      busy.value = false
    }
  }

  async function runTests() {
    busy.value = true
    try {
      testReport.value = await api.runConnectivityTests()
    } finally {
      busy.value = false
    }
  }

  async function readLog(lines = 200) {
    logChunk.value = await api.readLog(lines)
  }

  async function refreshVPNStatus(): Promise<void> {
    try {
      const st = await api.getVPNStatus()
      vpnConnected.value = st.connected
      vpnReconnecting.value = st.reconnecting ?? false
      vpnNodeName.value = st.node_name || ''
      vpnIP.value = st.vpn_ip || ''
      vpnDNS.value = st.dns || ''
      vpnProtocol.value = st.protocol || ''
      vpnConnectedAt.value = st.connected_at || 0
    } catch {
      // ignore
    }
  }

  async function connectVPN(nodeID: number): Promise<boolean> {
    try {
      const res = await api.connectVPN(nodeID, followSplitRoutes.value)
      if (res.ok) {
        await refreshVPNStatus()
        return true
      }
      lastError.value = res.summary || '连接失败'
      return false
    } catch (error) {
      lastError.value = errorText(error)
      return false
    }
  }

  async function disconnectVPN(): Promise<void> {
    try {
      await api.disconnectVPN()
      vpnConnected.value = false
      vpnReconnecting.value = false
      vpnNodeName.value = ''
      vpnIP.value = ''
      vpnDNS.value = ''
      vpnProtocol.value = ''
      vpnConnectedAt.value = 0
    } catch (error) {
      lastError.value = errorText(error)
    }
  }

  function clearMessageLater() {
    if (messageTimer) window.clearTimeout(messageTimer)
    messageTimer = window.setTimeout(() => {
      lastMessage.value = ''
    }, 4000)
  }

  return {
    appState,
    config,
    configPath,
    status,
    lastError,
    lastMessage,
    busy,
    testReport,
    logChunk,
    isRunning,
    // VPN / corplink
    vpnConnected,
    vpnReconnecting,
    vpnNodeName,
    vpnIP,
    vpnDNS,
    vpnProtocol,
    vpnConnectedAt,
    isAuthenticated,
    // selected node
    selectedNodeId,
    selectedNodeName,
    selectedNodeLatency,
    followSplitRoutes,
    refresh,
    loadConfig,
    saveConfig,
    toggleDaemon,
    runTests,
    readLog,
    refreshVPNStatus,
    connectVPN,
    disconnectVPN,
  }
})

function commandErrorText(res: CommandResult): string {
  const summary = res.summary || '命令执行失败'
  if (!res.details || res.details === summary) {
    return summary
  }
  return `${summary}\n\n${res.details}`
}

function errorText(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  if (typeof error === 'string') {
    return error
  }
  return '操作失败'
}
