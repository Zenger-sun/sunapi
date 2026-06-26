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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { XIcon, DownloadIcon, LinkIcon } from 'lucide-react'
import {
  fetchAuthenticatedBlob,
  useAuthenticatedImageSource,
} from '@/lib/authenticated-media'
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'

interface LightboxProps {
  src: string | null
  alt?: string
  filename?: string
  onClose: () => void
}

export function Lightbox({ src, alt, filename, onClose }: LightboxProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const imageSrc = useAuthenticatedImageSource(src ?? '')

  if (!src) return null

  const handleDownload = async () => {
    try {
      const blob = await fetchAuthenticatedBlob(src)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = filename || 'image'
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch {
      window.open(src, '_blank')
    }
  }

  const handleCopyLink = async () => {
    try {
      await navigator.clipboard.writeText(src)
      toast.success(t('Link copied'))
    } catch {
      toast.error(t('Failed to copy link'))
    }
  }

  return (
    <Dialog open={!!src} onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className='bg-background/95 max-w-[min(96vw,1100px)] gap-0 border-none p-0 shadow-2xl backdrop-blur'
      >
        <DialogTitle className='sr-only'>{alt || filename || 'Image'}</DialogTitle>
        <div className='relative flex max-h-[92vh] items-center justify-center p-3'>
          {loading && (
            <div className='bg-muted/40 absolute inset-3 animate-pulse rounded-md' />
          )}
          {imageSrc && (
            <img
              src={imageSrc}
              alt={alt || filename || 'Image'}
              onLoad={() => setLoading(false)}
              onError={() => setLoading(false)}
              className='max-h-[88vh] max-w-full rounded-md object-contain'
            />
          )}
        </div>
        <div className='flex items-center gap-2 border-t px-4 py-2.5'>
          <div className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
            {filename || alt}
          </div>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={handleCopyLink}
            className='gap-1.5'
          >
            <LinkIcon className='size-3.5' />
            {t('Copy link')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={handleDownload}
            className='gap-1.5'
          >
            <DownloadIcon className='size-3.5' />
            {t('Download')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            onClick={onClose}
            aria-label={t('Close')}
            className='size-8'
          >
            <XIcon className='size-4' />
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
