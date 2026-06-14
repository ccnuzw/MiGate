import { useMutation, useQuery } from '@tanstack/react-query';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { LockKeyhole } from 'lucide-react';
import { api } from '../api/endpoints';
import { Field, useToast } from '../components/ui';

const schema = z.object({
  username: z.string().min(1),
  password: z.string().min(1),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { showToast } = useToast();
  const session = useQuery({ queryKey: ['session'], queryFn: api.session });
  const form = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: { username: 'admin', password: '' } });
  const login = useMutation({
    mutationFn: (values: FormValues) => api.login(values.username, values.password),
    onSuccess: () => navigate(redirectTarget(location.state), { replace: true }),
    onError: () => showToast('登录失败，请检查用户名或密码', 'error'),
  });

  if (session.data?.authenticated || session.data?.auth_enabled === false) return <Navigate to="/" replace />;

  return (
    <div className="login-screen">
      <form className="login-panel" onSubmit={form.handleSubmit((values) => login.mutate(values))}>
        <div className="mb-6 flex items-center gap-3">
          <div className="grid h-10 w-10 place-items-center rounded-lg bg-panel-text text-panel-bg">
            <LockKeyhole className="h-5 w-5" />
          </div>
          <div>
            <h1 className="text-xl font-semibold">MiGate</h1>
            <p className="text-sm text-panel-muted">面板登录</p>
          </div>
        </div>
        {session.isError ? <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">会话检查失败，请直接登录或刷新页面。</div> : null}
        <div className="grid gap-4">
          <Field label="用户名">
            <input {...form.register('username')} autoComplete="username" />
          </Field>
          <Field label="密码">
            <input {...form.register('password')} type="password" autoComplete="current-password" />
          </Field>
          <button className="btn primary h-10" disabled={login.isPending}>
            {login.isPending ? '登录中...' : '登录'}
          </button>
        </div>
      </form>
    </div>
  );
}

function redirectTarget(state: unknown): string {
  const from = typeof state === 'object' && state && 'from' in state ? (state as { from?: { pathname?: string; search?: string } }).from : undefined;
  const path = from?.pathname || '/';
  const search = from?.search || '';
  if (path === '/login') return '/';
  return `${path}${search}`;
}
