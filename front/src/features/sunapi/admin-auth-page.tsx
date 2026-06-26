import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, KeyRound, Loader2, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  adminAuthErrorMessage,
  getAdminAuthStatus,
  loginAdmin,
  setupAdmin,
  type AdminCredentials,
} from './auth-api'

type AdminAuthMode = 'setup' | 'login'

interface AdminAuthPageProps {
  mode: AdminAuthMode
}

const ADMIN_USERNAME = 'admin'

export function AdminAuthPage({ mode }: AdminAuthPageProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { systemName, logo } = useSystemConfig()
  const { auth } = useAuthStore()
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [errorText, setErrorText] = useState('')

  const statusQuery = useQuery({
    queryKey: ['sunapi-admin-auth-status'],
    queryFn: getAdminAuthStatus,
    retry: false,
  })

  const status = statusQuery.data
  const isSetup = mode === 'setup'

  useEffect(() => {
    setErrorText('')
    setPassword('')
    setConfirmPassword('')
  }, [mode])

  useEffect(() => {
    if (!status) return
    if (status.authenticated && status.user) {
      auth.setUser(status.user)
      navigate({ to: '/dashboard', replace: true })
      return
    }
    if (mode === 'login' && !status.initialized) {
      navigate({ to: '/setup', replace: true })
      return
    }
    if (mode === 'setup' && status.initialized) {
      navigate({ to: '/login', replace: true })
    }
  }, [auth, mode, navigate, status])

  useEffect(() => {
    if (!statusQuery.error) return
    setErrorText(
      adminAuthErrorMessage(statusQuery.error, '无法读取本地认证状态，请稍后重试')
    )
  }, [statusQuery.error])

  const copy = useMemo(
    () =>
      isSetup
        ? {
            icon: ShieldCheck,
            title: '首次设置管理员密码',
            description:
              '本地服务尚未初始化，请为 admin 管理员创建登录密码。',
            helper: '之后进入控制台和创作台都需要使用这个密码。',
            submit: '创建并进入控制台',
            success: '管理员密码已设置',
          }
        : {
            icon: KeyRound,
            title: '管理员登录',
            description: '使用首次设置时创建的密码进入本地控制台。',
            helper: '当前系统只保留 admin 管理员账号。',
            submit: '进入控制台',
            success: '登录成功',
          },
    [isSetup]
  )

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextPassword = password.trim()
    setErrorText('')

    if (!nextPassword) {
      setErrorText(isSetup ? '请设置管理员密码' : '请输入管理员密码')
      return
    }
    if (isSetup && nextPassword.length < 8) {
      setErrorText('管理员密码至少需要 8 位')
      return
    }
    if (isSetup && nextPassword !== confirmPassword.trim()) {
      setErrorText('两次输入的密码不一致')
      return
    }

    mutation.mutate({
      username: ADMIN_USERNAME,
      password: nextPassword,
    })
  }

  const mutation = useMutation({
    mutationFn: (credentials: AdminCredentials) =>
      isSetup ? setupAdmin(credentials) : loginAdmin(credentials),
    onSuccess: async (user) => {
      auth.setUser(user)
      await queryClient.invalidateQueries({
        queryKey: ['sunapi-admin-auth-status'],
      })
      toast.success(copy.success)
      navigate({ to: '/dashboard', replace: true })
    },
    onError: (error) => {
      setErrorText(
        adminAuthErrorMessage(
          error,
          isSetup ? '管理员密码设置失败' : '登录失败'
        )
      )
    },
  })

  const loading = statusQuery.isLoading || mutation.isPending
  const Icon = copy.icon

  return (
    <main className='bg-background relative flex min-h-svh items-center justify-center px-4 py-10'>
      <Link
        to='/home'
        className='focus-visible:ring-ring/40 absolute top-6 left-6 inline-flex items-center gap-2 rounded-md px-1.5 py-1 text-sm font-semibold outline-none transition-colors hover:bg-accent focus-visible:ring-2'
        aria-label='返回首页'
      >
        <img
          src={logo}
          alt='SunAPI'
          className='size-7 rounded-md object-cover'
        />
        <span>{systemName || 'SunAPI'}</span>
      </Link>

      <Card className='w-full max-w-sm rounded-lg'>
        <CardHeader className='gap-3'>
          <div className='flex items-center gap-3'>
            <div className='border-border bg-muted flex size-10 items-center justify-center rounded-lg border'>
              <Icon className='size-5' />
            </div>
            <div className='min-w-0'>
              <CardTitle className='truncate'>{copy.title}</CardTitle>
              <CardDescription className='truncate'>
                {systemName || 'SunAPI'}
              </CardDescription>
            </div>
          </div>
          <CardDescription>{copy.description}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className='space-y-4' onSubmit={handleSubmit}>
            <div className='rounded-lg border bg-muted/40 px-3 py-2 text-sm'>
              <span className='text-muted-foreground'>管理员账号</span>
              <span className='ml-2 font-medium'>{ADMIN_USERNAME}</span>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='admin-password'>
                {isSetup ? '设置密码' : '密码'}
              </Label>
              <Input
                id='admin-password'
                type='password'
                value={password}
                autoComplete={isSetup ? 'new-password' : 'current-password'}
                placeholder={isSetup ? '至少 8 位' : '请输入管理员密码'}
                onChange={(event) => setPassword(event.target.value)}
                disabled={loading}
              />
            </div>

            {isSetup && (
              <div className='space-y-2'>
                <Label htmlFor='admin-password-confirm'>确认密码</Label>
                <Input
                  id='admin-password-confirm'
                  type='password'
                  value={confirmPassword}
                  autoComplete='new-password'
                  placeholder='再次输入管理员密码'
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  disabled={loading}
                />
              </div>
            )}

            {errorText && (
              <Alert variant='destructive'>
                <AlertCircle className='size-4' />
                <AlertDescription>{errorText}</AlertDescription>
              </Alert>
            )}

            <Button type='submit' className='w-full' disabled={loading}>
              {loading && <Loader2 className='size-4 animate-spin' />}
              {copy.submit}
            </Button>
          </form>

          <p className='text-muted-foreground mt-4 text-center text-xs'>
            {copy.helper}
          </p>
        </CardContent>
      </Card>
    </main>
  )
}
