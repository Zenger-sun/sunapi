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
*/
import { useState, useCallback, useEffect, useRef } from 'react'
import { ArrowDownIcon, SparklesIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Lightbox } from '@/components/ui/lightbox'
import { useChatHandler, usePlaygroundState } from '../hooks'
import type {
  GroupOption,
  Message as MessageType,
  MessageAttachment,
  ModelOption,
} from '../types'
import { ChatSuggestions } from './chat-suggestions'
import { PlaygroundChat } from './playground-chat'
import { PlaygroundInput } from './playground-input'

interface ChatWorkspaceProps {
  config: ReturnType<typeof usePlaygroundState>['config']
  parameterEnabled: ReturnType<typeof usePlaygroundState>['parameterEnabled']
  messages: MessageType[]
  models: ModelOption[]
  groups: GroupOption[]
  isLoadingModels: boolean
  onConfigChange: <
    K extends keyof ReturnType<typeof usePlaygroundState>['config'],
  >(
    key: K,
    value: ReturnType<typeof usePlaygroundState>['config'][K]
  ) => void
  updateMessages: ReturnType<typeof usePlaygroundState>['updateMessages']
  onGeneratingChange?: (generating: boolean) => void
  onRegisterStop?: (stop: (() => void) | null) => void
}

export function ChatWorkspace({
  config,
  parameterEnabled,
  messages,
  models,
  groups,
  isLoadingModels,
  onConfigChange,
  updateMessages,
  onGeneratingChange,
  onRegisterStop,
}: ChatWorkspaceProps) {
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )
  const [lightboxSrc, setLightboxSrc] = useState<{
    src: string
    alt?: string
    filename?: string
  } | null>(null)
  const submitLockRef = useRef(false)

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

  useEffect(() => {
    onGeneratingChange?.(isGenerating)
  }, [isGenerating, onGeneratingChange])

  useEffect(() => {
    onRegisterStop?.(isGenerating ? stopGeneration : null)
  }, [isGenerating, stopGeneration, onRegisterStop])

  useEffect(() => {
    if (!isGenerating) {
      submitLockRef.current = false
    }
  })

  const handleSendMessage = useCallback(
    (text: string, attachments: MessageAttachment[] = []) => {
      if (isGenerating || submitLockRef.current) return
      submitLockRef.current = true

      const userMessage: MessageType = {
        key: `user-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        from: 'user',
        versions: [{ id: 'v1', content: text }],
        attachments,
      }
      const assistantMessage: MessageType = {
        key: `assistant-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        from: 'assistant',
        versions: [{ id: 'v1', content: '' }],
        status: 'loading',
      }
      updateMessages((prev) => [...prev, userMessage, assistantMessage])
      sendChat([...messages, userMessage, assistantMessage])
    },
    [isGenerating, messages, sendChat, updateMessages]
  )

  const handleRegenerate = useCallback(
    (message: MessageType) => {
      if (isGenerating || submitLockRef.current) return
      submitLockRef.current = true

      const idx = messages.findIndex((m) => m.key === message.key)
      if (idx === -1) return
      const loadingMessage: MessageType = {
        key: `assistant-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        from: 'assistant',
        versions: [{ id: 'v1', content: '' }],
        status: 'loading',
      }
      const next = [...messages.slice(0, idx), loadingMessage]
      updateMessages(next)
      sendChat(next)
    },
    [isGenerating, messages, sendChat, updateMessages]
  )

  const handleDelete = useCallback(
    (message: MessageType) => {
      updateMessages((prev) => prev.filter((m) => m.key !== message.key))
    },
    [updateMessages]
  )

  const handleApplyEdit = useCallback(
    (newContent: string, submit: boolean) => {
      if (!editingMessageKey) return
      if (submit && (isGenerating || submitLockRef.current)) return

      const idx = messages.findIndex((m) => m.key === editingMessageKey)
      if (idx === -1) {
        setEditingMessageKey(null)
        return
      }
      const updated = messages.map((m) =>
        m.key === editingMessageKey
          ? { ...m, versions: [{ ...m.versions[0], content: newContent }] }
          : m
      )
      setEditingMessageKey(null)
      if (!submit || updated[idx].from !== 'user') {
        updateMessages(updated)
        return
      }
      const loadingMessage: MessageType = {
        key: `assistant-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        from: 'assistant',
        versions: [{ id: 'v1', content: '' }],
        status: 'loading',
      }
      const toSubmit = [...updated.slice(0, idx + 1), loadingMessage]
      submitLockRef.current = true
      updateMessages(toSubmit)
      sendChat(toSubmit)
    },
    [editingMessageKey, isGenerating, messages, sendChat, updateMessages]
  )

  const isEmpty = messages.length === 0

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <div className='flex min-h-0 flex-1 overflow-hidden'>
        {isEmpty ? (
          <EmptyState onSelectSuggestion={(text) => handleSendMessage(text)} />
        ) : (
          <PlaygroundChat
            messages={messages}
            isGenerating={isGenerating}
            editingKey={editingMessageKey}
            onCancelEdit={(open) => {
              if (!open) setEditingMessageKey(null)
            }}
            onCopyMessage={() => undefined}
            onDeleteMessage={handleDelete}
            onEditMessage={(msg) => setEditingMessageKey(msg.key)}
            onRegenerateMessage={handleRegenerate}
            onSaveEdit={(content) => handleApplyEdit(content, false)}
            onSaveEditAndSubmit={(content) => handleApplyEdit(content, true)}
          />
        )}
      </div>

      <div className='bg-background shrink-0 px-3 pt-2 pb-3 md:px-6'>
        <div className='mx-auto w-full max-w-3xl'>
          <PlaygroundInput
            disabled={isGenerating}
            groupValue={config.group}
            groups={groups}
            isGenerating={isGenerating}
            isModelLoading={isLoadingModels}
            modelValue={config.model}
            models={models}
            onGroupChange={(value) => onConfigChange('group', value)}
            onModelChange={(value) => onConfigChange('model', value)}
            onStop={stopGeneration}
            onSubmit={handleSendMessage}
          />
        </div>
      </div>

      <Lightbox
        src={lightboxSrc?.src ?? null}
        alt={lightboxSrc?.alt}
        filename={lightboxSrc?.filename}
        onClose={() => setLightboxSrc(null)}
      />
    </div>
  )
}

function EmptyState({
  onSelectSuggestion,
}: {
  onSelectSuggestion: (text: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex size-full flex-col items-center px-4 py-6 text-center'>
      <div className='bg-muted text-foreground rounded-full px-3 py-1 text-xs font-medium'>
        {t('Current conversation')}
      </div>
      <div className='flex min-h-0 flex-1 items-center justify-center py-8'>
        <div className='flex w-full max-w-3xl flex-col items-center gap-5'>
          <div className='border-border bg-background text-foreground flex size-12 items-center justify-center rounded-full border shadow-sm'>
            <SparklesIcon className='size-5' />
          </div>
          <div className='space-y-2'>
            <h2 className='text-2xl font-semibold tracking-normal text-balance md:text-3xl'>
              {t('What would you like SunAPI to help with today?')}
            </h2>
            <p className='text-muted-foreground text-sm md:text-base'>
              {t('Pick a suggestion or type your question below.')}
            </p>
          </div>
          <ChatSuggestions onSelect={onSelectSuggestion} />
          <div className='text-muted-foreground flex items-center gap-1.5 text-xs md:text-sm'>
            <ArrowDownIcon className='size-3.5' />
            <span>{t('Scroll down to start typing')}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
