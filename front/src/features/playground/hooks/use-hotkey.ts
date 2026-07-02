/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useEffect, useRef } from 'react'

export type HotkeyHandler = (event: KeyboardEvent) => void

export type HotkeyOptions = {
  /** Platform-aware modifier: ⌘ on macOS, Ctrl elsewhere. */
  combo: 'mod' | 'mod+shift' | 'none'
  key: string
  /** When true, the listener is attached to window. Defaults to true. */
  global?: boolean
  /** When true, prevent default. Defaults to true. */
  preventDefault?: boolean
  /** Allow hotkey to fire even when the target is a text input. */
  allowInInputs?: boolean
}

/**
 * Cross-platform modifier detection: ⌘ on macOS, Ctrl on Windows/Linux.
 */
export function isMacPlatform() {
  if (typeof navigator === 'undefined') return false
  return /Mac|iPod|iPhone|iPad/.test(navigator.platform)
}

function matchesCombo(event: KeyboardEvent, options: HotkeyOptions): boolean {
  const mod = isMacPlatform() ? event.metaKey : event.ctrlKey
  const shift = event.shiftKey
  const alt = event.altKey

  if (options.combo === 'mod') {
    if (!mod) return false
    if (shift || alt) return false
  } else if (options.combo === 'mod+shift') {
    if (!mod || !shift) return false
    if (alt) return false
  } else if (options.combo === 'none') {
    if (mod || shift || alt) return false
  }

  return event.key.toLowerCase() === options.key.toLowerCase()
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  return false
}

/**
 * useHotkey registers a keyboard shortcut.
 *
 * The listener is attached during the effect so React's render lifecycle
 * owns the subscription.
 */
export function useHotkey(
  options: HotkeyOptions,
  handler: HotkeyHandler
): void {
  const handlerRef = useRef(handler)
  handlerRef.current = handler

  useEffect(() => {
    if (typeof window === 'undefined') return

    const onKeyDown = (event: KeyboardEvent) => {
      if (!matchesCombo(event, options)) return
      if (!options.allowInInputs && isEditableTarget(event.target)) return

      if (options.preventDefault !== false) {
        event.preventDefault()
      }
      handlerRef.current(event)
    }

    const target = options.global === false ? document : window
    target.addEventListener('keydown', onKeyDown as EventListener)
    return () => {
      target.removeEventListener('keydown', onKeyDown as EventListener)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [options.combo, options.key, options.global, options.allowInInputs])
}
