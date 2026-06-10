<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import {
  ClipboardList,
  FileText,
  FlaskConical,
  Home,
  Network,
  Settings,
} from '@lucide/vue'
import { Window } from '@wailsio/runtime'
import { api } from './api/client'
import { useAppStore } from './stores/app'
import LoginDialog from './components/LoginDialog.vue'
import NodeDialog from './components/NodeDialog.vue'
import OverviewPage from './components/OverviewPage.vue'
import RuleManager from './components/RuleManager.vue'
import ConfigPage from './components/ConfigPage.vue'
import TestsPage from './components/TestsPage.vue'
import LogsPage from './components/LogsPage.vue'
import SettingsPage from './components/SettingsPage.vue'
import type { VPNNode } from './types'

type PageKey = 'overview' | 'rules' | 'config' | 'tests' | 'logs' | 'settings'

const page = ref<PageKey>('overview')
const store = useAppStore()
const showLogin = ref(false)
const showNodes = ref(false)
const startingMsg = ref('正在启动 E-CorpLink 服务...')
const starting = ref(true)

let statusTimer: number | undefined
let saveWindowTimer: number | undefined

const nav = [
  { key: 'overview' as PageKey, label: '概览', icon: Home },
  { key: 'rules' as PageKey, label: '规则', icon: Network },
  { key: 'config' as PageKey, label: '配置', icon: ClipboardList },
  { key: 'tests' as PageKey, label: '测试', icon: FlaskConical },
  { key: 'logs' as PageKey, label: '日志', icon: FileText },
  { key: 'settings' as PageKey, label: '设置', icon: Settings },
]

onMounted(async () => {
  window.addEventListener('resize', saveWindowSizeSoon)
  saveWindowSizeSoon()

  // Load persisted window state (includes selected node + toggle)
  try {
    const ws = await api.getWindowState()
    if (ws.selected_node_id) {
      store.selectedNodeId = ws.selected_node_id
      store.selectedNodeName = ws.selected_node_name ?? ''
      store.selectedNodeLatency = ws.selected_node_latency ?? 0
    }
    if (ws.follow_split_routes !== undefined) {
      store.followSplitRoutes = ws.follow_split_routes
    }
  } catch { /* ignore outside Wails */ }

  await ensureDaemon()

  // Check VPN status
  try {
    const st = await api.getVPNStatus()
    if (st.connected) {
      store.vpnConnected = true
      store.vpnNodeName = st.node_name || ''
      store.vpnIP = st.vpn_ip || ''
    }
  } catch { /* ignore */ }

  // Check if already authenticated
  try {
    const authed = await api.isAuthenticated()
    if (authed) {
      store.isAuthenticated = true
    } else {
      showLogin.value = true
    }
  } catch {
    showLogin.value = true
  }

  // Load config and state
  try {
    await store.loadConfig()
    await store.refresh()
    await store.readLog(80)
  } catch { /* ignore */ }

  starting.value = false
  startPolling()
})

onUnmounted(() => {
  window.removeEventListener('resize', saveWindowSizeSoon)
  if (saveWindowTimer) window.clearTimeout(saveWindowTimer)
  if (statusTimer) window.clearInterval(statusTimer)
})

function saveWindowSizeSoon() {
  if (saveWindowTimer) window.clearTimeout(saveWindowTimer)
  saveWindowTimer = window.setTimeout(async () => {
    try {
      const size = await Window.Size()
      await api.saveWindowState({
        width: size.width,
        height: size.height,
        selected_node_id: store.selectedNodeId ?? undefined,
        selected_node_name: store.selectedNodeName || undefined,
        selected_node_latency: store.selectedNodeLatency || undefined,
        follow_split_routes: store.followSplitRoutes,
      })
    } catch { /* no-op outside Wails */ }
  }, 300)
}

// ── Daemon auto-start ─────────────────────────────────────────────────────────
async function ensureDaemon(): Promise<void> {
  try {
    const st = await api.getVPNStatus()
    if (st.ok !== false) return
  } catch { /* fall through */ }

  try {
    startingMsg.value = '正在检查服务状态...'
    const svc = await api.getLaunchServiceStatus()
    if (!svc.installed) {
      startingMsg.value = '首次启动，正在安装服务（需要管理员权限）...'
      await api.installLaunchService({ label: '', binary_path: '', config_path: '', work_dir: '' })
    } else if (svc.needs_update) {
      startingMsg.value = '检测到更新，正在重新安装服务（需要管理员权限）...'
      await api.installLaunchService({ label: '', binary_path: '', config_path: '', work_dir: '' })
    }
  } catch { /* ignore */ }

  startingMsg.value = '等待服务就绪...'
  for (let i = 0; i < 30; i++) {
    await new Promise(r => setTimeout(r, 500))
    try {
      const st = await api.getVPNStatus()
      if (st.ok !== false) return
    } catch { /* keep waiting */ }
  }
}

function startPolling() {
  if (statusTimer) window.clearInterval(statusTimer)
  statusTimer = window.setInterval(async () => {
    try {
      await store.refreshVPNStatus()
      await store.refresh()
    } catch { /* ignore */ }
  }, 5000)
}

async function onLoggedIn() {
  store.isAuthenticated = true
  showLogin.value = false
  // Refresh state after login
  try {
    await store.refresh()
    await store.loadConfig()
  } catch { /* ignore */ }
}

async function onNodeSelected(node: VPNNode) {
  store.selectedNodeId = node.id
  store.selectedNodeName = node.name
  store.selectedNodeLatency = node.latency_ms
  showNodes.value = false
  // Persist selected node to ~/.ecorplink/gui_state.json
  try {
    const size = await Window.Size()
    await api.saveWindowState({
      width: size.width,
      height: size.height,
      selected_node_id: node.id,
      selected_node_name: node.name,
      selected_node_latency: node.latency_ms,
      follow_split_routes: store.followSplitRoutes,
    })
  } catch { /* no-op outside Wails */ }
}

async function logout() {
  try { await api.logout() } catch { /* ignore */ }
  store.isAuthenticated = false
  store.vpnConnected = false
  store.vpnNodeName = ''
  store.vpnIP = ''
  showLogin.value = true
}
</script>

<template>
  <n-config-provider>
    <n-message-provider>

      <!-- Starting overlay -->
      <div v-if="starting" class="center-page">
        <div class="starting-card">
          <div class="starting-logo">E-CorpLink</div>
          <n-spin size="medium" />
          <p class="starting-msg">{{ startingMsg }}</p>
        </div>
      </div>

      <!-- Main shell (shown after daemon is ready) -->
      <div v-else class="app-shell">

        <!-- Login modal (not closable) -->
        <LoginDialog v-model="showLogin" @logged-in="onLoggedIn" />

        <!-- Node selection modal -->
        <NodeDialog v-model="showNodes" @selected="onNodeSelected" />

        <!-- Sidebar -->
        <aside class="sidebar">
          <!-- Brand -->
          <div class="brand">
            <span class="brand-text">E-CorpLink</span>
          </div>

          <!-- VPN status card -->
          <div v-if="store.isAuthenticated" class="vpn-card">
            <div class="vpn-status" :class="store.vpnConnected ? 'vpn-on' : 'vpn-off'">
              <span class="vpn-dot" />
              <span class="vpn-status-text">{{ store.vpnConnected ? store.vpnNodeName || '已连接' : '未连接' }}</span>
            </div>
            <div v-if="store.vpnIP" class="vpn-ip">{{ store.vpnIP }}</div>
            <div class="vpn-btns">
              <button
                v-if="store.vpnConnected"
                class="vpn-btn vpn-btn-disconnect"
                @click="store.disconnectVPN()"
              >
                断开
              </button>
              <button
                class="vpn-btn vpn-btn-switch"
                @click="showNodes = true"
              >
                {{ store.vpnConnected ? '切换节点' : '连接节点' }}
              </button>
            </div>
          </div>

          <!-- Divider -->
          <div class="sidebar-divider" />

          <!-- Navigation -->
          <nav class="sidebar-nav">
            <button
              v-for="item in nav"
              :key="item.key"
              class="nav-item"
              :class="{ active: page === item.key }"
              @click="page = item.key"
            >
              <component :is="item.icon" :size="24" class="nav-icon" />
              <span>{{ item.label }}</span>
            </button>
          </nav>

          <!-- Spacer -->
          <div class="sidebar-spacer" />
        </aside>

        <!-- Main content -->
        <main class="main">
          <div v-if="store.lastError" class="error-banner">
            <n-tag type="error" size="small">{{ store.lastError }}</n-tag>
          </div>

          <OverviewPage v-if="page === 'overview'" @open-nodes="showNodes = true" />
          <RuleManager v-else-if="page === 'rules'" />
          <ConfigPage v-else-if="page === 'config'" />
          <TestsPage v-else-if="page === 'tests'" />
          <LogsPage v-else-if="page === 'logs'" />
          <SettingsPage v-else-if="page === 'settings'" @logout="logout" />
        </main>
      </div>

    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
/* ── Shared ── */
.center-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: #f5f7fb;
}

.starting-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  width: 320px;
  padding: 40px 28px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 4px 24px rgba(0,0,0,0.10);
}

.starting-logo {
  font-size: 22px;
  font-weight: 800;
  color: #1a1a2e;
  letter-spacing: -0.5px;
}

.starting-msg {
  margin: 0;
  font-size: 13px;
  color: #666;
  text-align: center;
}

/* ── App shell ── */
.app-shell {
  display: grid;
  grid-template-columns: 80px minmax(0, 1fr);
  height: 100vh;
  overflow: hidden;
  background: #f7f9fc;
}

/* ── Sidebar ── */
.sidebar {
  width: 80px;
  background: #3d5af1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 16px 0;
  gap: 0;
  flex-shrink: 0;
  overflow: hidden;
}

.brand {
  color: white;
  font-weight: 700;
  font-size: 11px;
  text-align: center;
  padding: 8px 4px 20px;
  line-height: 1.3;
}

.brand-text {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.6px;
  color: #fff;
}

/* VPN card — hidden, VPN status shown in OverviewPage */
.vpn-card { display: none; }

/* Divider */
.sidebar-divider {
  display: none;
}

/* Nav */
.sidebar-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
}

.nav-item {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 12px 4px;
  border: none;
  border-radius: 0;
  background: none;
  color: rgba(255,255,255,0.7);
  cursor: pointer;
  font-size: 11px;
  font-weight: 500;
  transition: background 0.15s, color 0.15s;
}

.nav-item:hover {
  background: rgba(255,255,255,0.1);
  color: white;
}

.nav-item.active {
  background: rgba(255,255,255,0.2);
  color: white;
}

.nav-icon {
  flex-shrink: 0;
  opacity: 0.85;
}

.nav-item span {
  display: block !important;
  color: rgba(255, 255, 255, 0.8) !important;
  font-size: 11px;
  font-weight: 500;
  line-height: 1.2;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 76px;
}

.nav-item.active span {
  color: #fff !important;
}

.nav-item.active .nav-icon {
  opacity: 1;
}

.sidebar-spacer {
  flex: 1;
}

/* ── Main area ── */
.main {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  overflow-x: hidden;
  background: #f7f9fc;
}

.error-banner {
  padding: 6px 16px;
  flex-shrink: 0;
}
</style>
