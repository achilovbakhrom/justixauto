import { useState } from 'react';
import type { ComponentType, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { Shell } from './Shell';

export interface AdminAppProps {
  SessionBoundary: ComponentType<{ children: ReactNode }>;
  /** Supplied by the eventual session adapter; never a demo identity default. */
  identityLabel?: string;
}

export function AdminApp({ SessionBoundary, identityLabel }: AdminAppProps) {
  // Private to this mounted app. No shared singleton, persister, fetch or default
  // session exists. The auth integration owns context invalidation before data use.
  const [queryClient] = useState(() => new QueryClient());
  return <QueryClientProvider client={queryClient}>
    <SessionBoundary>
      <BrowserRouter basename="/admin/">
        <Shell {...(identityLabel === undefined ? {} : { identityLabel })}>
          <Routes>
            <Route path="*" element={null} />
          </Routes>
        </Shell>
      </BrowserRouter>
    </SessionBoundary>
  </QueryClientProvider>;
}
