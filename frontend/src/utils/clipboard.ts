// copyToClipboard centralises clipboard access. navigator.clipboard is only
// available in secure contexts (HTTPS or localhost); on plain-HTTP intranet
// deployments it is undefined, which used to silently throw inside .then()
// chains and surface nothing to the user. We fall back to the legacy
// execCommand path so "copy" still works on those setups.
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // fall through to legacy path
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    ta.setAttribute('readonly', '')
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
