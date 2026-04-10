<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <game-form ref="formRef" :copy-game-id="gameID"></game-form>
    </div>
  </q-page>
</template>

<script lang="ts" setup>
import GameForm from '@/components/games/GameForm.vue'
import { ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { useQuasar } from 'quasar'

const $q = useQuasar()
const route = useRoute()
const gameID = ref(route.params.id)
const formRef = ref<InstanceType<typeof GameForm> | null>(null)

onBeforeRouteLeave(() => {
  if (!formRef.value?.isDirty || formRef.value?.savedSuccessfully) {
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

<style scoped></style>
