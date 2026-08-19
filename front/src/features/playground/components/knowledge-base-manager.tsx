/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  BookOpenIcon,
  FileTextIcon,
  Loader2Icon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  Trash2Icon,
  UploadIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  createKnowledgeBase,
  deleteKnowledgeBase,
  deleteKnowledgeDocument,
  listKnowledgeBases,
  listKnowledgeDocuments,
  toggleKnowledgeBase,
  updateKnowledgeBase,
  uploadKnowledgeDocument,
} from '../api'
import type { KnowledgeBaseItem, KnowledgeDocumentItem } from '../types'

type DeleteTarget =
  | { kind: 'base'; id: string; name: string }
  | { kind: 'document'; id: string; name: string; baseId: string }

interface KnowledgeBaseManagerProps {
  selectedKnowledgeBaseIds?: string[]
  onSelectedKnowledgeBaseIdsChange?: (ids: string[]) => void
}

export function KnowledgeBaseManager({
  selectedKnowledgeBaseIds = [],
  onSelectedKnowledgeBaseIdsChange,
}: KnowledgeBaseManagerProps = {}) {
  const isBlueprintSelection = onSelectedKnowledgeBaseIdsChange !== undefined
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [items, setItems] = useState<KnowledgeBaseItem[]>([])
  const [documents, setDocuments] = useState<KnowledgeDocumentItem[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isDocumentLoading, setIsDocumentLoading] = useState(false)
  const [isUploading, setIsUploading] = useState(false)
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [editing, setEditing] = useState<KnowledgeBaseItem | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const loadBases = useCallback(async () => {
    try {
      const next = await listKnowledgeBases()
      setItems(next)
      setSelectedId((current) => {
        if (current && next.some((item) => item.id === current)) return current
        return next[0]?.id ?? null
      })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '知识库加载失败')
    } finally {
      setIsLoading(false)
    }
  }, [])

  const loadDocuments = useCallback(async (baseId: string) => {
    setIsDocumentLoading(true)
    try {
      setDocuments(await listKnowledgeDocuments(baseId))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '文档加载失败')
      setDocuments([])
    } finally {
      setIsDocumentLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadBases()
  }, [loadBases])

  useEffect(() => {
    if (selectedId) {
      void loadDocuments(selectedId)
    } else {
      setDocuments([])
    }
  }, [loadDocuments, selectedId])

  const selected = useMemo(
    () => items.find((item) => item.id === selectedId) ?? null,
    [items, selectedId]
  )

  const filteredItems = useMemo(() => {
    const term = search.trim().toLowerCase()
    if (!term) return items
    return items.filter(
      (item) =>
        item.name.toLowerCase().includes(term) ||
        item.description?.toLowerCase().includes(term)
    )
  }, [items, search])

  const openCreate = () => {
    setEditing(null)
    setName('')
    setDescription('')
    setIsFormOpen(true)
  }

  const openEdit = (item: KnowledgeBaseItem) => {
    setEditing(item)
    setName(item.name)
    setDescription(item.description ?? '')
    setIsFormOpen(true)
  }

  const save = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) {
      toast.error('请输入知识库名称')
      return
    }
    setIsSaving(true)
    try {
      const saved = editing
        ? await updateKnowledgeBase(editing.id, {
            name: trimmedName,
            description: description.trim(),
          })
        : await createKnowledgeBase({
            name: trimmedName,
            description: description.trim(),
          })
      setIsFormOpen(false)
      setSelectedId(saved.id)
      await loadBases()
      toast.success(editing ? '知识库已更新' : '知识库已创建')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '知识库保存失败')
    } finally {
      setIsSaving(false)
    }
  }

  const toggle = async (item: KnowledgeBaseItem, enabled: boolean) => {
    if (onSelectedKnowledgeBaseIdsChange) {
      const nextIds = enabled
        ? Array.from(new Set([...selectedKnowledgeBaseIds, item.id]))
        : selectedKnowledgeBaseIds.filter((id) => id !== item.id)
      onSelectedKnowledgeBaseIdsChange(nextIds)
      return
    }
    setItems((current) =>
      current.map((entry) =>
        entry.id === item.id ? { ...entry, enabled } : entry
      )
    )
    try {
      await toggleKnowledgeBase(item.id, enabled)
      toast.success(enabled ? '知识库已开启' : '知识库已停用')
    } catch (error) {
      setItems((current) =>
        current.map((entry) =>
          entry.id === item.id ? { ...entry, enabled: item.enabled } : entry
        )
      )
      toast.error(error instanceof Error ? error.message : '状态更新失败')
    }
  }

  const uploadFiles = async (files: FileList | null) => {
    if (!selectedId || !files?.length) return
    setIsUploading(true)
    let uploaded = 0
    try {
      for (const file of Array.from(files)) {
        await uploadKnowledgeDocument(selectedId, file)
        uploaded += 1
      }
      await Promise.all([loadDocuments(selectedId), loadBases()])
      toast.success(`已导入 ${uploaded} 个文档`)
    } catch (error) {
      if (uploaded > 0) {
        await Promise.all([loadDocuments(selectedId), loadBases()])
      }
      toast.error(error instanceof Error ? error.message : '文档导入失败')
    } finally {
      setIsUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    setIsDeleting(true)
    try {
      if (deleteTarget.kind === 'base') {
        await deleteKnowledgeBase(deleteTarget.id)
        if (onSelectedKnowledgeBaseIdsChange) {
          onSelectedKnowledgeBaseIdsChange(
            selectedKnowledgeBaseIds.filter((id) => id !== deleteTarget.id)
          )
        }
        await loadBases()
        toast.success('知识库已删除')
      } else {
        await deleteKnowledgeDocument(deleteTarget.baseId, deleteTarget.id)
        await Promise.all([loadDocuments(deleteTarget.baseId), loadBases()])
        toast.success('文档已删除')
      }
      setDeleteTarget(null)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除失败')
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='flex shrink-0 items-center gap-1.5 px-2 py-2'>
        <div className='relative min-w-0 flex-1'>
          <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder='搜索知识库…'
            className='h-7 rounded-[7px] pl-7 text-xs'
          />
        </div>
        <Button
          size='icon-sm'
          variant='outline'
          onClick={openCreate}
          aria-label='新建知识库'
          title='新建知识库'
        >
          <PlusIcon className='size-3.5' />
        </Button>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto px-2 pb-3'>
        {isLoading ? (
          <EmptyState icon={Loader2Icon} label='正在加载' spinning />
        ) : filteredItems.length === 0 ? (
          <EmptyState icon={BookOpenIcon} label='暂无知识库' />
        ) : (
          <div className='space-y-1'>
            {filteredItems.map((item) => {
              const enabled = isBlueprintSelection
                ? selectedKnowledgeBaseIds.includes(item.id)
                : item.enabled
              return (
                <div
                  key={item.id}
                  className={cn(
                    'border-border/70 hover:bg-muted/60 flex w-full items-start gap-2 rounded-[7px] border px-2 py-2 text-left transition-colors',
                    selectedId === item.id && 'border-primary/40 bg-primary/5'
                  )}
                >
                  <BookOpenIcon className='text-muted-foreground mt-0.5 size-3.5 shrink-0' />
                  <button
                    type='button'
                    className='min-w-0 flex-1 text-left'
                    onClick={() => setSelectedId(item.id)}
                  >
                    <span className='block truncate text-xs font-medium'>
                      {item.name}
                    </span>
                    <span className='text-muted-foreground mt-0.5 block text-[10px]'>
                      {item.document_count} 个文档 · {item.chunk_count} 个分块
                    </span>
                  </button>
                  <span className='flex shrink-0 items-center gap-0.5'>
                    <Switch
                      size='sm'
                      checked={enabled}
                      onCheckedChange={(enabled) => void toggle(item, enabled)}
                      aria-label={
                        isBlueprintSelection
                          ? enabled
                            ? '从当前蓝图移除知识库'
                            : '为当前蓝图开启知识库'
                          : enabled
                            ? '停用知识库'
                            : '开启知识库'
                      }
                    />
                    <Button
                      size='icon-xs'
                      variant='ghost'
                      onClick={() => openEdit(item)}
                      aria-label='编辑知识库'
                      title='编辑'
                    >
                      <PencilIcon className='size-3' />
                    </Button>
                    <Button
                      size='icon-xs'
                      variant='ghost'
                      className='text-destructive hover:text-destructive'
                      onClick={() =>
                        setDeleteTarget({
                          kind: 'base',
                          id: item.id,
                          name: item.name,
                        })
                      }
                      aria-label='删除知识库'
                      title='删除'
                    >
                      <Trash2Icon className='size-3' />
                    </Button>
                  </span>
                </div>
              )
            })}
          </div>
        )}

        {selected && (
          <section className='border-border/70 mt-3 border-t pt-3'>
            <div className='mb-2 flex items-center gap-2 px-1'>
              <div className='min-w-0 flex-1'>
                <div className='flex items-center gap-1.5'>
                  <span className='truncate text-xs font-semibold'>文档</span>
                  {(isBlueprintSelection
                    ? selectedKnowledgeBaseIds.includes(selected.id)
                    : selected.enabled) && (
                    <Badge variant='secondary' className='h-4 px-1 text-[9px]'>
                      {isBlueprintSelection ? '用于当前蓝图' : '已开启'}
                    </Badge>
                  )}
                </div>
                {selected.description && (
                  <p className='text-muted-foreground mt-0.5 line-clamp-2 text-[10px]'>
                    {selected.description}
                  </p>
                )}
              </div>
              <input
                ref={fileInputRef}
                type='file'
                multiple
                accept='.txt,.md,.csv,.json,.html,.htm,.docx,.xlsx'
                className='hidden'
                onChange={(event) => void uploadFiles(event.target.files)}
              />
              <Button
                size='icon-sm'
                variant='outline'
                disabled={isUploading}
                onClick={() => fileInputRef.current?.click()}
                aria-label='导入文档'
                title='导入文档'
              >
                {isUploading ? (
                  <Loader2Icon className='size-3.5 animate-spin' />
                ) : (
                  <UploadIcon className='size-3.5' />
                )}
              </Button>
            </div>

            {isDocumentLoading ? (
              <EmptyState
                icon={Loader2Icon}
                label='正在加载'
                spinning
                compact
              />
            ) : documents.length === 0 ? (
              <EmptyState icon={FileTextIcon} label='暂无文档' compact />
            ) : (
              <div className='space-y-1'>
                {documents.map((document) => (
                  <div
                    key={document.id}
                    className='border-border/60 flex items-center gap-2 border-b px-1 py-1.5 last:border-0'
                  >
                    <FileTextIcon className='text-muted-foreground size-3.5 shrink-0' />
                    <div className='min-w-0 flex-1'>
                      <div className='truncate text-[11px] font-medium'>
                        {document.filename}
                      </div>
                      <div className='text-muted-foreground text-[9px]'>
                        {formatBytes(document.size)} · {document.chunk_count}{' '}
                        个分块
                      </div>
                    </div>
                    <Button
                      size='icon-xs'
                      variant='ghost'
                      className='text-muted-foreground hover:text-destructive'
                      onClick={() =>
                        setDeleteTarget({
                          kind: 'document',
                          id: document.id,
                          baseId: selected.id,
                          name: document.filename,
                        })
                      }
                      aria-label='删除文档'
                      title='删除文档'
                    >
                      <Trash2Icon className='size-3' />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}
      </div>

      <Dialog open={isFormOpen} onOpenChange={setIsFormOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '编辑知识库' : '新建知识库'}</DialogTitle>
            <DialogDescription>
              创建后可在蓝图中独立选择是否使用这个知识库。
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-3'>
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder='知识库名称'
              maxLength={80}
              autoFocus
            />
            <Textarea
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder='描述（可选）'
              rows={4}
            />
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setIsFormOpen(false)}>
              取消
            </Button>
            <Button disabled={isSaving} onClick={() => void save()}>
              {isSaving && <Loader2Icon className='animate-spin' />}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              删除{deleteTarget?.kind === 'base' ? '知识库' : '文档'}
            </DialogTitle>
            <DialogDescription>
              “{deleteTarget?.name}”删除后无法恢复。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='outline' onClick={() => setDeleteTarget(null)}>
              取消
            </Button>
            <Button
              variant='destructive'
              disabled={isDeleting}
              onClick={() => void confirmDelete()}
            >
              {isDeleting && <Loader2Icon className='animate-spin' />}
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function EmptyState({
  icon: Icon,
  label,
  spinning = false,
  compact = false,
}: {
  icon: typeof BookOpenIcon
  label: string
  spinning?: boolean
  compact?: boolean
}) {
  return (
    <div
      className={cn(
        'text-muted-foreground flex flex-col items-center justify-center gap-1.5 text-[11px]',
        compact ? 'py-5' : 'py-10'
      )}
    >
      <Icon className={cn('size-5 opacity-60', spinning && 'animate-spin')} />
      <span>{label}</span>
    </div>
  )
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
