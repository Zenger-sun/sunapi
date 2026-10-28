/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useEffect, useMemo, useState } from 'react'
import { DownloadIcon, FileIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  fetchAuthenticatedBlob,
  useAuthenticatedImageSource,
} from '@/lib/authenticated-media'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card'
import { Textarea } from '@/components/ui/textarea'
import {
  Branch,
  BranchMessages,
  BranchNext,
  BranchPage,
  BranchPrevious,
  BranchSelector,
} from '@/components/ai-elements/branch'
import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Loader } from '@/components/ai-elements/loader'
import { Message, MessageContent } from '@/components/ai-elements/message'
import {
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
} from '@/components/ai-elements/reasoning'
import { Response } from '@/components/ai-elements/response'
import { Shimmer } from '@/components/ai-elements/shimmer'
import {
  Source,
  Sources,
  SourcesContent,
  SourcesTrigger,
} from '@/components/ai-elements/sources'
import { MESSAGE_ROLES } from '../constants'
import { getMessageContentStyles } from '../lib/message-styles'
import { parseThinkTags } from '../lib/message-utils'
import type { Message as MessageType } from '../types'
import { MessageActions } from './message-actions'
import { MessageError } from './message-error'

interface PlaygroundChatProps {
  messages: MessageType[]
  onCopyMessage?: (message: MessageType) => void
  onRegenerateMessage?: (message: MessageType) => void
  onEditMessage?: (message: MessageType) => void
  onDeleteMessage?: (message: MessageType) => void
  isGenerating?: boolean
  editingKey?: string | null
  onSaveEdit?: (newContent: string) => void
  onCancelEdit?: (open: boolean) => void
  onSaveEditAndSubmit?: (newContent: string) => void
}

export function PlaygroundChat({
  messages,
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
  isGenerating = false,
  editingKey,
  onSaveEdit,
  onCancelEdit,
  onSaveEditAndSubmit,
}: PlaygroundChatProps) {
  const [editText, setEditText] = useState('')
  const [originalText, setOriginalText] = useState('')

  useEffect(() => {
    if (!editingKey) return
    const message = messages.find((m) => m.key === editingKey)
    const content = message?.versions?.[0]?.content || ''
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setEditText(content)

    setOriginalText(content)
  }, [editingKey, messages])

  const isEditing = (key: string) => editingKey === key
  const isEmpty = useMemo(() => !editText.trim(), [editText])
  const isChanged = useMemo(
    () => editText !== originalText,
    [editText, originalText]
  )
  return (
    <Conversation className='playground-chat-scroll size-full min-h-0'>
      {/* Remove outer padding; apply padding to inner centered container to align with input */}
      <ConversationContent className='p-0'>
        <div className='mx-auto w-full max-w-3xl px-4 py-4'>
          {messages.map((message, messageIndex) => {
            const { versions = [] } = message
            const isUserMessage = message.from === MESSAGE_ROLES.USER
            const isLastAssistantMessage =
              messageIndex === messages.length - 1 &&
              message.from === MESSAGE_ROLES.ASSISTANT
            return (
              <Branch defaultBranch={0} key={message.key}>
                <BranchMessages>
                  {versions.map((version, versionIndex) => (
                    <Message
                      className='group'
                      from={message.from}
                      key={`${message.key}-${version.id}-${versionIndex}`}
                    >
                      <div
                        className={cn(
                          'flex w-full min-w-0 flex-1 basis-full flex-col py-1',
                          isUserMessage ? 'items-end' : 'items-start'
                        )}
                      >
                        {isEditing(message.key) ? (
                          <div className='w-full space-y-2'>
                            <Textarea
                              value={editText}
                              onChange={(e) => setEditText(e.target.value)}
                              className='font-mono text-sm'
                              rows={8}
                            />
                            <div className='flex gap-2'>
                              {/* Save & Submit only makes sense for user messages */}
                              {message.from === MESSAGE_ROLES.USER && (
                                <Button
                                  size='sm'
                                  onClick={() =>
                                    onSaveEditAndSubmit?.(editText)
                                  }
                                  disabled={isEmpty || !isChanged}
                                >
                                  Save & Submit
                                </Button>
                              )}
                              <Button
                                size='sm'
                                onClick={() => onSaveEdit?.(editText)}
                                disabled={isEmpty || !isChanged}
                              >
                                Save
                              </Button>
                              <Button
                                size='sm'
                                variant='outline'
                                onClick={() => onCancelEdit?.(false)}
                              >
                                Cancel
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <>
                            {(() => {
                              const isAssistant =
                                message.from === MESSAGE_ROLES.ASSISTANT
                              const hasSources = !!message.sources?.length
                              const showReasoning =
                                isAssistant && !!message.reasoning?.content
                              const showLoader =
                                isAssistant &&
                                !message.isReasoningStreaming &&
                                (message.status === 'loading' ||
                                  (message.status === 'streaming' &&
                                    !version.content))
                              const showMessageContent =
                                (message.from === MESSAGE_ROLES.USER ||
                                  !message.isReasoningStreaming) &&
                                !!version.content
                              const imageAttachments =
                                message.attachments?.filter(
                                  (attachment) =>
                                    attachment.type === 'image' &&
                                    !attachment.omitted &&
                                    attachment.url.trim() !== ''
                                ) ?? []
                              const omittedImageCount =
                                message.attachments?.filter(
                                  (attachment) =>
                                    attachment.type === 'image' &&
                                    attachment.omitted
                                ).length ?? 0
                              const fileAttachments =
                                message.attachments?.filter(
                                  (attachment) =>
                                    attachment.type === 'file' &&
                                    !attachment.omitted
                                ) ?? []
                              const showAttachments =
                                message.from === MESSAGE_ROLES.USER &&
                                (imageAttachments.length > 0 ||
                                  omittedImageCount > 0 ||
                                  fileAttachments.length > 0)

                              // Extract visible content (remove <think> tags for assistant messages)
                              const displayContent = isAssistant
                                ? parseThinkTags(version.content).visibleContent
                                : version.content

                              const actions = (
                                <MessageActions
                                  message={message}
                                  onCopy={onCopyMessage}
                                  onRegenerate={onRegenerateMessage}
                                  onEdit={onEditMessage}
                                  onDelete={onDeleteMessage}
                                  isGenerating={isGenerating}
                                  alwaysVisible={isLastAssistantMessage}
                                  className='mt-1'
                                />
                              )

                              return (
                                <>
                                  {/* Sources */}
                                  {hasSources && (
                                    <Sources>
                                      <SourcesTrigger
                                        count={message.sources!.length}
                                      />
                                      <SourcesContent>
                                        {message.sources!.map(
                                          (source, sourceIndex) => (
                                            <Source
                                              href={source.href}
                                              key={`${message.key}-source-${sourceIndex}`}
                                              title={source.title}
                                            />
                                          )
                                        )}
                                      </SourcesContent>
                                    </Sources>
                                  )}

                                  {/* Reasoning */}
                                  {showReasoning && (
                                    <Reasoning
                                      defaultOpen={true}
                                      isStreaming={message.isReasoningStreaming}
                                    >
                                      <ReasoningTrigger />
                                      <ReasoningContent>
                                        {message.reasoning!.content}
                                      </ReasoningContent>
                                    </Reasoning>
                                  )}

                                  {/* Loader */}
                                  {showLoader && (
                                    <div className='flex items-center gap-2 py-2'>
                                      <Loader />
                                      <Shimmer className='text-sm' duration={1}>
                                        Responding...
                                      </Shimmer>
                                    </div>
                                  )}

                                  {/* Error or Content */}
                                  {message.status === 'error' ? (
                                    <>
                                      <MessageError
                                        message={message}
                                        className='mb-2'
                                      />
                                      {actions}
                                    </>
                                  ) : (
                                    (showMessageContent || showAttachments) && (
                                      <>
                                        {showAttachments && (
                                          <MessageImageAttachments
                                            attachments={imageAttachments}
                                            omittedCount={omittedImageCount}
                                          />
                                        )}
                                        {showAttachments &&
                                          fileAttachments.length > 0 && (
                                            <div className='mb-2 flex flex-wrap gap-2'>
                                              {fileAttachments.map(
                                                (attachment) => (
                                                  <MessageFileAttachment
                                                    attachment={attachment}
                                                    key={attachment.id}
                                                  />
                                                )
                                              )}
                                            </div>
                                          )}
                                        {showMessageContent && (
                                          <MessageContent
                                            variant='flat'
                                            className={cn(
                                              getMessageContentStyles()
                                            )}
                                          >
                                            <Response>
                                              {displayContent}
                                            </Response>
                                          </MessageContent>
                                        )}
                                        {actions}
                                      </>
                                    )
                                  )}
                                </>
                              )
                            })()}
                          </>
                        )}
                      </div>
                    </Message>
                  ))}
                </BranchMessages>

                {/* Branch selector for multiple versions */}
                {versions.length > 1 && (
                  <BranchSelector className='px-0' from={message.from}>
                    <BranchPrevious />
                    <BranchPage />
                    <BranchNext />
                  </BranchSelector>
                )}
              </Branch>
            )
          })}
        </div>
      </ConversationContent>
      <ConversationScrollButton />
    </Conversation>
  )
}

function MessageFileAttachment({
  attachment,
}: {
  attachment: NonNullable<MessageType['attachments']>[number]
}) {
  const { t } = useTranslation()
  const [isDownloading, setIsDownloading] = useState(false)

  const handleDownload = async () => {
    setIsDownloading(true)
    try {
      const blob = await fetchAuthenticatedBlob(attachment.url)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = attachment.filename || 'attachment'
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch {
      toast.error(t('Failed to download file'))
    } finally {
      setIsDownloading(false)
    }
  }

  return (
    <Button
      className='h-auto max-w-72 gap-2 px-3 py-2'
      disabled={isDownloading || !attachment.url}
      onClick={handleDownload}
      title={attachment.filename}
      variant='outline'
    >
      <FileIcon className='size-5 shrink-0' />
      <span className='min-w-0 text-left'>
        <span className='block truncate text-sm'>
          {attachment.filename || t('File')}
        </span>
        {attachment.size != null && (
          <span className='text-muted-foreground block text-xs'>
            {attachment.size < 1024 * 1024
              ? `${Math.ceil(attachment.size / 1024)} KB`
              : `${(attachment.size / (1024 * 1024)).toFixed(1)} MB`}
          </span>
        )}
      </span>
      <DownloadIcon
        aria-label={t('Download')}
        className='ml-1 size-4 shrink-0'
      />
    </Button>
  )
}

function MessageImageAttachments({
  attachments,
  omittedCount = 0,
}: {
  attachments: NonNullable<MessageType['attachments']>
  omittedCount?: number
}) {
  return (
    <div className='mb-2 flex flex-wrap justify-end gap-2'>
      {attachments.map((attachment, index) => {
        return (
          <MessageImageAttachment
            attachment={attachment}
            index={index}
            key={`${attachment.id}-${index}`}
          />
        )
      })}
      {omittedCount > 0 && (
        <div className='border-border bg-muted/30 text-muted-foreground flex h-20 min-w-20 items-center justify-center rounded-md border px-3 text-xs'>
          {omittedCount} image{omittedCount > 1 ? 's' : ''} omitted
        </div>
      )}
    </div>
  )
}

function MessageImageAttachment({
  attachment,
  index,
}: {
  attachment: NonNullable<MessageType['attachments']>[number]
  index: number
}) {
  const label = attachment.filename || `Image ${index + 1}`
  const src = useAuthenticatedImageSource(attachment.url)

  return (
    <HoverCard>
      <HoverCardTrigger
        delay={150}
        render={
          <button
            type='button'
            className='border-border bg-muted/30 hover:bg-muted h-20 w-20 overflow-hidden rounded-md border transition-colors'
            aria-label={label}
          >
            {src && (
              <img src={src} alt={label} className='size-full object-cover' />
            )}
          </button>
        }
      />
      <HoverCardContent className='w-auto p-2' align='end'>
        <div className='grid gap-2'>
          <div className='flex max-h-96 max-w-[min(28rem,80vw)] items-center justify-center overflow-hidden rounded-md border'>
            {src && (
              <img
                src={src}
                alt={label}
                className='max-h-96 max-w-full object-contain'
              />
            )}
          </div>
          <div className='text-muted-foreground max-w-96 truncate px-0.5 text-xs'>
            {label}
          </div>
        </div>
      </HoverCardContent>
    </HoverCard>
  )
}
