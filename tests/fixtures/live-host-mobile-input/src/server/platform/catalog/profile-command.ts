/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

export function profileAppServerCommand(): { command: string; args: string[] } {
  return { command: 'codex', args: ['app-server', '--stdio'] };
}
