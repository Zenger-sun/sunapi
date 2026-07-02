/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import type { LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { MESSAGE_ACTION_BUTTON_STYLES } from '../constants'

interface MessageActionButtonProps {
  icon: LucideIcon
  label: string
  onClick: () => void
  disabled?: boolean
  className?: string
  variant?: 'default' | 'destructive'
}

export function MessageActionButton({
  icon: Icon,
  label,
  onClick,
  disabled = false,
  className = '',
  variant = 'default',
}: MessageActionButtonProps) {
  const baseStyle =
    variant === 'destructive'
      ? MESSAGE_ACTION_BUTTON_STYLES.DELETE
      : MESSAGE_ACTION_BUTTON_STYLES.BASE

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            className={`${baseStyle} ${className}`}
            onClick={onClick}
            disabled={disabled}
            aria-label={label}
          />
        }
      >
        <Icon className={MESSAGE_ACTION_BUTTON_STYLES.ICON} />
      </TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
      </TooltipContent>
    </Tooltip>
  )
}
