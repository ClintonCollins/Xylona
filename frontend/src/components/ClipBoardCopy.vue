<template>
  <div
    role="button"
    tabindex="0"
    aria-label="Copy to clipboard"
    class="copy-clipboard"
    @click="copyValue(props.clipBoardValue)"
    @keydown.enter="copyValue(props.clipBoardValue)"
    @keydown.space.prevent="copyValue(props.clipBoardValue)">
    {{ props.displayText }}
    <q-tooltip
      id="clipBoardCopyTooltip"
      :anchor="props.tooltipAnchor"
      :self="props.tooltipSelf"
      :offset="[10, 10]"
      @before-show="resetClipboardCopy"
      v-html="clipboardInnerHTML"></q-tooltip>
  </div>
</template>

<script setup lang="ts">
import { copyToClipboard, QTooltip } from 'quasar'
import { ref } from 'vue'

const props = defineProps({
  clipBoardValue: {
    type: String,
    required: true,
    default: '',
  },
  displayText: {
    type: String,
    default: '',
  },
  tooltipAnchor: {
    type: String,
    default: 'center right',
  },
  tooltipSelf: {
    type: String,
    default: 'center left',
  },
  clipBoardInnerHTML: {
    type: String,
    default: 'Copy to clipboard',
  },
  clipBoardSuccessInnerHTML: {
    type: String,
    default: 'Copied!',
  },
})

const clipboardInnerHTML = ref(props.clipBoardInnerHTML)

async function resetClipboardCopy() {
  clipboardInnerHTML.value = props.clipBoardInnerHTML
}

async function copyValue(value: string) {
  copyToClipboard(value)
    .then(() => {
      clipboardInnerHTML.value = props.clipBoardSuccessInnerHTML
    })
    .catch((e) => {
      clipboardInnerHTML.value = "<span style='color: var(--xy-danger)'>Error trying to copy</span>"
      console.error(e)
    })
}
</script>

<style>
#clipBoardCopyTooltip {
  font-family: var(--xy-font-brand);
  font-weight: 400;
  font-size: 0.65rem;
}
</style>
