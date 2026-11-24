<script setup lang="ts">
import { ref } from 'vue'
import { withBase } from 'vitepress'

const command = 'npm i @jaromil/gestalt'
const status = ref('')

async function copyCommand() {
  try {
    await navigator.clipboard.writeText(command)
    status.value = 'Copied installation command'
  } catch {
    status.value = 'Select the command and copy it manually'
  }
}
</script>

<template>
  <section class="install-card" aria-labelledby="install-title">
    <h2 id="install-title">Install Gestalt with npm</h2>
    <p>Run <code>npx gestalt</code> to see the available commands.</p>
    <div class="install-card__command">
      <code>{{ command }}</code>
      <button type="button" @click="copyCommand">Copy</button>
    </div>
    <p class="visually-hidden" aria-live="polite">{{ status }}</p>
    <div class="install-card__links">
      <a :href="withBase('/start/')">Quick start</a>
      <a :href="withBase('/start/install')">Installation options</a>
    </div>
  </section>
</template>
