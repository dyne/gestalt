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
