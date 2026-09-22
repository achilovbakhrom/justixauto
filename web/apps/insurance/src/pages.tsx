import { useState } from 'react';
import {
  FilterSelect, InsuranceDialog, InsuranceTable, Page, Panel, ResultMeta, Stat, Stats, Toolbar, matches, plural, useCompanyNames,
  useInsuranceApplications, useSearchQuery,
} from '@justixauto/kit';
import { statusFilter } from './queues';

/** Applications of sellers addressed to this insurer; `overview` adds the status figures. */
function Applications({ overview }: { overview?: boolean }) {
  const q = useInsuranceApplications();
  const name = useCompanyNames();
  const [status, setStatus] = useState('');
  const [query, setQuery] = useSearchQuery();
  const [open, setOpen] = useState<string | null>(null);
  const all = q.data ?? [];
  const rows = all.filter((a) => (!status || a.status === status)
    && matches(query, name(a.sellerCompanyId), a.snapshot?.customer?.name, a.snapshot?.vehicle?.vin, a.snapshot?.vehicle?.model));
  const n = (s: string) => all.filter((a) => a.status === s).length;
  return <>
    {overview && <Stats>
      {statusFilter.slice(0, 4).map(([k, l]) => <Stat key={k} label={l} value={n(k)} onClick={() => setStatus(k)} />)}
    </Stats>}
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Клиент, VIN или продавец" onReset={() => setStatus('')}>
        <FilterSelect value={status} onChange={setStatus} all="Все статусы" options={statusFilter} />
      </Toolbar>
      <ResultMeta>{plural(rows.length, ['заявка', 'заявки', 'заявок'])} · черновики продавцов вам не видны</ResultMeta>
      <InsuranceTable rows={rows} loading={q.isLoading} error={q.error} counterparty={(a) => name(a.sellerCompanyId)} onOpen={setOpen} />
    </Panel>
    {open && <InsuranceDialog id={open} onClose={() => setOpen(null)} />}
  </>;
}

export function OverviewPage() {
  return <Page title="Обзор" subtitle="Заявки продавцов на рассмотрение"><Applications overview /></Page>;
}

export function ApplicationsPage() {
  return <Page title="Страхование рассрочки" subtitle="Заявки продавцов на рассмотрение"><Applications /></Page>;
}
