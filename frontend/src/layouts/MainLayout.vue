<template>
  <q-layout view="hHh Lpr lFf">
    <Navigation></Navigation>
    <q-page-container>
      <router-view/>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import Navigation from "@/components/Navigation.vue"
</script>
