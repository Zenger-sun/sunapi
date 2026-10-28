/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useRef, useState, type ComponentType, type CSSProperties } from 'react'
import {
  PaperclipIcon,
  FileIcon,
  ImageIcon,
  ScreenShareIcon,
  CameraIcon,
  SendIcon,
  SquareIcon,
  BarChartIcon,
  BoxIcon,
  NotepadTextIcon,
  CodeSquareIcon,
  GraduationCapIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  PromptInput,
  PromptInputAttachment,
  PromptInputButton,
  PromptInputFooter,
  PromptInputHeader,
  PromptInputTextarea,
  PromptInputTools,
  usePromptInputAttachments,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'
import { Suggestion, Suggestions } from '@/components/ai-elements/suggestion'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { uploadPlaygroundAttachment } from '../api'
import type { MessageAttachment, ModelOption, GroupOption } from '../types'
import {
  KnowledgeScopeSelector,
  type KnowledgeScopeValue,
} from './knowledge-scope-selector'

interface PlaygroundInputProps {
  onSubmit: (text: string, attachments: MessageAttachment[]) => void
  onStop?: () => void
  disabled?: boolean
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  groups: GroupOption[]
  groupValue: string
  onGroupChange: (value: string) => void
  knowledgeScope: KnowledgeScopeValue
  onKnowledgeScopeChange: (next: KnowledgeScopeValue) => void
}

type PlaygroundSuggestion = {
  icon: ComponentType<{ size?: number; style?: CSSProperties }> | null
  labelKey: string
  promptKey: string
  color?: string
  hideOnMobile?: boolean
}

const suggestions: PlaygroundSuggestion[] = [
  {
    icon: BarChartIcon,
    labelKey: 'Analyze data',
    promptKey: 'Analyze recent usage logs and find unusual trends',
    color: '#76d0eb',
  },
  {
    icon: BoxIcon,
    labelKey: 'Surprise me',
    promptKey: 'Give me an interesting prompt',
    color: '#76d0eb',
  },
  {
    icon: NotepadTextIcon,
    labelKey: 'Summarize text',
    promptKey: 'Summarize this month channel usage',
    color: '#ea8444',
  },
  {
    icon: CodeSquareIcon,
    labelKey: 'Code',
    promptKey: 'Help me write a React Hook',
    color: '#6c71ff',
  },
  {
    icon: GraduationCapIcon,
    labelKey: 'Get advice',
    promptKey: 'Which model should I use to translate documents?',
    color: '#76d0eb',
  },
  {
    icon: null,
    labelKey: 'More',
    promptKey: 'Give me more prompt suggestions',
    hideOnMobile: true,
  },
]

export function PlaygroundInput({
  onSubmit,
  onStop,
  disabled,
  isGenerating,
  models,
  modelValue,
  onModelChange,
  isModelLoading = false,
  groups,
  groupValue,
  onGroupChange,
  knowledgeScope,
  onKnowledgeScopeChange,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [isUploading, setIsUploading] = useState(false)

  const isModelSelectDisabled =
    disabled || isUploading || isModelLoading || models.length === 0
  const isGroupSelectDisabled = disabled || isUploading || groups.length === 0

  const handleSubmit = async (message: PromptInputMessage) => {
    const text = message.text?.trim() ?? ''
    const files = (message.files ?? []).filter((file) => file.file)

    if ((!text && files.length === 0) || disabled || isUploading) return

    setIsUploading(true)
    try {
      const attachments = await Promise.all(
        files.map<Promise<MessageAttachment>>(async (file) => {
          const uploaded = await uploadPlaygroundAttachment(file.file!)
          return {
            id: uploaded.id,
            file_id: uploaded.file_id || uploaded.id,
            type: uploaded.type,
            url: uploaded.url,
            mediaType: uploaded.media_type || file.mediaType,
            filename: uploaded.filename || file.filename,
            size: uploaded.size,
          }
        })
      )

      onSubmit(text, attachments)
      setText('')
    } catch (error) {
      const errorMessage =
        error instanceof Error
          ? error.message
          : t('Failed to upload attachment')
      toast.error(errorMessage)
      throw error
    } finally {
      setIsUploading(false)
    }
  }

  const handleFileAction = (action: string) => {
    toast.info(t('Feature in development'), {
      description: action,
    })
  }

  const handleSuggestionClick = (suggestion: string) => {
    onSubmit(suggestion, [])
  }

  return (
    <div className='grid shrink-0 gap-3 px-1'>
      <PromptInput
        accept='image/*,.pdf,.docx,.xlsx,.txt,.md,.csv,.json,.html,.htm,.xml,.yaml,.yml,.log,.js,.jsx,.ts,.tsx,.py,.go,.java,.c,.cpp,.h,.css,.sql,.sh,.rs,.toml,.ini'
        globalDrop
        groupClassName='rounded-xl'
        maxFileSize={12 * 1024 * 1024}
        maxFiles={8}
        multiple
        onError={(error) => toast.error(error.message)}
        onSubmit={handleSubmit}
      >
        <PlaygroundAttachmentPreview />

        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='px-5 md:text-base'
          disabled={disabled || isUploading}
          onChange={(event) => setText(event.target.value)}
          placeholder={t('Ask anything')}
          value={text}
        />

        <PromptInputFooter className='p-2.5'>
          <PromptInputTools>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <PromptInputButton
                    className='border font-medium'
                    disabled={disabled || isUploading}
                    variant='outline'
                  />
                }
              >
                <PaperclipIcon size={16} />
                <span className='hidden sm:inline'>{t('Attach')}</span>
                <span className='sr-only sm:hidden'>{t('Attach')}</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='start'>
                <PlaygroundUploadAction imageOnly />
                <PlaygroundUploadAction />
                <DropdownMenuItem
                  onClick={() => handleFileAction('take-screenshot')}
                >
                  <ScreenShareIcon className='mr-2' size={16} />
                  {t('Take screenshot')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => handleFileAction('take-photo')}
                >
                  <CameraIcon className='mr-2' size={16} />
                  {t('Take photo')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            <KnowledgeScopeSelector
              value={knowledgeScope}
              onChange={onKnowledgeScopeChange}
              disabled={disabled || isUploading}
              toolbar
            />
          </PromptInputTools>

          <div className='flex items-center gap-1.5 md:gap-2'>
            <ModelGroupSelector
              selectedModel={modelValue}
              models={models}
              onModelChange={onModelChange}
              selectedGroup={groupValue}
              groups={groups}
              onGroupChange={onGroupChange}
              disabled={disabled || isUploading}
              modelDisabled={isModelSelectDisabled}
              groupDisabled={isGroupSelectDisabled}
            />

            {isGenerating && onStop ? (
              <PromptInputButton
                className='text-foreground font-medium'
                onClick={onStop}
                variant='secondary'
              >
                <SquareIcon className='fill-current' size={16} />
                <span className='hidden sm:inline'>{t('Stop')}</span>
                <span className='sr-only sm:hidden'>{t('Stop')}</span>
              </PromptInputButton>
            ) : (
              <PlaygroundSubmitButton
                disabled={disabled || isUploading || isModelSelectDisabled}
                text={text}
              />
            )}
          </div>
        </PromptInputFooter>
      </PromptInput>

      <Suggestions>
        {suggestions.map(
          ({ icon: Icon, labelKey, promptKey, color, hideOnMobile }) => (
            <Suggestion
              className={`text-xs font-normal sm:text-sm ${
                hideOnMobile ? 'hidden sm:flex' : ''
              }`}
              key={labelKey}
              onClick={() => handleSuggestionClick(t(promptKey))}
              suggestion={t(promptKey)}
            >
              {Icon && <Icon size={16} style={{ color }} />}
              {t(labelKey)}
            </Suggestion>
          )
        )}
      </Suggestions>
    </div>
  )
}

function PlaygroundAttachmentPreview() {
  const attachments = usePromptInputAttachments()

  if (attachments.files.length === 0) {
    return null
  }

  return (
    <PromptInputHeader className='px-3 pt-3 pb-0'>
      {attachments.files.map((attachment) => (
        <PromptInputAttachment data={attachment} key={attachment.id} />
      ))}
    </PromptInputHeader>
  )
}

function PlaygroundUploadAction({
  imageOnly = false,
}: {
  imageOnly?: boolean
}) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const inputRef = useRef<HTMLInputElement>(null)
  const Icon = imageOnly ? ImageIcon : FileIcon

  return (
    <>
      {imageOnly && (
        <input
          accept='image/*'
          aria-label={t('Upload photo')}
          className='hidden'
          multiple
          onChange={(event) => {
            if (event.currentTarget.files)
              attachments.add(event.currentTarget.files)
            event.currentTarget.value = ''
          }}
          ref={inputRef}
          type='file'
        />
      )}
      <DropdownMenuItem
        onSelect={(event) => {
          if (imageOnly) {
            event.preventDefault()
            inputRef.current?.click()
          } else attachments.openFileDialog()
        }}
      >
        <Icon className='mr-2' size={16} />
        {t(imageOnly ? 'Upload photo' : 'Upload file')}
      </DropdownMenuItem>
    </>
  )
}

function PlaygroundSubmitButton({
  disabled,
  text,
}: {
  disabled?: boolean
  text: string
}) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()

  return (
    <PromptInputButton
      className='text-foreground font-medium'
      disabled={disabled || (!text.trim() && attachments.files.length === 0)}
      type='submit'
      variant='secondary'
    >
      <SendIcon size={16} />
      <span className='hidden sm:inline'>{t('Send')}</span>
      <span className='sr-only sm:hidden'>{t('Send')}</span>
    </PromptInputButton>
  )
}
