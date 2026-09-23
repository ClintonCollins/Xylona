<template>
  <section class="form-section form-section--last">
    <div class="section-header">
      <h2 class="section-title">Configuration Files</h2>
      <span class="section-line"></span>
    </div>
    <config-schema-list
      v-model="configSchemas"
      :can-edit-schemas="existingGame"
      @edit-schema="navigateToSchemaEditor" />
  </section>
</template>

<script lang="ts" setup>
import { inject } from 'vue'
import ConfigSchemaList from './ConfigSchemaList.vue'
import { gameFormContextKey } from './GameFormTypes'

const ctx = inject(gameFormContextKey)
if (!ctx) throw new Error('GameFormConfigTab must be used inside GameForm')

const { configSchemas, existingGame, navigateToSchemaEditor } = ctx
</script>
