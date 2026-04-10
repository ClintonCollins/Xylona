<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="row items-center">
        <q-btn aria-label="Back to nodes" dense flat icon="arrow_back" round to="/nodes" />
        <div class="xy-page-title q-ml-sm">Federation Activity</div>
      </div>
    </div>
    <federation-advisory-list />
  </q-page>
</template>

<script lang="ts" setup>
import FederationAdvisoryList from '@/components/nodes/FederationAdvisoryList.vue'
</script>

<style scoped></style>
