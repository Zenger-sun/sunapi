import { useEffect, useState } from 'react'
import { getCommonHeaders } from '@/lib/api'

export function isLocalAPIURL(url: string) {
  if (url.startsWith('/api/')) return true
  if (typeof window === 'undefined') return false
  try {
    const parsed = new URL(url, window.location.origin)
    return parsed.origin === window.location.origin && parsed.pathname.startsWith('/api/')
  } catch {
    return false
  }
}

export async function fetchAuthenticatedBlob(url: string) {
  const response = await fetch(url, {
    ...(isLocalAPIURL(url) ? { headers: getCommonHeaders() } : {}),
  })
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`)
  }
  return response.blob()
}

export function useAuthenticatedImageSource(url: string) {
  const [src, setSrc] = useState(() => (isLocalAPIURL(url) ? '' : url))

  useEffect(() => {
    if (!isLocalAPIURL(url)) {
      setSrc(url)
      return
    }

    let objectUrl: string | null = null
    let cancelled = false

    setSrc('')
    fetchAuthenticatedBlob(url)
      .then((blob) => {
        if (cancelled) return
        objectUrl = URL.createObjectURL(blob)
        setSrc(objectUrl)
      })
      .catch(() => {
        if (!cancelled) {
          setSrc('')
        }
      })

    return () => {
      cancelled = true
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl)
      }
    }
  }, [url])

  return src
}
