import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { checkedFixture } from '@justixauto/test-kit';
import { InsuranceApp } from './App';
import { Shell } from './Shell';
import type { ShellContext } from './Shell';
import { navigation, registeredPages } from '../routes/registry';

function Admit({ children }: { children: ReactNode }) { return children; }
const context = checkedFixture({ parse(value: unknown) {
  const keys = ['organizationName', 'accountLabel'];
  if (!value || typeof value !== 'object' || Object.keys(value).length !== keys.length
    || !keys.every(key => key in value && typeof (value as Record<string, unknown>)[key] === 'string')) throw new Error('Invalid shell fixture');
  return value as ShellContext;
} }, { organizationName: 'Synthetic Insurance', accountLabel: 'Synthetic account' });

afterEach(() => { cleanup(); vi.restoreAllMocks(); window.history.replaceState(null, '', '/insurance/'); });

describe('Insurance shell boundary', () => {
  it('never admits protected content or an identity without the injected boundary', () => {
    window.history.replaceState(null, '', '/insurance/users/one');
    render(<InsuranceApp SessionBoundary={() => null} context={context} />);
    expect(screen.queryByRole('main')).toBeNull();
    expect(screen.queryByText(context.organizationName, { selector: 'strong' })).toBeNull();
    expect(document.querySelector('input')).toBeNull();
  });

  it('preserves catalog order and grouping without advertising missing feature links', async () => {
    window.history.replaceState(null, '', '/insurance/');
    render(<InsuranceApp SessionBoundary={Admit} context={context} />);
    expect(screen.getByRole('main')).toBeDefined();
    expect(screen.getByText(context.organizationName, { selector: 'strong' })).toBeDefined();
    const menu = screen.getByRole('navigation', { name: 'Страховая' });
    expect([...menu.querySelectorAll('button')].map((button) => button.textContent)).toEqual(navigation.map((item) => item.label));
    expect(registeredPages).toHaveLength(0);
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    for (const button of menu.querySelectorAll('button')) {
      expect(button.disabled).toBe(true);
      await userEvent.click(button);
    }
    expect(window.location.pathname).toBe('/insurance/');
    await userEvent.tab();
    expect(document.activeElement).toBe(document.body);
    expect(screen.queryAllByRole('heading', { level: 1 })).toHaveLength(0);
  });

  it('keeps unknown Insurance deep links scoped and rejects sibling app prefixes', () => {
    window.history.replaceState(null, '', '/insurance/companies/unknown');
    const first = render(<InsuranceApp SessionBoundary={Admit} />);
    expect(screen.queryAllByRole('heading', { level: 1 })).toHaveLength(0);
    expect(window.location.pathname).toBe('/insurance/companies/unknown');
    first.unmount();
    window.history.replaceState(null, '', '/finance/');
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    render(<InsuranceApp SessionBoundary={Admit} />);
    expect(screen.queryByRole('main')).toBeNull();
    expect(warning).toHaveBeenCalled();
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
    window.history.replaceState(null, '', '/insurance/');
    const first = render(<InsuranceApp SessionBoundary={Observe} />);
    first.unmount();
    render(<InsuranceApp SessionBoundary={Observe} />);
    expect(clients).toHaveLength(2);
    expect(clients[0]).not.toBe(clients[1]);
  });
});
