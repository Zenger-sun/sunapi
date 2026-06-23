/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useCallback, useEffect, useRef } from 'react'
import {
  getPlaygroundSession,
  getPlaygroundSessions,
  savePlaygroundSession,
  deletePlaygroundSession,
  deleteAllPlaygroundSessions,
  updatePlaygroundSessionMeta,
} from '../api'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  loadConfig,
  saveConfig,
  loadParameterEnabled,
  saveParameterEnabled,
  loadMessages,
  saveMessages,
  loadActiveSessionId,
  saveActiveSessionId,
  sanitizeMessagesOnLoad,
  stripMessageImageData,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
  PlaygroundSession,
} from '../types'

function truncateText(text: string, maxLength: number) {
  const normalized = text.replace(/\s+/g, ' ').trim()
  const chars = Array.from(normalized)
  return chars.length > maxLength
    ? `${chars.slice(0, maxLength - 3).join('')}...`
    : normalized
}

function getMessageText(message: Message | undefined) {
  return message?.versions?.[0]?.content?.trim() || ''
}

function getSessionTitle(messages: Message[]) {
  const firstUserMessage = messages.find((message) => message.from === 'user')
  const content = getMessageText(firstUserMessage)
  if (content) {
    return truncateText(content, 80)
  }

  if (firstUserMessage?.attachments?.length) {
    return 'Image'
  }

  return 'Current conversation'
}

function getSessionSummary(messages: Message[]) {
  const lastMessageWithContent = [...messages]
    .reverse()
    .find((message) => getMessageText(message))
  const content = getMessageText(lastMessageWithContent)

  if (content) {
    return truncateText(content, 120)
  }

  const lastMessageWithAttachments = [...messages]
    .reverse()
    .find((message) => message.attachments?.length)
  if (lastMessageWithAttachments?.attachments?.length) {
    return 'Image'
  }

  return ''
}

function getSnapshotTitle(
  messages: Message[],
  sessionId: number | null,
  sessions: PlaygroundSession[]
) {
  if (messages.length > 0) {
    return getSessionTitle(messages)
  }

  const currentSession = sessionId
    ? sessions.find((session) => session.id === sessionId)
    : null

  return currentSession?.title || 'Current conversation'
}

function sanitizeSession(session: PlaygroundSession): PlaygroundSession {
  const messages = Array.isArray(session.messages)
    ? sanitizeMessagesOnLoad(session.messages)
    : undefined
  const config =
    session.config && typeof session.config === 'object' ? session.config : {}

  return {
    ...session,
    messages,
    config,
  }
}

function summarizeSession(session: PlaygroundSession): PlaygroundSession {
  const summary = { ...session }
  delete summary.messages
  return summary
}

function sortSessions(sessions: PlaygroundSession[]) {
  return [...sessions].sort((a, b) => {
    if (Boolean(a.pinned) !== Boolean(b.pinned)) {
      return Number(Boolean(b.pinned)) - Number(Boolean(a.pinned))
    }
    if (a.updated_time !== b.updated_time) {
      return b.updated_time - a.updated_time
    }
    return b.id - a.id
  })
}

function createSessionSummary(
  messages: Message[],
  config: PlaygroundConfig,
  sessionId: number | null,
  draftVersion: number,
  sessions: PlaygroundSession[]
): PlaygroundSession {
  const now = Math.floor(Date.now() / 1000)
  const currentSession = sessionId
    ? sessions.find((session) => session.id === sessionId)
    : null

  return {
    id: sessionId ?? -(draftVersion + 1),
    user_id: 1,
    title: getSnapshotTitle(messages, sessionId, sessions),
    summary: getSessionSummary(messages),
    model: config.model,
    group: config.group,
    pinned: currentSession?.pinned ?? false,
    message_count: messages.length,
    config,
    created_time: currentSession?.created_time ?? now,
    updated_time: now,
  }
}

/**
 * Main state management hook for playground
 */
export function usePlaygroundState() {
  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(() => {
    const savedConfig = loadConfig()
    return { ...DEFAULT_CONFIG, ...savedConfig }
  })
  const configRef = useRef<PlaygroundConfig>(config)

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    () => {
      const saved = loadParameterEnabled()
      return { ...DEFAULT_PARAMETER_ENABLED, ...saved }
    }
  )

  const [messages, setMessages] = useState<Message[]>(() => {
    return loadMessages() || []
  })
  const messagesRef = useRef<Message[]>(messages)

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])
  const [sessions, setSessions] = useState<PlaygroundSession[]>([])
  const [activeSessionId, setActiveSessionId] = useState<number | null>(() =>
    loadActiveSessionId()
  )
  const [previewSession, setPreviewSession] =
    useState<PlaygroundSession | null>(null)
  const [isLoadingPreviewSession, setIsLoadingPreviewSession] = useState(false)
  const [isLoadingSessions, setIsLoadingSessions] = useState(false)
  const [isSavingSession, setIsSavingSession] = useState(false)

  const sessionsRef = useRef<PlaygroundSession[]>([])
  const activeSessionIdRef = useRef<number | null>(activeSessionId)
  const draftVersionRef = useRef(0)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const saveInFlightRef = useRef(false)
  const savePromiseRef = useRef<Promise<void> | null>(null)
  const saveAgainRef = useRef(false)
  const ignoreCreatedSessionRef = useRef(false)
  const queuedSaveRef = useRef<{
    messages: Message[]
    config: PlaygroundConfig
    sessionId: number | null
    draftVersion: number
  } | null>(null)

  const replaceSessions = useCallback((nextSessions: PlaygroundSession[]) => {
    sessionsRef.current = nextSessions
    setSessions(nextSessions)
  }, [])

  const setActiveSession = useCallback((sessionId: number | null) => {
    activeSessionIdRef.current = sessionId
    setActiveSessionId(sessionId)
    saveActiveSessionId(sessionId)
  }, [])

  const upsertSession = useCallback(
    (session: PlaygroundSession, replaceSessionId?: number) => {
      const summary = summarizeSession(session)
      const nextSessions = sortSessions([
        summary,
        ...sessionsRef.current.filter(
          (item) => item.id !== session.id && item.id !== replaceSessionId
        ),
      ])
      replaceSessions(nextSessions)
    },
    [replaceSessions]
  )

  const applySession = useCallback(
    (session: PlaygroundSession) => {
      const sanitized = sanitizeSession(session)
      const nextMessages = sanitized.messages ?? []
      const nextConfig = {
        ...DEFAULT_CONFIG,
        ...sanitized.config,
        model:
          sanitized.config.model || sanitized.model || DEFAULT_CONFIG.model,
        group:
          sanitized.config.group || sanitized.group || DEFAULT_CONFIG.group,
      }

      draftVersionRef.current += 1
      configRef.current = nextConfig
      messagesRef.current = nextMessages
      setActiveSession(sanitized.id)
      setConfig(nextConfig)
      setMessages(nextMessages)
      saveConfig(nextConfig)
      saveMessages(nextMessages)
    },
    [setActiveSession]
  )

  const persistSessionSnapshot = useCallback(
    async (
      snapshotMessages: Message[],
      snapshotConfig: PlaygroundConfig,
      snapshotSessionId: number | null,
      snapshotDraftVersion: number,
      activateCreatedSession: boolean,
      replaceSessionId?: number
    ) => {
      if (snapshotMessages.length === 0 && !snapshotSessionId) return

      setIsSavingSession(true)
      try {
        const savedSession = sanitizeSession(
          await savePlaygroundSession({
            id: snapshotSessionId,
            title: getSnapshotTitle(
              snapshotMessages,
              snapshotSessionId,
              sessionsRef.current
            ),
            model: snapshotConfig.model,
            group: snapshotConfig.group,
            summary: getSessionSummary(snapshotMessages),
            messages: stripMessageImageData(snapshotMessages),
            config: snapshotConfig,
          })
        )

        const existingSession = snapshotSessionId
          ? sessionsRef.current.find(
              (session) => session.id === snapshotSessionId
            )
          : null
        upsertSession(
          existingSession
            ? {
                ...savedSession,
                title: existingSession.title,
                summary: existingSession.summary,
                model: existingSession.model,
                group: existingSession.group,
                created_time: existingSession.created_time,
              }
            : savedSession,
          replaceSessionId
        )

        if (
          !snapshotSessionId &&
          activateCreatedSession &&
          !ignoreCreatedSessionRef.current &&
          activeSessionIdRef.current === null &&
          draftVersionRef.current === snapshotDraftVersion
        ) {
          setActiveSession(savedSession.id)
          if (
            queuedSaveRef.current &&
            queuedSaveRef.current.draftVersion === snapshotDraftVersion &&
            queuedSaveRef.current.sessionId === null
          ) {
            queuedSaveRef.current = {
              ...queuedSaveRef.current,
              sessionId: savedSession.id,
            }
          }
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to save playground session:', error)
      } finally {
        setIsSavingSession(false)
      }
    },
    [setActiveSession, upsertSession]
  )

  const runQueuedSave = useCallback(async () => {
    if (saveInFlightRef.current) {
      saveAgainRef.current = true
      return
    }

    const queued = queuedSaveRef.current
    if (!queued || (queued.messages.length === 0 && !queued.sessionId)) return

    queuedSaveRef.current = null
    saveInFlightRef.current = true
    saveAgainRef.current = false

    const savePromise = persistSessionSnapshot(
      queued.messages,
      queued.config,
      queued.sessionId,
      queued.draftVersion,
      true
    )
    savePromiseRef.current = savePromise
    await savePromise

    saveInFlightRef.current = false
    savePromiseRef.current = null

    if (saveAgainRef.current) {
      saveAgainRef.current = false
      void runQueuedSave()
    }
  }, [persistSessionSnapshot])

  const scheduleSessionSave = useCallback(
    (nextMessages: Message[], nextConfig: PlaygroundConfig) => {
      if (nextMessages.length === 0 && !activeSessionIdRef.current) return

      queuedSaveRef.current = {
        messages: nextMessages,
        config: nextConfig,
        sessionId: activeSessionIdRef.current,
        draftVersion: draftVersionRef.current,
      }

      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
      }

      saveTimerRef.current = setTimeout(() => {
        void runQueuedSave()
      }, 700)
    },
    [runQueuedSave]
  )

  useEffect(() => {
    let cancelled = false

    async function loadSessions() {
      setIsLoadingSessions(true)
      try {
        const loadedSessions = (await getPlaygroundSessions()).map(
          sanitizeSession
        )
        if (cancelled) return

        const sortedSessions = sortSessions(loadedSessions)
        replaceSessions(sortedSessions)

        const activeSessionStillExists =
          activeSessionIdRef.current === null ||
          sortedSessions.some(
            (session) => session.id === activeSessionIdRef.current
          )

        if (!activeSessionStillExists) {
          setActiveSession(null)
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to load playground sessions:', error)
      } finally {
        if (!cancelled) {
          setIsLoadingSessions(false)
        }
      }
    }

    void loadSessions()

    return () => {
      cancelled = true
      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
      }
    }
  }, [replaceSessions, setActiveSession])

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        configRef.current = updated
        saveConfig(updated)
        scheduleSessionSave(messagesRef.current, updated)
        return updated
      })
    },
    [scheduleSessionSave]
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: Message[] | ((prev: Message[]) => Message[])) => {
      setMessages((prev) => {
        const newMessages =
          typeof updater === 'function' ? updater(prev) : updater
        messagesRef.current = newMessages
        saveMessages(newMessages)
        scheduleSessionSave(newMessages, configRef.current)
        return newMessages
      })
    },
    [scheduleSessionSave]
  )

  // Start a new conversation
  const clearMessages = useCallback(() => {
    const previousMessages = messagesRef.current
    const previousConfig = configRef.current
    const previousSessionId = activeSessionIdRef.current
    const previousDraftVersion = draftVersionRef.current

    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
    }

    if (previousMessages.length > 0) {
      const optimisticSession = createSessionSummary(
        previousMessages,
        previousConfig,
        previousSessionId,
        previousDraftVersion,
        sessionsRef.current
      )
      upsertSession(optimisticSession)
      void persistSessionSnapshot(
        previousMessages,
        previousConfig,
        previousSessionId,
        previousDraftVersion,
        false,
        optimisticSession.id
      )
    }

    draftVersionRef.current += 1
    messagesRef.current = []
    setActiveSession(null)
    setMessages([])
    saveMessages([])
  }, [persistSessionSnapshot, setActiveSession, upsertSession])

  const previewHistorySession = useCallback(
    async (sessionId: number) => {
      const sessionSummary = sessionsRef.current.find(
        (item) => item.id === sessionId
      )
      if (!sessionSummary) return

      setPreviewSession(sessionSummary)
      setIsLoadingPreviewSession(true)
      try {
        const session = sanitizeSession(await getPlaygroundSession(sessionId))
        upsertSession(session)
        setPreviewSession(session)
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to load playground session:', error)
        setPreviewSession(null)
      } finally {
        setIsLoadingPreviewSession(false)
      }
    },
    [upsertSession]
  )

  const closePreviewSession = useCallback(() => {
    setPreviewSession(null)
    setIsLoadingPreviewSession(false)
  }, [])

  const restoreSession = useCallback(
    (session: PlaygroundSession) => {
      const previousMessages = messagesRef.current
      const previousConfig = configRef.current
      const previousSessionId = activeSessionIdRef.current
      const previousDraftVersion = draftVersionRef.current

      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
        saveTimerRef.current = null
      }

      if (previousMessages.length > 0 || previousSessionId) {
        void persistSessionSnapshot(
          previousMessages,
          previousConfig,
          previousSessionId,
          previousDraftVersion,
          false
        )
      }

      applySession(session)
      closePreviewSession()
    },
    [applySession, closePreviewSession, persistSessionSnapshot]
  )

  const loadSession = useCallback(
    async (sessionId: number) => {
      const sessionSummary = sessionsRef.current.find(
        (item) => item.id === sessionId
      )
      if (!sessionSummary || sessionId <= 0) return
      if (
        activeSessionIdRef.current === sessionId &&
        messagesRef.current.length > 0
      )
        return

      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
        saveTimerRef.current = null
      }

      try {
        const session = sanitizeSession(await getPlaygroundSession(sessionId))
        upsertSession(session)
        applySession(session)
        closePreviewSession()
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to load playground session:', error)
      }
    },
    [applySession, closePreviewSession, upsertSession]
  )

  const renameSession = useCallback(
    async (sessionId: number, title: string) => {
      const trimmed = title.trim()
      if (!trimmed) return

      const previousSessions = sessionsRef.current
      replaceSessions(
        sortSessions(
          previousSessions.map((session) =>
            session.id === sessionId ? { ...session, title: trimmed } : session
          )
        )
      )

      try {
        const updated = sanitizeSession(
          await updatePlaygroundSessionMeta(sessionId, { title: trimmed })
        )
        upsertSession(updated)
      } catch (error) {
        replaceSessions(previousSessions)
        // eslint-disable-next-line no-console
        console.error('Failed to rename playground session:', error)
      }
    },
    [replaceSessions, upsertSession]
  )

  const togglePinSession = useCallback(
    async (sessionId: number) => {
      const previousSessions = sessionsRef.current
      const target = previousSessions.find(
        (session) => session.id === sessionId
      )
      if (!target) return

      const pinned = !target.pinned
      replaceSessions(
        sortSessions(
          previousSessions.map((session) =>
            session.id === sessionId ? { ...session, pinned } : session
          )
        )
      )

      try {
        const updated = sanitizeSession(
          await updatePlaygroundSessionMeta(sessionId, { pinned })
        )
        upsertSession(updated)
      } catch (error) {
        replaceSessions(previousSessions)
        // eslint-disable-next-line no-console
        console.error('Failed to pin playground session:', error)
      }
    },
    [replaceSessions, upsertSession]
  )

  const deleteSession = useCallback(
    async (sessionId: number) => {
      const previousSessions = sessionsRef.current
      replaceSessions(
        previousSessions.filter((session) => session.id !== sessionId)
      )
      if (previewSession?.id === sessionId) {
        closePreviewSession()
      }

      const wasActive = activeSessionIdRef.current === sessionId
      if (wasActive) {
        draftVersionRef.current += 1
        messagesRef.current = []
        setActiveSession(null)
        setMessages([])
        saveMessages([])
      }

      try {
        await deletePlaygroundSession(sessionId)
      } catch (error) {
        replaceSessions(previousSessions)
        // eslint-disable-next-line no-console
        console.error('Failed to delete playground session:', error)
      }
    },
    [closePreviewSession, previewSession?.id, replaceSessions, setActiveSession]
  )

  const clearHistory = useCallback(async () => {
    const previousSessions = sessionsRef.current
    const previousActiveSessionId = activeSessionIdRef.current
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
    }
    queuedSaveRef.current = null
    saveAgainRef.current = false

    replaceSessions([])
    closePreviewSession()

    if (activeSessionIdRef.current !== null) {
      setActiveSession(null)
    }
    ignoreCreatedSessionRef.current = true

    try {
      if (savePromiseRef.current) {
        await savePromiseRef.current
        replaceSessions([])
      }
      await deleteAllPlaygroundSessions()
    } catch (error) {
      replaceSessions(previousSessions)
      setActiveSession(previousActiveSessionId)
      // eslint-disable-next-line no-console
      console.error('Failed to clear playground sessions:', error)
    } finally {
      ignoreCreatedSessionRef.current = false
    }
  }, [closePreviewSession, replaceSessions, setActiveSession])

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    configRef.current = DEFAULT_CONFIG
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    scheduleSessionSave(messagesRef.current, DEFAULT_CONFIG)
  }, [scheduleSessionSave])

  useEffect(() => {
    sessionsRef.current = sessions
  }, [sessions])

  useEffect(() => {
    messagesRef.current = messages
  }, [messages])

  useEffect(() => {
    configRef.current = config
  }, [config])

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
      }
    }
  }, [])

  return {
    // State
    config,
    parameterEnabled,
    messages,
    models,
    groups,
    sessions,
    activeSessionId,
    previewSession,
    isLoadingPreviewSession,
    isLoadingSessions,
    isSavingSession,

    // Setters
    setModels,
    setGroups,

    // Actions
    updateConfig,
    updateParameterEnabled,
    updateMessages,
    clearMessages,
    loadSession,
    previewHistorySession,
    closePreviewSession,
    restoreSession,
    renameSession,
    deleteSession,
    clearHistory,
    togglePinSession,
    resetConfig,
  }
}
