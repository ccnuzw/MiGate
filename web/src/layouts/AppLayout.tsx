import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  Activity,
  Boxes,
  ChevronLeft,
  ChevronRight,
  Gauge,
  GitBranch,
  Languages,
  LogOut,
  Moon,
  Route,
  ServerCog,
  Settings,
  Shield,
  Sun,
} from 'lucide-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import clsx from 'clsx';
import { api } from '../api/endpoints';
import { useI18n } from '../lib/i18n';
import { useToast } from '../components/ui';
import { useState } from 'react';

const navItems = [
  { to: '/', key: 'overview', icon: Gauge },
  { to: '/inbounds', key: 'inbounds', icon: Shield },
  { to: '/outbounds', key: 'outbounds', icon: Boxes },
  { to: '/routing', key: 'routing', icon: Route },
  { to: '/xray', key: 'xray', icon: Activity },
  { to: '/singbox', key: 'singbox', icon: ServerCog },
  { to: '/settings', key: 'settings', icon: Settings },
] as const;

export default function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { lang, setLang, t } = useI18n();
  const { showToast } = useToast();
  const [theme, setTheme] = useState(() => document.documentElement.dataset.theme || 'light');
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const session = useQuery({ queryKey: ['session'], queryFn: api.session });
  const version = useQuery({ queryKey: ['version'], queryFn: api.version });
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => navigate('/login', { replace: true }),
    onError: () => showToast(t('logoutFailed'), 'error'),
  });

  const toggleTheme = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    localStorage.setItem('migate-theme', next);
    setTheme(next);
  };

  return (
    <div className={clsx('min-h-screen bg-panel-bg text-panel-text', sidebarOpen && 'sidebar-open')}>
      <div className="mobile-topbar">
        <button className="icon-button" onClick={() => setSidebarOpen(true)}>
          <ChevronRight className="h-5 w-5" />
        </button>
        <div className="font-semibold">MiGate</div>
      </div>
      <aside className="sidebar">
        <div className="flex items-center justify-between px-4 py-4">
          <div>
            <div className="text-lg font-semibold tracking-normal">MiGate</div>
          <div className="text-xs text-panel-muted">{t('singleBinaryPanel')}</div>
          </div>
          <button className="icon-button md:hidden" onClick={() => setSidebarOpen(false)}>
            <ChevronLeft className="h-5 w-5" />
          </button>
        </div>
        <nav className="grid gap-1 px-2">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) => clsx('nav-link', isActive && 'nav-link-active')}
                onClick={() => setSidebarOpen(false)}
              >
                <Icon className="h-4 w-4" />
                <span>{t(item.key)}</span>
              </NavLink>
            );
          })}
        </nav>
        <div className="mt-auto border-t border-panel-line p-3">
          <div className="rounded-lg bg-panel-soft p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <GitBranch className="h-4 w-4" />
              <span className="truncate">{version.data?.version || 'dev'}</span>
            </div>
            <div className="mt-2 truncate text-xs text-panel-muted">{session.data?.username || t('notLoggedIn')}</div>
          </div>
          <div className="mt-3 grid grid-cols-3 gap-2">
            <button className="icon-button h-9" onClick={toggleTheme} title="主题切换">
              {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
            <button className="icon-button h-9" onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')} title="语言切换">
              <Languages className="h-4 w-4" />
            </button>
            <button className="icon-button h-9" onClick={() => logout.mutate()} title="登出">
              <LogOut className="h-4 w-4" />
            </button>
          </div>
        </div>
      </aside>
      <div className="sidebar-overlay" onClick={() => setSidebarOpen(false)} />
      <main className="main-shell" key={location.pathname}>
        <Outlet />
      </main>
    </div>
  );
}
