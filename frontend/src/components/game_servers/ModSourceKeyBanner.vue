<template>
  <q-banner class="bg-warning text-dark q-mb-md rounded-borders" dense>
    <template #avatar>
      <q-icon name="vpn_key" />
    </template>
    This source requires an API key. An admin can configure this in
    <router-link to="/admin/settings">Node Settings</router-link>.
  </q-banner>
</template>

<script lang="ts" setup></script>

<style scoped></style>
