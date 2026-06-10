<script setup lang="ts">
import { ref } from 'vue'
import * as Service from '../bindings/ecorplink/internal/gui/service'
import type { TestResult } from '../types'

const targets = [
  { name: 'Baidu',    url: 'https://www.baidu.com' },
  { name: 'Bing',     url: 'https://www.bing.com' },
  { name: 'QQ',       url: 'https://www.qq.com' },
  { name: 'Google',   url: 'https://www.google.com/generate_204' },
  { name: 'GitHub',   url: 'https://github.com' },
  { name: 'YouTube',  url: 'https://www.youtube.com/generate_204' },
  { name: 'X',        url: 'https://x.com' },
]

type RowState = 'idle' | 'running' | 'done'
interface Row {
  name: string
  url: string
  state: RowState
  result: TestResult | null
}

const rows = ref<Row[]>(targets.map(t => ({ name: t.name, url: t.url, state: 'idle', result: null })))
const running = ref(false)

async function runAll() {
  if (running.value) return
  running.value = true
  rows.value = targets.map(t => ({ name: t.name, url: t.url, state: 'running', result: null }))

  // Fire all tests concurrently; each updates its row as it completes.
  await Promise.all(targets.map(async (t, i) => {
    try {
      const res = await Service.RunSingleTest({ name: t.name, url: t.url })
      rows.value[i] = { name: t.name, url: t.url, state: 'done', result: res }
    } catch (e) {
      rows.value[i] = { name: t.name, url: t.url, state: 'done', result: { name: t.name, url: t.url, reachable: false, duration_millis: 0, error: String(e) } }
    }
  }))

  running.value = false
}
</script>

<template>
  <section class="page-stack">
    <div class="toolbar">
      <h2>连通性测试</h2>
      <n-button type="primary" :loading="running" @click="runAll">一键测试</n-button>
    </div>
    <div class="panel" style="overflow-x: auto">
      <table class="data-table">
        <thead>
          <tr>
            <th>名称</th>
            <th>目标</th>
            <th>状态</th>
            <th>HTTP</th>
            <th>耗时</th>
            <th>错误</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.name">
            <td>{{ row.name }}</td>
            <td class="url-cell">{{ row.url }}</td>
            <td>
              <n-tag v-if="row.state === 'idle'" size="small">待测试</n-tag>
              <n-spin v-else-if="row.state === 'running'" :size="14" />
              <n-tag v-else :type="row.result?.reachable ? 'success' : 'error'" size="small">
                {{ row.result?.reachable ? '可达' : '失败' }}
              </n-tag>
            </td>
            <td>{{ row.result?.http_status || (row.state === 'done' ? '--' : '') }}</td>
            <td>{{ row.result ? row.result.duration_millis + ' ms' : '' }}</td>
            <td class="err-cell">{{ row.result?.error || '' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.url-cell { font-size: 12px; color: #666; max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.err-cell { font-size: 11px; color: #e53e3e; max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
