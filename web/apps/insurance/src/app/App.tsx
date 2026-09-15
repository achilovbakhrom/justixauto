import { useState } from 'react';
import type { ComponentType, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { Shell } from './Shell';
import type { ShellContext } from './Shell';

export interface InsuranceAppProps {
  SessionBoundary: ComponentType<{ children: ReactNode }>;
  context?: ShellContext;
}

export function InsuranceApp({ SessionBoundary, context }: InsuranceAppProps) {
  // Memory-only, private to this mount. The session adapter owns admission and
  // eventual context invalidation before any protected feature data is used.
  const [queryClient] = useState(() => new QueryClient());
  return <QueryClientProvider client={queryClient}>
    <SessionBoundary><BrowserRouter basename="/insurance/">
      <Shell {...(context === undefined ? {} : { context })}>
        <Routes><Route path="*" element={null} /></Routes>
      </Shell>
    </BrowserRouter></SessionBoundary>
  </QueryClientProvider>;
}
