import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api } from './api/endpoints';
import { ConfirmProvider, LoadingBlock, ToastProvider } from './components/ui';
import { I18nProvider } from './lib/i18n';
import AppLayout from './layouts/AppLayout';
import LoginPage from './routes/LoginPage';
import OverviewPage from './routes/OverviewPage';
import InboundsPage from './routes/InboundsPage';
import OutboundsPage from './routes/OutboundsPage';
import RoutingPage from './routes/RoutingPage';
import CorePage from './routes/CorePage';
import SettingsPage from './routes/SettingsPage';

export default function App() {
  return (
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route element={<RequireSession />}>
              <Route element={<AppLayout />}>
                <Route index element={<OverviewPage />} />
                <Route path="inbounds" element={<InboundsPage />} />
                <Route path="outbounds" element={<OutboundsPage />} />
                <Route path="routing" element={<RoutingPage />} />
                <Route path="xray" element={<CorePage core="xray" />} />
                <Route path="singbox" element={<CorePage core="singbox" />} />
                <Route path="settings" element={<SettingsPage />} />
              </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  );
}

function RequireSession() {
  const location = useLocation();
  const session = useQuery({ queryKey: ['session'], queryFn: api.session });
  if (session.isLoading) return <LoadingBlock />;
  if (session.data?.auth_enabled && !session.data.authenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }
  return <Outlet />;
}
