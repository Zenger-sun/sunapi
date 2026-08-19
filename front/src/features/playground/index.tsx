/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BookOpenIcon } from 'lucide-react'
import { toast } from 'sonner'
import { AuthenticatedLayout } from '@/components/layout'
import { getUserGroups, getUserModels } from './api'
import { AgentWorkspace } from './components/agent-workspace'
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
import { KnowledgeBaseManager } from './components/knowledge-base-manager'
import {
  useVideoDefaultParams,
  VideoNotice,
  VideoParamsPanel,
  VideoPromptTips,
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
  const [agentSeedPrompt, setAgentSeedPrompt] = useState('')
  const stopGenerationRef = useRef<(() => void) | null>(null)

  const imageModels = useMemo(
    () => models.filter((model) => model.supportsImage),
    [models]
  )
  const videoModels = useMemo(
    () => models.filter((model) => model.supportsVideo),
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

  useEffect(() => {
    if (activeCapability !== 'video') return
    const fallbackModel = videoModels[0]?.value
    if (!fallbackModel) return
    if (!videoModels.some((model) => model.value === config.model)) {
      updateConfig('model', fallbackModel)
    }
  }, [activeCapability, config.model, updateConfig, videoModels])

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

  const handleStartAgentTaskFromChat = useCallback((prompt: string) => {
    setAgentSeedPrompt(`${Date.now()}::${prompt}`)
    setActiveCapability('agent')
    toast.success(t('已从对话开启智能体任务草稿'))
  }, [t])

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
          <VideoPromptTips />
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
              activeSessionId={activeSessionId}
              isLoadingModels={isLoadingModels}
              onConfigChange={updateConfig}
              updateMessages={updateMessages}
              onGeneratingChange={setIsWorkspaceGenerating}
              onRegisterStop={(stop) => {
                stopGenerationRef.current = stop
              }}
              onStartAgentTask={handleStartAgentTaskFromChat}
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
              models={videoModels}
              groups={groups}
              isLoadingModels={isLoadingModels}
              onModelChange={(value) => updateConfig('model', value)}
              onGroupChange={(value) => updateConfig('group', value)}
              onGeneratingChange={setIsWorkspaceGenerating}
            />
          )}

          {activeCapability === 'knowledge' && (
            <div className='flex min-h-0 flex-1 flex-col p-3 md:p-4'>
              <div className='mb-3 flex shrink-0 items-center gap-2 px-1'>
                <div className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
                  <BookOpenIcon className='size-4' />
                </div>
                <div className='min-w-0'>
                  <h1 className='text-base font-semibold'>
                    {t('Knowledge Base')}
                  </h1>
                  <p className='text-muted-foreground text-xs'>
                    {t('Manage reusable documents for agent retrieval.')}
                  </p>
                </div>
              </div>
              <div className='border-border/60 bg-muted/20 flex min-h-0 flex-1 overflow-hidden rounded-2xl border'>
                <KnowledgeBaseManager />
              </div>
            </div>
          )}

          {activeCapability === 'agent' && (
            <AgentWorkspace
              groups={groups}
              models={models}
              currentModel={config.model}
              currentGroup={config.group}
              seedPrompt={agentSeedPrompt}
            />
          )}
        </section>
      </div>
    </AuthenticatedLayout>
  )
}
