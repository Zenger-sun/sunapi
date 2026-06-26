const ADMIN_SESSION_TOKEN_STORAGE_KEY = 'sunapi_admin_session_token'

export function saveAdminSessionToken(token: unknown) {
  if (typeof window === 'undefined') return
  if (typeof token === 'string' && token.trim()) {
    window.localStorage.setItem(
      ADMIN_SESSION_TOKEN_STORAGE_KEY,
      token.trim()
    )
  }
}

export function clearAdminSessionToken() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(ADMIN_SESSION_TOKEN_STORAGE_KEY)
}

export function getAdminSessionToken() {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(ADMIN_SESSION_TOKEN_STORAGE_KEY) ?? ''
}
