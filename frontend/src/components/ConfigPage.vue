<script setup lang="ts">
import { watchEffect } from 'vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()

watchEffect(() => {
  if (!store.config) return
  store.config.dns.upstream ||= []
  store.config.dns.hijack ||= ['0.0.0.0:53']
  if (!store.config.dns.hijack.length) store.config.dns.hijack.push('0.0.0.0:53')
  store.config.corplink ||= { company_name: '', insecure_skip_verify: true, debug_http_body: false }
  store.config.corplink.insecure_skip_verify ??= true
  store.config.corplink.debug_http_body ??= false
})

function addDNS() {
  if (!store.config) return
  store.config.dns.upstream ||= []
  store.config?.dns.upstream.push('8.8.8.8:53')
}
</script>

<template>
  <section class="page-stack" v-if="store.config">
    <div class="toolbar">
      <h2>配置</h2>
      <n-button type="primary" @click="store.saveConfig">保存配置</n-button>
    </div>

    <div class="form-grid">
      <div class="panel">
        <h3>TUN</h3>
        <n-form label-placement="top">
          <n-form-item label="名称"><n-input v-model:value="store.config.tun.name" /></n-form-item>
          <n-form-item label="IP"><n-input v-model:value="store.config.tun.ip" /></n-form-item>
          <n-form-item label="Mask"><n-input-number v-model:value="store.config.tun.mask" :min="1" :max="32" /></n-form-item>
          <n-form-item label="MTU"><n-input-number v-model:value="store.config.tun.mtu" :min="576" :max="9000" /></n-form-item>
        </n-form>
      </div>

      <div class="panel">
        <h3>DNS / Fake IP</h3>
        <n-form label-placement="top">
          <n-form-item label="Fake IP 池"><n-input v-model:value="store.config.fakeip.pool" /></n-form-item>
          <n-form-item label="系统 DNS 劫持"><n-switch v-model:value="store.config.dns.system_hijack" /></n-form-item>
          <n-form-item label="劫持监听"><n-input v-model:value="store.config.dns.hijack[0]" /></n-form-item>
          <n-form-item label="上游 DNS">
            <div class="list-editor">
              <n-input v-for="(_, index) in store.config.dns.upstream" :key="index" v-model:value="store.config.dns.upstream[index]" />
              <n-button size="small" @click="addDNS">添加 DNS</n-button>
            </div>
          </n-form-item>
        </n-form>
      </div>

      <div class="panel">
        <h3>出口 / 日志</h3>
        <n-form label-placement="top">
          <n-form-item label="DIRECT 出口网卡"><n-input v-model:value="store.config.direct_outbound.interface" placeholder="留空自动探测" /></n-form-item>
          <n-form-item label="GeoIP 文件"><n-input v-model:value="store.config.geoip.file" placeholder="留空使用内嵌 mmdb" /></n-form-item>
          <n-form-item label="跳过 TLS 证书校验"><n-switch v-model:value="store.config.corplink.insecure_skip_verify" /></n-form-item>
          <n-form-item label="记录 HTTP Body"><n-switch v-model:value="store.config.corplink.debug_http_body" /></n-form-item>
          <n-form-item label="日志等级"><n-select v-model:value="store.config.log.level" :options="['debug', 'info', 'warn', 'error'].map((v) => ({ label: v, value: v }))" /></n-form-item>
          <n-form-item label="日志文件"><n-input v-model:value="store.config.log.file" /></n-form-item>
          <n-form-item label="保留天数"><n-input-number v-model:value="store.config.log.max_age" :min="1" :max="365" /></n-form-item>
        </n-form>
      </div>
    </div>
  </section>
</template>
