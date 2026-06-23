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
import { useMemo } from 'react'
import { Link, useLocation } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { type TopNavLink } from '../types'

type TopNavProps = React.HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
}

const consolePathPrefixes = [
  '/dashboard',
  '/channels',
  '/groups',
  '/keys',
  '/settings',
  '/usage-logs',
]

function normalizePath(path: string) {
  const pathname = path.split('?')[0] || '/'
  return pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
}

function isPathMatch(pathname: string, href: string) {
  const current = normalizePath(pathname)
  const target = normalizePath(href)

  if (target === '/') {
    return current === '/'
  }

  return current === target || current.startsWith(`${target}/`)
}

function isConsolePath(pathname: string) {
  return consolePathPrefixes.some((prefix) => isPathMatch(pathname, prefix))
}

function isTopNavLinkActive(pathname: string, link: TopNavLink) {
  if (link.isActive) return true
  if (link.external || /^https?:\/\//.test(link.href)) return false

  if (normalizePath(link.href) === '/dashboard' || normalizePath(link.href) === '/channels') {
    return isConsolePath(pathname)
  }

  return isPathMatch(pathname, link.href)
}

export function TopNav({ className, links, ...props }: TopNavProps) {
  const pathname = useLocation({ select: (location) => location.pathname })

  const normalizedLinks = useMemo(
    () =>
      links.map((link) => ({
        disabled: false,
        external: false,
        ...link,
        isActive: isTopNavLinkActive(pathname, link),
      })),
    [links, pathname]
  )

  const getLinkClassName = (isActive?: boolean, disabled?: boolean) =>
    cn(
      'rounded-md px-2 py-1 text-sm transition-colors',
      isActive
        ? 'bg-accent text-accent-foreground font-semibold'
        : 'text-muted-foreground font-medium hover:bg-accent/60 hover:text-foreground',
      disabled && 'pointer-events-none opacity-50'
    )

  const getDropdownLinkClassName = (isActive?: boolean, disabled?: boolean) =>
    cn(
      'w-full',
      isActive ? 'text-foreground font-semibold' : 'text-muted-foreground',
      disabled && 'pointer-events-none opacity-50'
    )

  const renderDropdownLink = (link: TopNavLink) =>
    link.external ? (
      <a
        href={link.href}
        target='_blank'
        rel='noopener noreferrer'
        className={getDropdownLinkClassName(link.isActive, link.disabled)}
      >
        {link.title}
      </a>
    ) : (
      <Link
        to={link.href}
        className={getDropdownLinkClassName(link.isActive, link.disabled)}
        disabled={link.disabled}
      >
        {link.title}
      </Link>
    )

  const renderLink = (link: TopNavLink) =>
    link.external ? (
      <a
        key={`${link.title}-${link.href}`}
        href={link.href}
        target='_blank'
        rel='noopener noreferrer'
        className={getLinkClassName(link.isActive, link.disabled)}
        aria-current={link.isActive ? 'page' : undefined}
      >
        {link.title}
      </a>
    ) : (
      <Link
        key={`${link.title}-${link.href}`}
        to={link.href}
        disabled={link.disabled}
        className={getLinkClassName(link.isActive, link.disabled)}
        aria-current={link.isActive ? 'page' : undefined}
      >
        {link.title}
      </Link>
    )

  return (
    <>
      <div className='lg:hidden'>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<Button size='icon' variant='outline' className='size-7' />}
          >
            <Menu />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='bottom' align='start'>
            {normalizedLinks.map((link) => (
              <DropdownMenuItem
                key={`${link.title}-${link.href}`}
                render={renderDropdownLink(link)}
              />
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <nav
        className={cn(
          'hidden items-center space-x-2 lg:flex lg:space-x-2 xl:space-x-3',
          className
        )}
        {...props}
      >
        {normalizedLinks.map(renderLink)}
      </nav>
    </>
  )
}
