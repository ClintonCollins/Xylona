<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="row items-center">
        <q-btn flat dense round icon="arrow_back" aria-label="Back to nodes" to="/nodes" />
        <div class="xy-page-title q-ml-sm">Federation Activity</div>
      </div>
    </div>
    <federation-advisory-list />
  </q-page>
</template>

<script setup lang="ts">
import FederationAdvisoryList from '@/components/nodes/FederationAdvisoryList.vue'
</script>

<style scoped></style>
