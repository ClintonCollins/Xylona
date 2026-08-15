<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <game-form
        ref="formRef"
        :copy-game-id="copyGameId"
        :existing-game-id="existingGameId"></game-form>
    </div>
  </q-page>
</template>

<script lang="ts" setup>
import GameForm from '@/components/games/GameForm.vue'
import { useQuasar } from 'quasar'
import { computed, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'

const props = defineProps<{
  mode?: 'create' | 'edit' | 'copy'
}>()

const $q = useQuasar()
const route = useRoute()
const formRef = ref<InstanceType<typeof GameForm> | null>(null)
const gameID = computed(() => String(route.params['id'] ?? ''))
const existingGameId = computed(() => (props.mode === 'edit' ? gameID.value : ''))
const copyGameId = computed(() => (props.mode === 'copy' ? gameID.value : ''))

onBeforeRouteLeave(() => {
  if (!formRef.value?.isDirty) {
    return true
  }
  return new Promise<boolean>((resolve) => {
    $q.dialog({
      title: 'Unsaved Changes',
      message: 'You have unsaved changes. Are you sure you want to leave?',
      cancel: { flat: true, label: 'Stay' },
      ok: { color: 'negative', label: 'Discard Changes' },
      persistent: true,
    })
      .onOk(() => resolve(true))
      .onCancel(() => resolve(false))
      .onDismiss(() => resolve(false))
  })
})
</script>
