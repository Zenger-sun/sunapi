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

export function formatNumber(value: number | undefined, digits = 0) {
  return new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits,
  }).format(Number(value || 0))
}

export function formatCurrency(
  value: number | undefined,
  symbol = '$',
  digits = 4
) {
  return `${symbol}${formatNumber(value || 0, digits)}`
}

export function formatDateTime(timestamp: number | undefined) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function splitModels(models: string) {
  return models
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}
