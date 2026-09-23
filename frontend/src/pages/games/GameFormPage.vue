<template>
  <q-page class="xy-page-content">
    <game-form
      ref="formRef"
      :copy-game-id="copyGameId"
      :existing-game-id="existingGameId"></game-form>
  </q-page>
</template>

<script lang="ts" setup>
import GameForm from '@/components/games/GameForm.vue'
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'

import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'

const props = defineProps<{
  mode?: 'create' | 'edit' | 'copy'
}>()

const route = useRoute()
const formRef = ref<InstanceType<typeof GameForm> | null>(null)
const gameID = computed(() => String(route.params['id'] ?? ''))
const existingGameId = computed(() => (props.mode === 'edit' ? gameID.value : ''))
const copyGameId = computed(() => (props.mode === 'copy' ? gameID.value : ''))

useUnsavedChangesGuard(() => formRef.value?.isDirty ?? false)
</script>
