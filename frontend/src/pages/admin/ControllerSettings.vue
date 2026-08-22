<template>
  <q-page class="xy-page-content">
    <page-header
      subtitle="Configure services owned by this controller and shared across connected nodes."
      title="Controller Settings" />
    <email-settings />
  </q-page>
</template>

<script lang="ts" setup>
import EmailSettings from '@/components/admin/EmailSettings.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
</script>
