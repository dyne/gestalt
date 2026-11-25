<script setup lang="ts">
import { ref } from 'vue'
import { VPButton } from 'vitepress/theme'

const command = 'npm install -g --prefix "$HOME/.local" --allow-scripts=@jaromil/gestalt @jaromil/gestalt'
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
    <h2 id="install-title">Install Gestalt for your user</h2>
    <p>On Linux or macOS, use Node.js 24+, npm, and the Codex CLI. This installs the command in <code>~/.local/bin</code> and sets up Gestalt Agents without <code>sudo</code>.</p>
    <div class="install-card__command">
      <code>{{ command }}</code>
      <button type="button" @click="copyCommand">Copy</button>
    </div>
    <p class="visually-hidden" aria-live="polite">{{ status }}</p>
    <p>If <code>~/.local/bin</code> is missing from your PATH, run <code>export PATH="$HOME/.local/bin:$PATH"</code> and add the same line to <code>~/.bashrc</code> (Bash) or <code>~/.zshrc</code> (Zsh).</p>
    <div class="install-card__links">
      <VPButton size="big" theme="brand" href="/start/" text="Quick start" />
      <VPButton size="big" theme="alt" href="/start/install" text="Installation options" />
    </div>
  </section>
</template>
