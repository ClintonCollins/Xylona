<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="xy-page-title">Node Settings</div>
    </div>
    <node-api-key-settings />
    <q-separator class="q-my-lg" />
    <s-m-t-p-settings />
  </q-page>
</template>

<script setup lang="ts">
import NodeApiKeySettings from '@/components/admin/NodeApiKeySettings.vue'
import SMTPSettings from '@/components/admin/SMTPSettings.vue'
</script>

<style scoped></style>
