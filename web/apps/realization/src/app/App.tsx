import { useState } from 'react';
import type { ComponentType, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes, useLocation } from 'react-router-dom';
import { Shell } from './Shell';
import type { ShellContext } from './Shell';

export interface RealizationAppProps {
  SessionBoundary: ComponentType<{ children: ReactNode }>;
  context?: ShellContext;
}

function ScopedShell({ context }: { context?: ShellContext }) {
  const { pathname } = useLocation();
  // The root basename alone cannot exclude other independent applications.
  if (/^\/(?:admin|finance|insurance|api)(?:\/|$)/.test(pathname)) return null;
  return <Shell {...(context === undefined ? {} : { context })}>
    <Routes><Route path="*" element={null} /></Routes>
  </Shell>;
}

export function RealizationApp({ SessionBoundary, context }: RealizationAppProps) {
  const [queryClient] = useState(() => new QueryClient());
  return <QueryClientProvider client={queryClient}>
    <SessionBoundary><BrowserRouter basename="/">
      <ScopedShell {...(context === undefined ? {} : { context })} />
    </BrowserRouter></SessionBoundary>
  </QueryClientProvider>;
}
