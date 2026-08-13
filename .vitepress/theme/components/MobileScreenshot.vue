<script setup lang="ts">
import { computed } from 'vue'
import { withBase } from 'vitepress'

const props = withDefaults(
  defineProps<{
    src: string
    alt: string
    caption: string
    width?: number
    height?: number
    eager?: boolean
  }>(),
  { width: 390, height: 844, eager: false }
)

const imageUrl = computed(() => withBase(`/images/gestalt-mobile/${props.src}`))
</script>

<template>
<figure class="mobile-screenshot">
  <a :href="imageUrl" :aria-label="`Open full-size screenshot: ${caption}`">
    <img
      :src="imageUrl"
      :alt="alt"
      :width="width"
      :height="height"
      :loading="eager ? 'eager' : 'lazy'"
      :fetchpriority="eager ? 'high' : undefined"
    />
  </a>
  <figcaption>{{ caption }}</figcaption>
</figure>
</template>
