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
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Groups } from '@/features/groups'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

type ChannelGroupTab = 'channels' | 'groups'

type ChannelsProps = {
  defaultTab?: ChannelGroupTab
}

function ChannelsContent() {
  return <ChannelsTable />
}

export function Channels({ defaultTab = 'channels' }: ChannelsProps) {
  return (
    <ChannelsProvider>
      <ChannelsFrame defaultTab={defaultTab} />
      <ChannelsDialogs />
    </ChannelsProvider>
  )
}

function ChannelsFrame({ defaultTab }: { defaultTab: ChannelGroupTab }) {
  const navigate = useNavigate()
  const { t } = useTranslation()

  const handleTabChange = (value: string) => {
    if (value === 'groups') {
      navigate({ to: '/groups' })
      return
    }
    navigate({ to: '/channels' })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Channels & Groups')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {defaultTab === 'channels' && <ChannelsPrimaryButtons />}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs
          value={defaultTab}
          onValueChange={handleTabChange}
          className='min-h-0 gap-0'
        >
          <TabsList className='mb-4 h-8 rounded-full bg-muted p-1'>
            <TabsTrigger
              value='channels'
              className='h-6 rounded-full px-3 text-sm data-active:shadow-none'
            >
              {t('Channels')}
            </TabsTrigger>
            <TabsTrigger
              value='groups'
              className='h-6 rounded-full px-3 text-sm data-active:shadow-none'
            >
              {t('Group pricing')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='channels' className='mt-0'>
            <ChannelsContent />
          </TabsContent>
          <TabsContent value='groups' className='mt-0'>
            <Groups embedded />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
