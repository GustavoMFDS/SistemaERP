import { getSessionScope } from './auth'

// Non-sensitive display marker. Separate for each tenant and employee.
const PREFIX = 'esterium:fiscal-setup-reminder:v1:'
const fallback = new Set<string>()

function key(): string | null {
  const scope = getSessionScope()
  return scope ? PREFIX + encodeURIComponent(scope) : null
}

export function fiscalReminderAlreadyShown(): boolean {
  const id = key()
  if (!id) return true // No valid tenant and user: never show.
  if (fallback.has(id)) return true
  try { return sessionStorage.getItem(id) === 'shown' } catch { return false }
}

export function markFiscalReminderShown(): void {
  const id = key()
  if (!id) return
  fallback.add(id)
  try { sessionStorage.setItem(id, 'shown') } catch {
    // Still suppressed during navigation in this SPA if storage is disabled.
  }
}

export function resetFiscalReminderForLogin(): void {
  const id = key()
  if (!id) return
  fallback.delete(id)
  try { sessionStorage.removeItem(id) } catch {
    // No credential is stored in the reminder marker.
  }
}
