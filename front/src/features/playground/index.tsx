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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BotIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { AuthenticatedLayout } from '@/components/layout'
import { getUserGroups, getUserModels } from './api'
import { ChatWorkspace } from './components/chat-workspace'
import {
  ImageParamsPanel,
  ImagePromptTips,
  useImageDefaultParams,
} from './components/image-params-panel'
import { ImageWorkspace } from './components/image-workspace'
import {
  PlaygroundSidebar,
  type PlaygroundCapability,
} from './components/playground-sidebar'
import {
  useVideoDefaultParams,
  VideoNotice,
  VideoParamsPanel,
  VideoWorkspace,
} from './components/video-workspace'
import { usePlaygroundState } from './hooks'

export function Playground() {
  const { t } = useTranslation()
  const {
    config,
    parameterEnabled,
    messages,
    models,
    groups,
    sessions,
    activeSessionId,
    isLoadingSessions,
    isSavingSession,
    setModels,
    setGroups,
    updateConfig,
    updateMessages,
    clearMessages,
    loadSession,
    renameSession,
    deleteSession,
    togglePinSession,
  } = usePlaygroundState()

  const [activeCapability, setActiveCapability] =
    useState<PlaygroundCapability>('chat')
  const [isWorkspaceGenerating, setIsWorkspaceGenerating] = useState(false)
  const [imageParams, setImageParams] = useState(useImageDefaultParams)
  const [videoParams, setVideoParams] = useState(useVideoDefaultParams)
  const stopGenerationRef = useRef<(() => void) | null>(null)

  const imageModels = useMemo(
    () => models.filter((model) => model.supportsImage),
    [models]
  )

  const { data: modelsData, isLoading: isLoadingModels } = useQuery({
    queryKey: ['playground-models', config.group],
    queryFn: async () => {
      try {
        return await getUserModels(config.group)
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to load playground models')
        )
        return []
      }
    },
  })

  const { data: groupsData } = useQuery({
    queryKey: ['playground-groups'],
    queryFn: async () => {
      try {
        return await getUserGroups()
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to load playground groups')
        )
        return []
      }
    },
  })

  useEffect(() => {
    if (!modelsData) return
    setModels(modelsData)

    const currentModelExists = modelsData.some(
      (model) => model.value === config.model
    )
    const fallbackModel = modelsData[0]?.value
    if (fallbackModel && !currentModelExists) {
      updateConfig('model', fallbackModel)
    }
  }, [config.model, modelsData, setModels, updateConfig])

  useEffect(() => {
    if (!groupsData) return
    setGroups(groupsData)

    const currentGroupExists = groupsData.some(
      (group) => group.value === config.group
    )
    if (!currentGroupExists && groupsData.length > 0) {
      updateConfig(
        'group',
        groupsData.find((group) => group.value === 'default')?.value ??
          groupsData[0].value
      )
    }
  }, [config.group, groupsData, setGroups, updateConfig])

  useEffect(() => {
    setIsWorkspaceGenerating(false)
    stopGenerationRef.current = null
  }, [activeCapability])

  useEffect(() => {
    if (activeCapability !== 'image') return
    const fallbackModel = imageModels[0]?.value
    if (!fallbackModel) return
    if (!imageModels.some((model) => model.value === config.model)) {
      updateConfig('model', fallbackModel)
    }
  }, [activeCapability, config.model, imageModels, updateConfig])

  const handleNewChat = useCallback(() => {
    setActiveCapability('chat')
    clearMessages()
  }, [clearMessages])

  const handleSelectSession = useCallback(
    (sessionId: number) => {
      setActiveCapability('chat')
      void loadSession(sessionId)
    },
    [loadSession]
  )

  const hasConversation = messages.length > 0
  const hasDraftSession = activeSessionId === null && hasConversation

  const workspaceContent = useMemo(() => {
    if (activeCapability === 'image') {
      return (
        <>
          <ImageParamsPanel value={imageParams} onChange={setImageParams} />
          <ImagePromptTips />
        </>
      )
    }

    if (activeCapability === 'video') {
      return (
        <>
          <VideoNotice />
          <VideoParamsPanel value={videoParams} onChange={setVideoParams} />
        </>
      )
    }

    return null
  }, [activeCapability, imageParams, videoParams])

  return (
    <AuthenticatedLayout
      sidebarWidth='18.75rem'
      sidebar={
        <PlaygroundSidebar
          activeCapability={activeCapability}
          activeSessionId={activeSessionId}
          hasDraftSession={hasDraftSession}
          isGenerating={isWorkspaceGenerating}
          isLoadingSessions={isLoadingSessions}
          isSavingSession={isSavingSession}
          onCapabilityChange={setActiveCapability}
          onDeleteSession={(id) => void deleteSession(id)}
          onNewChat={handleNewChat}
          onRenameSession={(id, title) => void renameSession(id, title)}
          onSelectSession={handleSelectSession}
          onTogglePinSession={(id) => void togglePinSession(id)}
          sessions={sessions}
          workspaceContent={workspaceContent}
        />
      }
    >
      <div className='bg-muted/30 flex size-full min-h-0 p-2 md:p-4'>
        <section className='bg-background border-border/60 flex min-h-0 min-w-0 flex-1 overflow-hidden rounded-3xl border shadow-sm'>
          {activeCapability === 'chat' && (
            <ChatWorkspace
              config={config}
              parameterEnabled={parameterEnabled}
              messages={messages}
              models={models}
              groups={groups}
              isLoadingModels={isLoadingModels}
              onConfigChange={updateConfig}
              updateMessages={updateMessages}
              onGeneratingChange={setIsWorkspaceGenerating}
              onRegisterStop={(stop) => {
                stopGenerationRef.current = stop
              }}
            />
          )}

          {activeCapability === 'image' && (
            <ImageWorkspace
              params={imageParams}
              model={config.model}
              group={config.group}
              models={imageModels}
              groups={groups}
              isLoadingModels={isLoadingModels}
              onModelChange={(value) => updateConfig('model', value)}
              onGroupChange={(value) => updateConfig('group', value)}
              onGeneratingChange={setIsWorkspaceGenerating}
            />
          )}

          {activeCapability === 'video' && (
            <VideoWorkspace
              params={videoParams}
              model={config.model}
              group={config.group}
              onGeneratingChange={setIsWorkspaceGenerating}
            />
          )}

          {activeCapability === 'agent' && <AgentWorkspace />}
        </section>
      </div>
    </AuthenticatedLayout>
  )
}

function AgentWorkspace() {
  const { t } = useTranslation()

  return (
    <div className='flex size-full min-h-0 flex-col items-center justify-center px-4 py-12 text-center'>
      <div className='border-border/70 bg-muted/20 flex aspect-[4/3] w-full max-w-sm items-center justify-center rounded-2xl border-2 border-dashed'>
        <div className='text-muted-foreground flex flex-col items-center gap-2 text-sm'>
          <BotIcon className='size-6' />
          <span>{t('Agent workspace')}</span>
        </div>
      </div>
      <p className='text-muted-foreground mt-5 max-w-md text-xs leading-relaxed'>
        {t(
          'Multi-step agent orchestration is being planned. The placeholder keeps the layout aligned with the live experience.'
        )}
      </p>
    </div>
  )
}
