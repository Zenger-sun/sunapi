/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useCallback } from 'react'
import { toast } from 'sonner'
import { MESSAGE_ACTION_LABELS } from '../constants'

/**
 * Hook to guard message actions when generation is in progress
 * Provides a wrapper that checks if generation is active before executing
 */
export function useMessageActionGuard(isGenerating: boolean) {
  const guardAction = useCallback(
    (action: () => void) => {
      return () => {
        if (isGenerating) {
          toast.warning(MESSAGE_ACTION_LABELS.WAIT_GENERATION)
          return
        }
        action()
      }
    },
    [isGenerating]
  )

  return { guardAction }
}
