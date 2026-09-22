/**
 * Insurance and partner-finance applications, shared by the seller cabinet
 * (Realization) and the provider cabinets (Financing, Insurance). The server
 * decides who may do what (allowedActions); these views only render it.
 */
import { useState } from 'react';
import { fileUrl, get, list, patch, post } from './http';
import { ActionButton, useData } from './shell';
import { Badge, Button, Details, Modal, Notice, Panel, Table, date, dateTime, money } from './ui';
import type { FieldSpec } from './ui';

type Money = { amountMinor: string; currency: string };
/** Sale facts frozen at submission; drafts have no snapshot yet. */
type SaleSnapshot = { price?: Money; paymentScheme?: string; vehicleId?: string; vehicle?: { vin: string; model: string }; customer?: { name: string } } | null;

const vehicleText = (s: SaleSnapshot) => s?.vehicle ? `${s.vehicle.model} · VIN ${s.vehicle.vin}` : '—';

// Queues change on the other side (seller or provider); poll them.
const QUEUE_POLL_MS = 30_000;

/** Company id → name from the directory of active companies. */
export function useCompanyNames() {
  const q = useData(['directory-all'], () => list<{ id: string; name: string }>('/identity/directory/companies?limit=200'));
  return (id: string) => q.data?.find((c) => c.id === id)?.name ?? '—';
}

const note: FieldSpec[] = [{ name: 'note', label: 'Сообщение', type: 'textarea', required: true }];
const reasonField: FieldSpec[] = [{ name: 'reason', label: 'Причина', type: 'textarea', required: true }];

interface Message { kind: string; note: string; side: string; occurredAt: string; termsVersion?: number | null }

const messageLabel: Record<string, string> = {
  submitted: 'Заявка отправлена', taken: 'Взята в работу', request: 'Запрос информации', response: 'Ответ продавца',
  approved: 'Одобрено', declined: 'Отказ', terms: 'Условия', counter: 'Встречное предложение', agreed: 'Условия приняты', take: 'Взята в работу',
};

function Thread({ messages }: { messages: Message[] | undefined }) {
  return <Panel title="Переписка" padded><ul className="kit-timeline">{(messages ?? []).map((m, i) =>
    <li key={i}><b>{dateTime(m.occurredAt)} · {m.side === 'seller' ? 'Продавец' : 'Партнёр'}</b> — {messageLabel[m.kind] ?? m.kind}
      {m.termsVersion ? ` (условия №${m.termsVersion})` : ''}{m.note ? `: ${m.note}` : ''}</li>)}</ul></Panel>;
}

export const applicationStatusLabel: Record<string, string> = {
  draft: 'Черновик', submitted: 'Отправлена', review: 'На рассмотрении', 'needs-info': 'Нужна информация',
  approved: 'Одобрена', declined: 'Отказ', terms: 'Предложены условия', agreed: 'Условия приняты',
};
export const applicationTone = (s: string) =>
  s === 'approved' || s === 'agreed' ? 'success' : s === 'declined' ? 'danger' : s === 'needs-info' || s === 'terms' ? 'warning' : 'info';

/** Provider-side wording of the reference ("Новая заявка", "Условия отправлены"). */
const providerLabel: Record<string, string> = { submitted: 'Новая заявка', 'needs-info': 'Нужны сведения', terms: 'Условия отправлены', agreed: 'Условия согласованы', declined: 'Отказ', approved: 'Одобрена' };
const sellerLabel: Record<string, string> = { terms: 'Условия получены', agreed: 'Условия согласованы', declined: 'Отказ' };

export function StatusBadge({ status, side }: { status: string; side?: string | undefined }) {
  const label = (side === 'seller' ? sellerLabel[status] : side ? providerLabel[status] : undefined) ?? applicationStatusLabel[status] ?? status;
  return <Badge tone={applicationTone(status)}>{label}</Badge>;
}

/** Latest revision of the seller's sale, confirmed on submit (the snapshot is taken from it). */
async function dealRevision(dealId: string) {
  return (await get<unknown>(`/retail/deals/${dealId}`)).revision;
}

// ---------------- insurance ----------------

export interface InsuranceApplication {
  id: string; side: 'seller' | 'insurer'; sellerCompanyId: string; insurerCompanyId: string; retailDealId: string; status: string;
  note: string; snapshot: SaleSnapshot; history?: Message[];
  allowedActions: string[]; revision: string; submittedAt: string | null; decidedAt: string | null;
}

export function useInsuranceApplications() {
  return useData(['insurance-applications'], () => list<InsuranceApplication>('/insurance/applications?limit=100'), true, QUEUE_POLL_MS);
}

export function InsuranceTable({ rows, loading, error, counterparty, onOpen }: {
  rows: InsuranceApplication[] | undefined; loading: boolean; error: unknown; counterparty: (a: InsuranceApplication) => string; onOpen: (id: string) => void;
}) {
  return <Table rows={rows} loading={loading} error={error} rowKey={(a) => a.id} onRowClick={(a) => onOpen(a.id)} empty="Заявок нет" columns={[
    { title: 'Заявка / продавец', render: (a) => <><div className="cell-main">{counterparty(a)}</div><div className="cell-sub">{a.submittedAt ? `Отправлена ${date(a.submittedAt)}` : 'Черновик'}</div></> },
    { title: 'Клиент / автомобиль', render: (a) => <><div className="cell-main">{a.snapshot?.customer?.name ?? '—'}</div><div className="cell-sub">{a.snapshot?.vehicle ? `${a.snapshot.vehicle.model} · ${a.snapshot.vehicle.vin}` : ''}</div></> },
    { title: 'Цена автомобиля', render: (a) => money(a.snapshot?.price) },
    { title: 'Статус', render: (a) => <StatusBadge status={a.status} side={a.side} /> },
    { title: 'Действие', render: (a) => <Button size="sm" onClick={() => onOpen(a.id)}>Открыть</Button> },
  ]} />;
}

export function InsuranceDialog({ id, onClose, describeDeal }: { id: string; onClose: () => void; describeDeal?: (dealId: string) => string }) {
  const q = useData(['insurance-application', id], () => get<InsuranceApplication>(`/insurance/applications/${id}`));
  const name = useCompanyNames();
  const a = q.data?.data;
  const refresh = [['insurance-application', id], ['insurance-applications'], ['deal']];
  const act = (action: string, path: string, label: string, fields: FieldSpec[], variant?: 'primary' | 'danger') =>
    a?.allowedActions.includes(action) && <ActionButton key={action} label={label} fields={fields} refresh={refresh} {...(variant ? { variant } : {})}
      onSubmit={(v) => post(`/insurance/applications/${id}/${path}`, v, { ifMatch: a.revision })} />;
  return <Modal title="Заявка на страхование" onClose={onClose} size="wide" footer={a && <>
    {a.allowedActions.includes('edit') && <ActionButton label="Изменить" refresh={refresh} fields={[
      { name: 'note', label: 'Комментарий для страховой', type: 'textarea', initial: a.note },
    ]} onSubmit={(v) => patch(`/insurance/applications/${id}`, { insurerCompanyId: a.insurerCompanyId, note: v.note }, { ifMatch: a.revision })} />}
    {a.allowedActions.includes('submit') && <ActionButton label="Отправить" variant="primary" refresh={refresh}
      fields={[{ name: 'confirmation', label: 'Данные сделки проверены; страховая получит их снимок', type: 'checkbox' }]}
      onSubmit={async (v) => post(`/insurance/applications/${id}/submit`, { confirmation: v.confirmation, dealRevision: await dealRevision(a.retailDealId) }, { ifMatch: a.revision })} />}
    {act('respond', 'responses', 'Ответить', note, 'primary')}
    {act('take', 'take', 'Взять в работу', [], 'primary')}
    {act('request', 'information-requests', 'Запросить информацию', note)}
    {act('approve', 'approve', 'Одобрить', note, 'primary')}
    {act('decline', 'decline', 'Отказать', note, 'danger')}
  </>}>
    {a && <>
      <Details items={[
        ['Статус', <StatusBadge status={a.status} side={a.side} />],
        [a.side === 'seller' ? 'Страховая' : 'Продавец', name(a.side === 'seller' ? a.insurerCompanyId : a.sellerCompanyId)],
        ['Сделка', describeDeal ? describeDeal(a.retailDealId) : a.retailDealId.slice(0, 8)],
        ['Автомобиль', vehicleText(a.snapshot)],
        ['Клиент', a.snapshot?.customer?.name ?? '—'],
        ['Цена автомобиля', money(a.snapshot?.price)],
        ['Комментарий продавца', a.note || '—'],
        ['Решение', dateTime(a.decidedAt)],
      ]} />
      {a.status === 'needs-info' && a.side === 'seller' && <Notice kind="warning">Страховая запросила информацию — ответьте в переписке.</Notice>}
      <Thread messages={a.history} />
    </>}
  </Modal>;
}

// ---------------- financing ----------------

export interface Calculation {
  price: Money; input: { downPayment: Money; termMonths: number; firstDueDate: string };
  output: { markup: Money; total: Money; financedAmount: Money; schedule: { number: number; dueDate: string; total: Money; balance: Money }[] };
  programId: string; programVersion: number; calculatedAt: string;
}
export interface FinanceApplication {
  id: string; side: 'seller' | 'provider'; sellerCompanyId: string; providerCompanyId: string; retailDealId: string;
  programId: string | null; programVersion: number | null; calculation: Calculation | null; calculationDigest: string; status: string;
  snapshot: SaleSnapshot; currentTermsVersion: number | null;
  terms?: { number: number; calculation: Calculation; note: string; createdAt: string }[]; history?: Message[]; allowedActions: string[]; revision: string;
}
export interface ProgramVersion { number: string; name: string; currency: string; terms: { markupBps: number; minDownPaymentBps: number; termMonths: number[] }; eligibility: { minPriceMinor: string; maxPriceMinor: string }; createdAt: string }
export interface Program { id: string; provider: { id: string; name: string; kind: string }; status: string; statusReason: string; publishedVersion: number | null; versions: ProgramVersion[]; revision: string }

export const percent = (bps: number) => `${(bps / 100).toLocaleString('ru-RU')}%`;

export function useFinanceApplications() {
  return useData(['finance-applications'], () => list<FinanceApplication>('/financing/applications?limit=100'), true, QUEUE_POLL_MS);
}

export function FinanceTable({ rows, loading, error, counterparty, onOpen, programName }: {
  rows: FinanceApplication[] | undefined; loading: boolean; error: unknown; counterparty: (a: FinanceApplication) => string; onOpen: (id: string) => void;
  programName?: (a: FinanceApplication) => string;
}) {
  return <Table rows={rows} loading={loading} error={error} rowKey={(a) => a.id} onRowClick={(a) => onOpen(a.id)} empty="Заявок нет" columns={[
    { title: 'Заявка / продавец', render: (a) => <><div className="cell-main">{counterparty(a)}</div>{a.calculation && <div className="cell-sub">{a.calculation.input.termMonths} мес. · взнос {money(a.calculation.input.downPayment)}</div>}</> },
    { title: 'Клиент', render: (a) => a.snapshot?.customer?.name ?? '—' },
    { title: 'Автомобиль', render: (a) => a.snapshot?.vehicle ? <><div className="cell-main">{a.snapshot.vehicle.model}</div><div className="cell-sub">{a.snapshot.vehicle.vin}</div></> : '—' },
    ...(programName ? [{ title: 'Программа', render: programName }] : []),
    { title: 'Цена автомобиля', render: (a) => money(a.snapshot?.price ?? a.calculation?.price) },
    { title: 'Статус', render: (a) => <StatusBadge status={a.status} side={a.side} /> },
    { title: 'Действие', render: (a) => <Button size="sm" onClick={() => onOpen(a.id)}>Открыть</Button> },
  ]} />;
}

export function CalculationView({ calc, title }: { calc: Calculation | null | undefined; title: string }) {
  const [open, setOpen] = useState(false);
  if (!calc) return null;
  return <Panel title={title} padded actions={<Button variant="secondary" size="sm" onClick={() => setOpen(!open)}>{open ? 'Скрыть график' : 'График платежей'}</Button>}>
    <Details items={[['Цена', money(calc.price)], ['Первый взнос', money(calc.input.downPayment)], ['Наценка', money(calc.output.markup)],
      ['Финансируется', money(calc.output.financedAmount)], ['Итого к оплате', money(calc.output.total)],
      ['Срок', `${calc.input.termMonths} мес., первый платёж ${date(calc.input.firstDueDate)}`]]} />
    {open && <table className="kit-table"><thead><tr><th>№</th><th>Дата</th><th>Платёж</th><th>Остаток</th></tr></thead>
      <tbody>{calc.output.schedule.map((r) => <tr key={r.number}><td>{r.number}</td><td>{date(r.dueDate)}</td><td>{money(r.total)}</td><td>{money(r.balance)}</td></tr>)}</tbody></table>}
  </Panel>;
}

const calcFields = (currency: string, terms: number[]): FieldSpec[] => [
  { name: 'downPayment', label: 'Первый взнос', type: 'money', required: true, currency },
  { name: 'termMonths', label: 'Срок, месяцев', type: 'select', required: true, options: terms.map((t) => [String(t), `${t} мес.`]) },
  { name: 'firstDueDate', label: 'Дата первого платежа', type: 'date', required: true },
];
const calcInputs = (v: Record<string, unknown>) => ({ downPayment: v.downPayment, termMonths: Number(v.termMonths), firstDueDate: v.firstDueDate });

export function FinanceDialog({ id, onClose, describeDeal }: { id: string; onClose: () => void; describeDeal?: (dealId: string) => string }) {
  const q = useData(['finance-application', id], () => get<FinanceApplication>(`/financing/applications/${id}`));
  const name = useCompanyNames();
  const a = q.data?.data;
  const program = useData(['program', a?.programId], () => get<Program>(`/financing/programs/${a!.programId}`), !!a?.programId);
  const refresh = [['finance-application', id], ['finance-applications'], ['document-requests', id]];
  const version = program.data?.data.versions.find((v) => Number(v.number) === a?.programVersion);
  const currency = a?.calculation?.price.currency ?? 'USD';
  const termOptions = version?.terms.termMonths ?? (a?.calculation ? [a.calculation.input.termMonths] : [12]);
  const can = (x: string) => a?.allowedActions.includes(x) ?? false;
  const current = a?.terms?.find((t) => t.number === a.currentTermsVersion);
  return <Modal title="Заявка на финансирование" onClose={onClose} size="wide" footer={a && <>
    {can('edit') && <ActionButton label="Пересчитать" refresh={refresh} fields={calcFields(currency, termOptions)}
      onSubmit={(v) => patch(`/financing/applications/${id}`, { retailDealId: a.retailDealId, providerCompanyId: a.providerCompanyId,
        programId: a.programId, programVersion: a.programVersion, calculationInputs: calcInputs(v) }, { ifMatch: a.revision })} />}
    {can('submit') && <ActionButton label="Отправить" variant="primary" refresh={refresh}
      fields={[{ name: 'confirmation', label: 'Расчёт и данные сделки проверены', type: 'checkbox' }]}
      onSubmit={async (v) => post(`/financing/applications/${id}/submit`, { confirmation: v.confirmation,
        dealRevision: await dealRevision(a.retailDealId), calculationDigest: a.calculationDigest }, { ifMatch: a.revision })} />}
    {can('respond') && <ActionButton label="Ответить" variant="primary" fields={note} refresh={refresh}
      onSubmit={(v) => post(`/financing/applications/${id}/responses`, v, { ifMatch: a.revision })} />}
    {can('agree') && <ActionButton label={`Принять условия №${a.currentTermsVersion}`} variant="primary" refresh={refresh}
      fields={[{ name: 'confirmation', label: 'Клиент согласен с условиями и графиком', type: 'checkbox' }]}
      onSubmit={(v) => post(`/financing/applications/${id}/agree`, { confirmation: v.confirmation, termsVersion: a.currentTermsVersion }, { ifMatch: a.revision })} />}
    {can('counter') && <ActionButton label="Встречное предложение" fields={note} refresh={refresh}
      onSubmit={(v) => post(`/financing/applications/${id}/counter`, { note: v.note, termsVersion: a.currentTermsVersion }, { ifMatch: a.revision })} />}
    {can('take') && <ActionButton label="Взять в работу" variant="primary" refresh={refresh} onSubmit={() => post(`/financing/applications/${id}/take`, {}, { ifMatch: a.revision })} />}
    {can('request') && <ActionButton label="Запросить информацию" fields={note} refresh={refresh}
      onSubmit={(v) => post(`/financing/applications/${id}/information-requests`, v, { ifMatch: a.revision })} />}
    {can('terms') && <ActionButton label="Предложить условия" variant="primary" refresh={refresh} fields={[...calcFields(currency, termOptions), ...note]}
      onSubmit={(v) => post(`/financing/applications/${id}/terms`, { note: v.note, calculationInputs: calcInputs(v) }, { ifMatch: a.revision })} />}
    {can('decline') && <ActionButton label="Отказать" variant="danger" fields={reasonField} refresh={refresh}
      onSubmit={(v) => post(`/financing/applications/${id}/decline`, v, { ifMatch: a.revision })} />}
  </>}>
    {a && <>
      <Details items={[
        ['Статус', <StatusBadge status={a.status} side={a.side} />],
        [a.side === 'seller' ? 'Банк / МФО' : 'Продавец', name(a.side === 'seller' ? a.providerCompanyId : a.sellerCompanyId)],
        ['Программа', version ? `${version.name} (версия ${version.number}, наценка ${percent(version.terms.markupBps)})` : '—'],
        ['Сделка', describeDeal ? describeDeal(a.retailDealId) : a.retailDealId.slice(0, 8)],
        ['Автомобиль', vehicleText(a.snapshot)],
        ['Клиент', a.snapshot?.customer?.name ?? '—'],
      ]} />
      {a.status === 'needs-info' && a.side === 'seller' && <Notice kind="warning">Партнёр запросил информацию — ответьте в переписке.</Notice>}
      <CalculationView calc={a.calculation} title="Расчёт продавца" />
      {current && <CalculationView calc={current.calculation} title={`Условия №${current.number}: ${current.note}`} />}
      <Thread messages={a.history} />
      {a.status === 'agreed' && <DocumentRequests applicationId={a.id} side={a.side} />}
    </>}
  </Modal>;
}

// ---------------- document exchange after agreement ----------------

interface DocumentRequest { id: string; title: string; requirements: string; status: string; statusNote: string; submissions: { version: number; fileId: string; note: string; createdAt: string }[]; revision: string }
const docLabel: Record<string, string> = { requested: 'Запрошен', review: 'На проверке', changes: 'Нужны исправления', accepted: 'Принят', cancelled: 'Отменён' };

export function DocumentRequests({ applicationId, side }: { applicationId: string; side: string }) {
  const q = useData(['document-requests', applicationId], () => list<DocumentRequest>(`/financing/applications/${applicationId}/document-requests`));
  const refresh = [['document-requests', applicationId]];
  return <Panel title="Документы" actions={side === 'provider' && <ActionButton label="Запросить документ" refresh={refresh} fields={[
    { name: 'title', label: 'Документ', type: 'text', required: true },
    { name: 'requirements', label: 'Требования', type: 'textarea', required: true },
  ]} onSubmit={(v) => post(`/financing/applications/${applicationId}/document-requests`, v)} />}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(d) => d.id} empty="Документы не запрашивались" columns={[
      { title: 'Документ', render: (d) => <><b>{d.title}</b><div className="cell-sub">{d.requirements}</div></> },
      { title: 'Статус', render: (d) => <><Badge tone={d.status === 'accepted' ? 'success' : d.status === 'changes' ? 'warning' : undefined}>{docLabel[d.status]}</Badge>
        {d.statusNote && <div className="cell-sub">{d.statusNote}</div>}</> },
      { title: 'Файлы', render: (d) => d.submissions.map((s) => <div key={s.version}><a href={fileUrl(s.fileId)} target="_blank" rel="noreferrer">Версия {s.version}</a> · {dateTime(s.createdAt)}{s.note ? ` · ${s.note}` : ''}</div>) },
      { title: '', render: (d) => {
        const last = d.submissions[d.submissions.length - 1];
        return <div className="kit-row">
          {side === 'seller' && (d.status === 'requested' || d.status === 'changes') && <ActionButton label="Загрузить" variant="primary" refresh={refresh} fields={[
            { name: 'file', label: 'Файл', type: 'file', purpose: 'finance-document', required: true },
            { name: 'note', label: 'Комментарий', type: 'textarea' },
          ]} onSubmit={(v) => post(`/financing/document-requests/${d.id}/submissions`, { attachmentBindingId: v.file, note: v.note ?? '' }, { ifMatch: d.revision })} />}
          {side === 'provider' && d.status === 'review' && last && <>
            <ActionButton label="Принять" variant="primary" refresh={refresh} fields={[{ name: 'confirmation', label: 'Документ проверен', type: 'checkbox' }]}
              onSubmit={(v) => post(`/financing/document-requests/${d.id}/accept`, { submissionVersion: last.version, confirmation: v.confirmation }, { ifMatch: d.revision })} />
            <ActionButton label="Вернуть" fields={note} refresh={refresh}
              onSubmit={(v) => post(`/financing/document-requests/${d.id}/return`, { submissionVersion: last.version, note: v.note }, { ifMatch: d.revision })} />
          </>}
          {side === 'provider' && (d.status === 'requested' || d.status === 'changes') && <ActionButton label="Отменить" fields={reasonField} refresh={refresh}
            onSubmit={(v) => post(`/financing/document-requests/${d.id}/cancel`, v, { ifMatch: d.revision })} />}
        </div>;
      } },
    ]} />
  </Panel>;
}
