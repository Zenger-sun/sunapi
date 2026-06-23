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
import { useState } from 'react'
import {
  PaperclipIcon,
  FileIcon,
  ScreenShareIcon,
  CameraIcon,
  GlobeIcon,
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
  PromptInputActionAddAttachments,
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
}

const suggestions = [
  {
    icon: BarChartIcon,
    label: '分析数据',
    prompt: '分析最近的调用日志，找出异常趋势',
    color: '#76d0eb',
  },
  {
    icon: BoxIcon,
    label: '给我惊喜',
    prompt: '给我一个有意思的提示词',
    color: '#76d0eb',
  },
  {
    icon: NotepadTextIcon,
    label: '总结文本',
    prompt: '总结一下本月的渠道用量',
    color: '#ea8444',
  },
  {
    icon: CodeSquareIcon,
    label: '代码',
    prompt: '帮我写一个 React Hook',
    color: '#6c71ff',
  },
  {
    icon: GraduationCapIcon,
    label: '获取建议',
    prompt: '我应该用哪个模型来翻译文档？',
    color: '#76d0eb',
  },
  { icon: null, label: '更多', prompt: '给我更多提示建议' },
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
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [isUploading, setIsUploading] = useState(false)

  const isModelSelectDisabled =
    disabled || isUploading || isModelLoading || models.length === 0
  const isGroupSelectDisabled = disabled || isUploading || groups.length === 0

  const handleSubmit = async (message: PromptInputMessage) => {
    const text = message.text?.trim() ?? ''
    const imageFiles = (message.files ?? []).filter(
      (file) => file.mediaType?.startsWith('image/') && file.file
    )

    if ((!text && imageFiles.length === 0) || disabled || isUploading) return

    setIsUploading(true)
    try {
      const attachments = await Promise.all(
        imageFiles.map<Promise<MessageAttachment>>(async (file) => {
          const uploaded = await uploadPlaygroundAttachment(file.file!)
          return {
            id: uploaded.id,
            file_id: uploaded.file_id || uploaded.id,
            type: 'image',
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
        accept='image/*'
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
                <PromptInputActionAddAttachments label={t('Upload photo')} />
                <DropdownMenuItem onClick={() => handleFileAction('file')}>
                  <FileIcon className='mr-2' size={16} />
                  {t('Upload file')}
                </DropdownMenuItem>
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

            <PromptInputButton
              className='border font-medium'
              disabled={disabled || isUploading}
              onClick={() => toast.info(t('Search feature in development'))}
              variant='outline'
            >
              <GlobeIcon size={16} />
              <span className='hidden sm:inline'>{t('Search')}</span>
              <span className='sr-only sm:hidden'>{t('Search')}</span>
            </PromptInputButton>
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
        {suggestions.map(({ icon: Icon, label, prompt, color }) => (
          <Suggestion
            className={`text-xs font-normal sm:text-sm ${
              label === '更多' ? 'hidden sm:flex' : ''
            }`}
            key={label}
            onClick={() => handleSuggestionClick(prompt)}
            suggestion={prompt}
          >
            {Icon && <Icon size={16} style={{ color }} />}
            {label}
          </Suggestion>
        ))}
      </Suggestions>
    </div>
  )
}

function PlaygroundAttachmentPreview() {
  const attachments = usePromptInputAttachments()
  const imageAttachments = attachments.files.filter((file) =>
    file.mediaType?.startsWith('image/')
  )

  if (imageAttachments.length === 0) {
    return null
  }

  return (
    <PromptInputHeader className='px-3 pt-3 pb-0'>
      {imageAttachments.map((attachment) => (
        <PromptInputAttachment data={attachment} key={attachment.id} />
      ))}
    </PromptInputHeader>
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
  const hasImageAttachment = attachments.files.some((file) =>
    file.mediaType?.startsWith('image/')
  )

  return (
    <PromptInputButton
      className='text-foreground font-medium'
      disabled={disabled || (!text.trim() && !hasImageAttachment)}
      type='submit'
      variant='secondary'
    >
      <SendIcon size={16} />
      <span className='hidden sm:inline'>{t('Send')}</span>
      <span className='sr-only sm:hidden'>{t('Send')}</span>
    </PromptInputButton>
  )
}
