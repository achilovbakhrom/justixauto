import { useState } from 'react';
import { InsuranceDialog, InsuranceTable, Page, Panel, Tabs, useCompanyNames, useInsuranceApplications } from '@justixauto/kit';
import { inQueue, queues } from './queues';

export function ApplicationsPage() {
  const q = useInsuranceApplications();
  const name = useCompanyNames();
  const [queue, setQueue] = useState('new');
  const [open, setOpen] = useState<string | null>(null);
  return <Page title="Заявки на страхование" subtitle="Продажи продавцов в собственную рассрочку">
    <Tabs value={queue} onChange={setQueue} tabs={queues} />
    <Panel><InsuranceTable rows={q.data?.filter((a) => inQueue(queue, a.status))} loading={q.isLoading} error={q.error}
      counterparty={(a) => name(a.sellerCompanyId)} onOpen={setOpen} /></Panel>
    {open && <InsuranceDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}
