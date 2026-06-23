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
import type { FieldPath } from 'react-hook-form'
import type { ChannelFormValues } from './channel-form'

type ChannelFormErrorMap = Partial<
  Record<FieldPath<ChannelFormValues>, unknown>
>

const ADVANCED_SETTINGS_FIELDS = new Set<FieldPath<ChannelFormValues>>([
  'priority',
  'weight',
])

export function isAdvancedSettingsField(
  fieldName: string
): fieldName is FieldPath<ChannelFormValues> {
  return ADVANCED_SETTINGS_FIELDS.has(fieldName as FieldPath<ChannelFormValues>)
}

export function hasAdvancedSettingsErrors(
  errors: ChannelFormErrorMap
): boolean {
  return Object.keys(errors).some((fieldName) =>
    isAdvancedSettingsField(fieldName)
  )
}
