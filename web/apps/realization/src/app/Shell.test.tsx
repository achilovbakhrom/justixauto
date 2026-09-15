import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { checkedFixture } from '@justixauto/test-kit';
import { RealizationApp } from './App';
import { Shell } from './Shell';
import { navigation, registeredPages } from '../routes/registry';

function Admit({ children }: { children: ReactNode }) { return children; }
const fixture = checkedFixture({ parse(value: unknown) {
  if (!value || typeof value !== 'object' || !('label' in value)
    || typeof value.label !== 'string' || Object.keys(value).length !== 1) throw new Error('Invalid shell fixture');
  return value as { label: string };
} }, { label: 'Synthetic Realization' });

afterEach(() => { cleanup(); vi.restoreAllMocks(); window.history.replaceState(null, '', '/'); });

describe('Realization shell boundary', () => {
  it('never admits protected content or an identity without the injected boundary', () => {
    window.history.replaceState(null, '', '/users/one');
    render(<RealizationApp SessionBoundary={() => null} context={{ company: fixture.label, branches: 'All synthetic', companyMark: 'QA', userMark: 'QA', locale: 'RU' }} />);
    expect(screen.queryByRole('main')).toBeNull();
    expect(screen.queryByText(fixture.label)).toBeNull();
    expect(document.querySelector('input')).toBeNull();
  });

  it('preserves catalog order and grouping without advertising missing feature links', async () => {
    window.history.replaceState(null, '', '/');
    render(<RealizationApp SessionBoundary={Admit} context={{ company: fixture.label, branches: 'All synthetic', companyMark: 'QA', userMark: 'QA', locale: 'RU' }} />);
    expect(screen.getByRole('main')).toBeDefined();
    expect(screen.getAllByText(fixture.label)).toHaveLength(2);
    const menu = document.querySelector('aside')!;
    expect([...menu.querySelectorAll<HTMLButtonElement>('button[data-page-id]')].map((button) => button.getAttribute('aria-label'))).toEqual(navigation.map((item) => item.label));
    expect(registeredPages).toHaveLength(0);
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    for (const button of menu.querySelectorAll<HTMLButtonElement>('button[data-page-id]')) {
      expect(button.disabled).toBe(true);
      await userEvent.click(button);
    }
    expect(window.location.pathname).toBe('/');
    await userEvent.tab();
    expect(document.activeElement).toBe(document.body);
    expect(screen.queryAllByRole('heading', { level: 1 })).toHaveLength(0);
  });

  it('keeps unknown Realization deep links scoped and rejects sibling app prefixes', () => {
    window.history.replaceState(null, '', '/companies/unknown');
    const first = render(<RealizationApp SessionBoundary={Admit} />);
    expect(screen.queryAllByRole('heading', { level: 1 })).toHaveLength(0);
    expect(window.location.pathname).toBe('/companies/unknown');
    first.unmount();
    window.history.replaceState(null, '', '/finance/');
    render(<RealizationApp SessionBoundary={Admit} />);
    expect(screen.queryByRole('main')).toBeNull();
  });

  it('has no default identity, persistence or network activity', () => {
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    const fetch = vi.spyOn(globalThis, 'fetch');
    render(<Shell><h1>Реестр недоступен</h1></Shell>);
    expect(screen.queryByText(/Демо|Synthetic/)).toBeNull();
    expect(storage).not.toHaveBeenCalled();
    expect(fetch).not.toHaveBeenCalled();
  });

  it('creates independent memory-only query clients for separately mounted apps', () => {
    const clients: object[] = [];
    function Observe({ children }: { children: ReactNode }) {
      clients.push(useQueryClient());
      return children;
    }
    window.history.replaceState(null, '', '/');
    const first = render(<RealizationApp SessionBoundary={Observe} />);
    first.unmount();
    render(<RealizationApp SessionBoundary={Observe} />);
    expect(clients).toHaveLength(2);
    expect(clients[0]).not.toBe(clients[1]);
  });
});
