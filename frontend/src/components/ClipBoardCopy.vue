<template>
  <div
    aria-label="Copy to clipboard"
    class="copy-clipboard"
    role="button"
    tabindex="0"
    @click="copyValue(props.clipBoardValue)"
    @keydown.enter="copyValue(props.clipBoardValue)"
    @keydown.space.prevent="copyValue(props.clipBoardValue)">
    {{ props.displayText }}
    <!-- eslint-disable vue/no-v-text-v-html-on-component, vue/no-v-html -- accepted per CLAUDE.md -->
    <q-tooltip
      :anchor="props.tooltipAnchor"
      :offset="[10, 10]"
      :self="props.tooltipSelf"
      class="clipboard-tooltip"
      @before-show="resetClipboardCopy"
      v-html="clipboardInnerHTML"></q-tooltip>
    <!-- eslint-enable vue/no-v-text-v-html-on-component, vue/no-v-html -->
  </div>
</template>

<script lang="ts" setup>
import { copyToClipboard, QTooltip } from 'quasar'
import { ref } from 'vue'

const props = defineProps({
  clipBoardValue: {
    type: String,
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
.clipboard-tooltip {
  font-family: var(--xy-font-brand);
  font-weight: 400;
  font-size: 0.65rem;
}
</style>
