<script setup lang="ts">
import { ref } from 'vue'
import { withBase } from 'vitepress'

const command = 'curl -fsSL https://dyne.github.io/gestalt/install.sh | bash'
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
    <p class="install-card__eyebrow">One command. One profile.</p>
    <h2 id="install-title">Install the complete Gestalt toolset</h2>
    <p>Installs the manager, Gestalt Agents, context mode, and Gestalt Mobile in user-owned directories.</p>
    <div class="install-card__command">
      <code>{{ command }}</code>
      <button type="button" @click="copyCommand">Copy</button>
    </div>
    <p class="visually-hidden" aria-live="polite">{{ status }}</p>
    <div class="install-card__links">
      <a :href="withBase('/start/install')">Read before installing</a>
      <a :href="withBase('/install.sh')" download>Download installer</a>
    </div>
  </section>
</template>
