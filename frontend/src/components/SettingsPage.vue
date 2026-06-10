<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api/client'
import { useAppStore } from '../stores/app'
import type { CommandResult, ServiceStatus } from '../types'

const emit = defineEmits<{
  (e: 'logout'): void
}>()

const store = useAppStore()
const service = ref<ServiceStatus | null>(null)
const result = ref('')
const resultDetails = ref('')
const version = ref('')
const cleanupBusy = ref(false)
const cleanupMsg = ref('')
const rulesBusy = ref(false)
const rulesMsg = ref('')
const socks5Msg = ref('')

onMounted(async () => {
  await refreshService()
  try { version.value = await api.getVersion() } catch { /* ignore */ }
})

async function refreshService() {
  service.value = await api.getLaunchServiceStatus()
}

async function installService() {
  const res = await api.installLaunchService({
    label: 'com.ecorplink.daemon',
    binary_path: store.appState?.daemon_path || '',
    config_path: store.configPath,
    work_dir: ''
  })
  setResult(res)
  await refreshService()
}

async function uninstallService() {
  const res = await api.uninstallLaunchService()
  setResult(res)
  await refreshService()
}

function setResult(res: CommandResult) {
  result.value = res.summary || res.details || ''
  resultDetails.value = res.details && res.details !== result.value ? res.details : ''
}

async function doCleanupRoutes() {
  cleanupBusy.value = true
  cleanupMsg.value = ''
  try {
    const res = await api.cleanupRoutes()
    cleanupMsg.value = res.ok ? '路由和 DNS 已重置' : (res.summary || '操作失败')
  } catch (e) {
    cleanupMsg.value = String(e)
  } finally {
    cleanupBusy.value = false
  }
}

async function doApplyRecommendedRules() {
  rulesBusy.value = true
  rulesMsg.value = ''
  try {
    const res = await api.applyRecommendedRules(store.configPath)
    rulesMsg.value = res.summary || (res.ok ? '完成' : '失败')
    if (res.ok) await store.loadConfig(store.configPath)
  } catch (e) {
    rulesMsg.value = String(e)
  } finally {
    rulesBusy.value = false
  }
}

async function saveSOCKS5() {
  socks5Msg.value = ''
  const ok = await store.saveConfig()
  socks5Msg.value = ok ? 'SOCKS5 设置已保存' : '保存失败'
}

async function doLogout() {
  try { await api.logout() } catch { /* ignore */ }
  emit('logout')
}
</script>

<template>
  <section class="page-stack">
    <div class="toolbar">
      <h2>设置</h2>
      <n-button @click="refreshService">刷新服务</n-button>
    </div>
    <div class="form-grid">
      <div class="panel">
        <h3>路径</h3>
        <div class="kv"><span>版本</span><strong>{{ version || '—' }}</strong></div>
        <div class="kv"><span>Daemon</span><strong>{{ store.appState?.daemon_path || '~/.ecorplink/bin/ecorplink-daemon' }}</strong></div>
        <div class="kv"><span>配置文件</span><strong>{{ store.configPath }}</strong></div>
        <div class="kv"><span>日志文件</span><strong>{{ store.appState?.log_path || '~/.ecorplink/ecorplink.log' }}</strong></div>
      </div>
      <div class="panel">
        <h3>系统服务</h3>
        <div class="kv"><span>安装状态</span><strong>{{ service?.installed ? '已安装' : '未安装' }}</strong></div>
        <div class="kv"><span>运行状态</span><strong>{{ service?.running ? '运行中' : '未运行' }}</strong></div>
        <div class="button-row service-actions">
          <n-button type="primary" @click="installService">安装</n-button>
          <n-button @click="uninstallService">卸载</n-button>
        </div>
        <p v-if="result" class="muted service-result">{{ result }}</p>
        <details v-if="resultDetails" class="command-details">
          <summary>查看详情</summary>
          <pre>{{ resultDetails }}</pre>
        </details>
      </div>
      <div class="panel">
        <h3>网络诊断</h3>
        <p class="muted hint">清除残留的 TUN 捕获路由（0/1、128/1）并重置系统 DNS，可在未连接时使用。</p>
        <div class="button-row">
          <n-button :loading="cleanupBusy" @click="doCleanupRoutes">重置路由 / DNS</n-button>
        </div>
        <p v-if="cleanupMsg" :class="['muted', 'service-result', cleanupMsg.includes('失败') ? 'err' : '']">
          {{ cleanupMsg }}
        </p>
        <n-divider style="margin: 12px 0" />
        <p class="muted hint">将当前配置的规则替换为内置推荐规则。</p>
        <div class="button-row">
          <n-popconfirm @positive-click="doApplyRecommendedRules">
            <template #trigger>
              <n-button :loading="rulesBusy">使用推荐规则</n-button>
            </template>
            将覆盖当前所有规则，确定继续？
          </n-popconfirm>
        </div>
        <p v-if="rulesMsg" :class="['muted', 'service-result', rulesMsg.includes('失败') ? 'err' : '']">
          {{ rulesMsg }}
        </p>
      </div>
      <div class="panel">
        <h3>SOCKS5</h3>
        <n-form v-if="store.config" label-placement="top">
          <n-form-item label="启用">
            <n-switch v-model:value="store.config.socks5.enabled" />
          </n-form-item>
          <n-form-item label="监听地址">
            <n-input v-model:value="store.config.socks5.bind_host" :disabled="!store.config.socks5.enabled" />
          </n-form-item>
          <n-form-item label="端口">
            <n-input-number
              v-model:value="store.config.socks5.port"
              :min="1"
              :max="65535"
              :disabled="!store.config.socks5.enabled"
            />
          </n-form-item>
          <div class="button-row">
            <n-button type="primary" @click="saveSOCKS5">保存</n-button>
          </div>
          <p v-if="socks5Msg" :class="['muted', 'service-result', socks5Msg.includes('失败') ? 'err' : '']">
            {{ socks5Msg }}
          </p>
        </n-form>
      </div>
    </div>
    <div class="logout-section">
      <n-button type="error" ghost @click="doLogout">退出登录</n-button>
    </div>
  </section>
</template>

<style scoped>
.logout-section {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid #e8ecf3;
}
.hint {
  font-size: 12px;
  margin: 0 0 10px;
}
.err { color: #d03050; }
</style>
