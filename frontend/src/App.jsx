import React from 'react';
import { useAppStore } from './stores/appStore';
import { AuthGuard } from './components/auth/AuthGuard';
import { MainLayout } from './components/layout/MainLayout';
import { ProvidersView } from './components/providers/ProvidersView';
import { RoutingView } from './components/routing/RoutingView';
import { SettingsView } from './components/settings/SettingsView';

function App() {
  const currentView = useAppStore((state) => state.currentView);
  
  const renderView = () => {
    switch (currentView) {
      case 'providers':
        return <ProvidersView />;
      case 'routing':
        return <RoutingView />;
      case 'settings':
        return <SettingsView />;
      default:
        return <ProvidersView />;
    }
  };
  
  return (
    <AuthGuard>
      <MainLayout>
        {renderView()}
      </MainLayout>
    </AuthGuard>
  );
}

export default App;
