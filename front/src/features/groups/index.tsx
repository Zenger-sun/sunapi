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
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Edit, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  type Group,
  type GroupPayload,
  createGroup,
  deleteGroup,
  getGroups,
  updateGroup,
} from '@/features/sunapi/api'
import {
  formatCurrency,
  formatDateTime,
  formatNumber,
} from '@/features/sunapi/format'

type GroupsProps = {
  embedded?: boolean
}

const emptyGroup: GroupPayload = {
  name: '',
  description: '',
  price_multiplier: 1,
}

function toForm(group?: Group | null): GroupPayload {
  if (!group) return { ...emptyGroup }
  return {
    name: group.name,
    description: group.description || '',
    price_multiplier: group.price_multiplier,
  }
}

export function Groups({ embedded = false }: GroupsProps) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Group | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState<GroupPayload>(emptyGroup)

  const groupsQuery = useQuery({
    queryKey: ['sunapi-groups'],
    queryFn: getGroups,
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['sunapi-groups'] })
    queryClient.invalidateQueries({ queryKey: ['sunapi-dashboard'] })
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      editing ? updateGroup(editing.id, form) : createGroup(form),
    onSuccess: () => {
      toast.success(editing ? '分组已更新' : '分组已创建')
      setDialogOpen(false)
      setEditing(null)
      invalidate()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteGroup,
    onSuccess: () => {
      toast.success('分组已删除')
      invalidate()
    },
  })

  useEffect(() => {
    setForm(toForm(editing))
  }, [editing, dialogOpen])

  const groups = groupsQuery.data?.items || []

  const actions = (
    <Button
      size='sm'
      onClick={() => {
        setEditing(null)
        setDialogOpen(true)
      }}
    >
      <Plus className='size-4' />
      新增分组
    </Button>
  )

  const content = (
    <>
      {embedded && <div className='mb-3 flex justify-end'>{actions}</div>}
      <div className='rounded-lg border bg-card'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead>说明</TableHead>
              <TableHead className='text-right'>倍率</TableHead>
              <TableHead className='text-right'>渠道</TableHead>
              <TableHead className='text-right'>调用</TableHead>
              <TableHead className='text-right'>Token</TableHead>
              <TableHead className='text-right'>费用</TableHead>
              <TableHead>更新时间</TableHead>
              <TableHead className='w-[120px] text-right'>操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.map((group) => (
              <TableRow key={group.id}>
                <TableCell>
                  <Badge variant='secondary'>{group.name}</Badge>
                </TableCell>
                <TableCell className='text-muted-foreground'>
                  {group.description || '-'}
                </TableCell>
                <TableCell className='text-right font-mono'>
                  {formatNumber(group.price_multiplier, 3)}
                </TableCell>
                <TableCell className='text-right font-mono'>
                  {formatNumber(group.channels)}
                </TableCell>
                <TableCell className='text-right font-mono'>
                  {formatNumber(group.request_count)}
                </TableCell>
                <TableCell className='text-right font-mono'>
                  {formatNumber(group.used_tokens)}
                </TableCell>
                <TableCell className='text-right font-mono'>
                  {formatCurrency(group.cost, '$', 4)}
                </TableCell>
                <TableCell className='text-muted-foreground text-xs'>
                  {formatDateTime(group.updated_at)}
                </TableCell>
                <TableCell className='text-right'>
                  <div className='flex justify-end gap-1'>
                    <Button
                      variant='ghost'
                      size='icon'
                      onClick={() => {
                        setEditing(group)
                        setDialogOpen(true)
                      }}
                    >
                      <Edit className='size-4' />
                    </Button>
                    <Button
                      variant='ghost'
                      size='icon'
                      disabled={group.name === 'default'}
                      onClick={() => deleteMutation.mutate(group.id)}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  )

  const dialog = (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? '编辑分组' : '新增分组'}</DialogTitle>
        </DialogHeader>
        <div className='grid gap-4 py-2'>
          <div className='grid gap-2'>
            <Label>名称</Label>
            <Input
              value={form.name}
              disabled={editing?.name === 'default'}
              onChange={(event) =>
                setForm((prev) => ({ ...prev, name: event.target.value }))
              }
            />
          </div>
          <div className='grid gap-2'>
            <Label>说明</Label>
            <Input
              value={form.description || ''}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  description: event.target.value,
                }))
              }
            />
          </div>
          <div className='grid gap-2'>
            <Label>价格倍率</Label>
            <Input
              type='number'
              min='0'
              step='0.001'
              value={form.price_multiplier}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  price_multiplier: Number(event.target.value),
                }))
              }
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => setDialogOpen(false)}>
            取消
          </Button>
          <Button
            disabled={!form.name.trim() || saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  if (embedded) {
    return (
      <>
        {content}
        {dialog}
      </>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>分组</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        用分组管理渠道池，并按倍率计算本地估算价格。
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>{actions}</SectionPageLayout.Actions>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
      {dialog}
    </SectionPageLayout>
  )
}
