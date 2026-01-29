export const canUseClipboard = () => {
  if (typeof window === 'undefined') return false
  if (typeof window.isSecureContext === 'boolean') {
    return window.isSecureContext && Boolean(navigator?.clipboard?.writeText)
  }
  const protocol = window.location?.protocol
  const hostname = window.location?.hostname
  const isLocalhost = hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1'
  if (protocol === 'https:' || isLocalhost) {
    return Boolean(navigator?.clipboard?.writeText)
  }
  return false
}

export const copyToClipboard = async (text) => {
  if (!text) return false
  if (navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(String(text))
      return true
    } catch (err) {
      // Fall back to legacy clipboard handling.
    }
  }
  return copyToClipboardFallback(String(text))
}

const copyToClipboardFallback = (text) => {
  if (typeof document === 'undefined') return false
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.select()
    const ok = document.execCommand?.('copy')
    document.body.removeChild(textarea)
    return Boolean(ok)
  } catch (err) {
    return false
  }
}
