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
import { useEffect, useMemo, useState } from 'react'
import { api } from '@/lib/api'
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
                              const showAttachments =
                                message.from === MESSAGE_ROLES.USER &&
                                (imageAttachments.length > 0 ||
                                  omittedImageCount > 0)

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

function useAuthenticatedImageSource(url: string) {
  const [src, setSrc] = useState(() => (url.startsWith('/api/') ? '' : url))

  useEffect(() => {
    if (!url.startsWith('/api/')) {
      setSrc(url)
      return
    }

    let objectUrl: string | null = null
    let cancelled = false

    setSrc('')
    api
      .get(url, {
        responseType: 'blob',
        skipBusinessError: true,
        skipErrorHandler: true,
      })
      .then((response) => {
        if (cancelled) return
        objectUrl = URL.createObjectURL(response.data as Blob)
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
