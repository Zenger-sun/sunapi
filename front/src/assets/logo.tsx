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
import { type SVGProps } from 'react'
import { cn } from '@/lib/utils'

export function Logo({ className, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      id='sunapi-logo'
      viewBox='0 0 64 64'
      xmlns='http://www.w3.org/2000/svg'
      height='24'
      width='24'
      className={cn('size-6', className)}
      {...props}
    >
      <title>SunAPI</title>
      <defs>
        <linearGradient
          id='sunapi-falcon-ink'
          x1='13'
          y1='9'
          x2='53'
          y2='57'
          gradientUnits='userSpaceOnUse'
        >
          <stop stopColor='#2f496c' />
          <stop offset='0.48' stopColor='#102a4c' />
          <stop offset='1' stopColor='#071a33' />
        </linearGradient>
        <linearGradient
          id='sunapi-falcon-mid'
          x1='23'
          y1='11'
          x2='51'
          y2='46'
          gradientUnits='userSpaceOnUse'
        >
          <stop stopColor='#5f7899' />
          <stop offset='1' stopColor='#18365d' />
        </linearGradient>
        <linearGradient
          id='sunapi-falcon-shadow'
          x1='18'
          y1='21'
          x2='46'
          y2='54'
          gradientUnits='userSpaceOnUse'
        >
          <stop stopColor='#203d64' />
          <stop offset='1' stopColor='#08162a' />
        </linearGradient>
      </defs>
      <path
        fill='url(#sunapi-falcon-ink)'
        d='M8.5 34.7 16 27.2 8.7 29.1l9.4-5.8 7.7-5 13.5-1.1 9.7 2.5 6.1 5.1 2.4 6.3 3.6 4.2-1.8 8.7-5.4 7.6-9.9 6.7-13.6 1.4-8.5-5.6-8.1 1.6 5-7.6-9.2 3.7 4.7-8.1-8.6 3.2z'
      />
      <path
        fill='url(#sunapi-falcon-mid)'
        d='m23.5 18.4 15.9-1.2 9.7 2.5-6.6 10.2-15.7-1.9-18.1 1.1 9.4-5.8z'
      />
      <path
        fill='#132d50'
        d='m8.5 34.7 18.3-6.7 15.7 1.9-22.7 4.2-10.2 9.6-5.9 3.2z'
      />
      <path fill='#223f67' d='m19.8 34.1 12.3 4-18.3 17.6 5-7.6-9.2 3.7z' />
      <path
        fill='url(#sunapi-falcon-shadow)'
        d='m32.1 38.1 9.8 5.6-11.5 16-8.5-5.6z'
      />
      <path fill='#2f4c74' d='m41.9 43.7 7.8 4.8-5.7 9.8-13.6 1.4z' />
      <path
        fill='#eef5fb'
        d='m22.6 39.4 12.3-3.6-8.8 7.8 9.2-.9-13.9 12.8 4.2-9.4-9.5 5.7z'
      />
      <path fill='#ffffff' d='m33.3 33.7 8.4 1.9 8.4 7.7-13.9-3.1-8.8 2.9z' />
      <path fill='#f8fbff' d='m43 37.2 5.4-6.7 6.9 5.7-7.9 4.7z' />
      <path fill='#0a1d37' d='m48.4 30.5 6.7-5.7 2.4 6.3 3.6 4.2-5.8.9z' />
      <path fill='#102a4c' d='m49.7 48.5 4.2 3.1-9.9 6.7z' />
      <path fill='#0b203d' d='m42.5 29.9 6.6-10.2 6.1 5.1-6.8 5.7-5.4 6.7z' />
      <path
        fill='#ffffff'
        d='m27.4 29.2 7.5 1.1-2.6 1.4 2.4 5.4c-4.6-.6-7.2-3.5-7.3-7.9z'
      />
      <path
        fill='#071a33'
        d='m32.3 31.7 4.7-2.6 5.4.8-3 5.2c-1.2 2-3.6 2.9-5.8 2.1z'
      />
      <path fill='#ffffff' d='m35.4 30.6 1.4-1.6 1.7 1.4-1.4 1.7z' />
      <path fill='#ffffff' d='m52.2 35.6 2.4 1.5-3.5 2-2-1.4z' />
    </svg>
  )
}
