import {
  ActionButton,
  Badge,
  Details,
  Modal,
  Panel,
  Table,
  date,
  dateTime,
  fileUrl,
  get,
  money,
  post,
  useData,
  useSession,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import {
  useVehicleLabel,
  stageOrder,
  stageLabel,
  channelLabel,
  sourceLabel,
  purposeLabel,
  schemeLabel,
  dealLabel,
} from '../data';
import type { Deal, RetailInvoice, Lead } from '../data';

const reason: FieldSpec[] = [{ name: 'reason', label: 'Причина', type: 'textarea', required: true }];

// ---------------- listings ----------------

// ---------------- CRM ----------------

const eventLabel: Record<string, string> = {
  'lead.created': 'Лид создан',
  'lead.assigned': 'Назначен ответственный',
  'lead.stage_changed': 'Смена этапа',
  'lead.won': 'Продажа',
  'deal.reserved': 'Сделка создана, автомобиль зарезервирован',
  'deal.contract_recorded': 'Договор подписан',
  'deal.invoice_issued': 'Выставлен счёт',
  'deal.payment_submitted': 'Внесена оплата',
  'deal.payment_accepted': 'Оплата принята',
  'deal.payment_rejected': 'Оплата отклонена',
  'deal.registered': 'Регистрация',
  'deal.delivered': 'Автомобиль выдан',
  'deal.cancelled': 'Сделка отменена',
};
function LeadDialogFooter({
  l,
  id,
  refresh,
  currentUserId,
}: {
  l: Lead;
  id: string;
  refresh: unknown[][];
  currentUserId: string;
}) {
  const next = stageOrder[stageOrder.indexOf(l.stage) + 1];
  return (
    <>
      <ActionButton
        label="Контакт"
        refresh={refresh}
        fields={[
          {
            name: 'channel',
            label: 'Канал',
            type: 'select',
            required: true,
            options: Object.entries(channelLabel),
          },
          { name: 'note', label: 'Итог', type: 'textarea', required: true },
        ]}
        onSubmit={(v) => post(`/retail/leads/${id}/contacts`, v)}
      />
      {l.assignedUserId !== currentUserId && (
        <ActionButton
          label="Взять себе"
          refresh={refresh}
          onSubmit={() =>
            post(`/retail/leads/${id}/assign`, { assignedUserId: currentUserId }, { ifMatch: l.revision })
          }
        />
      )}
      {next && (
        <ActionButton
          label={`→ ${stageLabel[next]}`}
          variant="primary"
          refresh={refresh}
          onSubmit={() => post(`/retail/leads/${id}/stage`, { stage: next }, { ifMatch: l.revision })}
        />
      )}
      <ActionButton
        label="Потерян"
        variant="danger"
        fields={reason}
        refresh={refresh}
        onSubmit={(v) =>
          post(`/retail/leads/${id}/stage`, { stage: 'lost', reason: v.reason }, { ifMatch: l.revision })
        }
      />
    </>
  );
}

function LeadDialogBody({ l, currentUserId }: { l: Lead; currentUserId: string }) {
  return (
    <>
      <Details
        items={[
          ['Этап', stageLabel[l.stage]],
          ['Источник', sourceLabel[l.source] ?? l.source],
          ['Телефон', l.customer?.phone || '—'],
          ['Ответственный', l.assignedUserId === currentUserId ? 'Я' : l.assignedUserId ? 'Назначен' : 'Не назначен'],
          ['Причина потери', l.lostReason || '—'],
          ['Сделка', l.dealId ? 'Создана' : '—'],
        ]}
      />
      <Panel title="Контакты" padded>
        <ul className="kit-timeline">
          {(l.contacts ?? []).map((c, i) => (
            <li key={i}>
              {dateTime(c.occurredAt)} — {channelLabel[c.channel] ?? c.channel}: {c.note}
            </li>
          ))}
        </ul>
      </Panel>
      <Panel title="История" padded>
        <ul className="kit-timeline">
          {(l.history ?? []).map((h, i) => (
            <li key={i}>
              {dateTime(h.occurredAt)} — {eventLabel[h.type] ?? h.type}
              {h.reason ? ` · ${h.reason}` : ''}
            </li>
          ))}
        </ul>
      </Panel>
    </>
  );
}

export function LeadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['lead', id], () => get<Lead>(`/retail/leads/${id}`));
  const s = useSession();
  const l = q.data?.data;
  const refresh = [['lead', id], ['leads']];
  const open = l && l.stage !== 'won' && l.stage !== 'lost';
  return (
    <Modal
      title={l ? `Лид: ${l.customer?.displayName ?? ''}` : 'Лид'}
      onClose={onClose}
      size="wide"
      footer={l && open && <LeadDialogFooter l={l} id={id} refresh={refresh} currentUserId={s.view.user.id} />}
    >
      {l && <LeadDialogBody l={l} currentUserId={s.view.user.id} />}
    </Modal>
  );
}

// ---------------- deals ----------------

const purposes: Record<string, string[]> = {
  cash: ['vehicle-payment', 'registration'],
  'own-installment': ['first-installment', 'registration'],
  'partner-finance': ['registration'],
};

const check = (ok: boolean | undefined) => (ok === undefined ? null : ok ? '✓' : '✗');

function DealDialogFooter({ d, id, refresh }: { d: Deal; id: string; refresh: unknown[][] }) {
  const can = (a: string) => d.allowedActions.includes(a);
  return (
    <>
      {can('record-contract') && (
        <ActionButton
          label="Договор подписан"
          refresh={refresh}
          fields={[
            { name: 'signedOn', label: 'Дата подписания', type: 'date', required: true },
            { name: 'reference', label: 'Номер договора', type: 'text', required: true },
            { name: 'file', label: 'Скан договора', type: 'file', purpose: 'deal-document' },
          ]}
          onSubmit={(v) =>
            post(
              `/retail/deals/${id}/contract-records`,
              { signedOn: v.signedOn, reference: v.reference, bindingIds: v.file ? [v.file] : [] },
              { ifMatch: d.revision },
            )
          }
        />
      )}
      {can('issue-invoice') && (
        <ActionButton
          label="Выставить счёт"
          refresh={refresh}
          fields={[
            {
              name: 'purpose',
              label: 'Назначение',
              type: 'select',
              required: true,
              options: (purposes[d.paymentScheme] ?? []).map((p) => [p, purposeLabel[p] ?? p]),
            },
            { name: 'amount', label: 'Сумма', type: 'money', required: true, currency: d.price.currency },
            {
              name: 'recipientSnapshot',
              label: 'Плательщик (как в счёте)',
              type: 'text',
              required: true,
              initial: d.customer.displayName,
            },
            { name: 'dueDate', label: 'Оплатить до', type: 'date', required: true },
          ]}
          onSubmit={(v) => post(`/retail/deals/${id}/invoices`, v, { ifMatch: d.revision })}
        />
      )}
      {can('record-registration') && (
        <ActionButton
          label="Регистрация"
          refresh={refresh}
          fields={[
            { name: 'registeredOn', label: 'Дата регистрации', type: 'date', required: true },
            { name: 'plateNumber', label: 'Госномер', type: 'text', required: true },
            { name: 'reference', label: 'Номер свидетельства', type: 'text', required: true },
          ]}
          onSubmit={(v) => post(`/retail/deals/${id}/registration`, v, { ifMatch: d.revision })}
        />
      )}
      {can('deliver') && (
        <ActionButton
          label="Выдать автомобиль"
          variant="primary"
          refresh={refresh}
          fields={[{ name: 'occurredAt', label: 'Когда выдан', type: 'datetime', required: true }]}
          intro={<p>Автомобиль будет списан со склада. Действие необратимо.</p>}
          onSubmit={(v) => post(`/retail/deals/${id}/deliveries`, v, { ifMatch: d.revision })}
        />
      )}
      {can('cancel') && (
        <ActionButton
          label="Отменить сделку"
          variant="danger"
          fields={reason}
          refresh={refresh}
          onSubmit={(v) => post(`/retail/deals/${id}/cancel`, v, { ifMatch: d.revision })}
        />
      )}
    </>
  );
}

function DealDialogBody({
  d,
  label,
  refresh,
}: {
  d: Deal;
  label: (vehicleId: string) => string;
  refresh: unknown[][];
}) {
  const c = d.checklist;
  return (
    <>
      <Details
        items={[
          ['Автомобиль', label(d.vehicleId)],
          ['Схема', schemeLabel[d.paymentScheme]],
          ['Цена', money(d.price)],
          ['Статус', dealLabel[d.status]],
          [
            'Договор',
            d.contractSignedOn ? (
              <>
                {d.contractReference} от {date(d.contractSignedOn)}
                {d.contractFileIds.map((f, i) => (
                  <span key={f}>
                    {' '}
                    ·{' '}
                    <a href={fileUrl(f)} target="_blank" rel="noreferrer">
                      скан {i + 1}
                    </a>
                  </span>
                ))}
              </>
            ) : (
              '—'
            ),
          ],
          [
            'Регистрация',
            d.registeredOn ? `${d.plateNumber} (${d.registrationReference}) от ${date(d.registeredOn)}` : '—',
          ],
          ['Выдан', dateTime(d.deliveredAt)],
          ['Причина отмены', d.statusReason || '—'],
        ]}
      />
      {c && d.status === 'reserved' && (
        <Panel title="Готовность к выдаче" padded>
          <ul className="kit-timeline">
            <li>{check(c.contract)} Договор</li>
            {c.vehiclePayment !== undefined && <li>{check(c.vehiclePayment)} Оплата автомобиля</li>}
            {c.firstInstallment !== undefined && <li>{check(c.firstInstallment)} Первый взнос</li>}
            {c.insuranceApproved !== undefined && <li>{check(c.insuranceApproved)} Одобрение страховой</li>}
            <li>{check(c.registrationPaid)} Оплата регистрации</li>
            <li>{check(c.registered)} Регистрация</li>
            {!c.policyResolved && <li>✗ Правила выдачи при финансировании банком/МФО ещё не утверждены (OD-01)</li>}
          </ul>
        </Panel>
      )}
      {(d.invoices ?? []).map((i) => (
        <RetailInvoicePanel key={i.id} invoice={i} refresh={refresh} />
      ))}
      <Panel title="История" padded>
        <ul className="kit-timeline">
          {(d.history ?? []).map((h, i) => (
            <li key={i}>
              {dateTime(h.occurredAt)} — {eventLabel[h.type] ?? h.type}
              {h.reason ? ` · ${h.reason}` : ''}
            </li>
          ))}
        </ul>
      </Panel>
    </>
  );
}

export function DealDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['deal', id], () => get<Deal>(`/retail/deals/${id}`));
  const label = useVehicleLabel();
  const d = q.data?.data;
  const refresh = [['deal', id], ['deals'], ['vehicles'], ['leads']];
  return (
    <Modal
      title={d ? `Сделка: ${d.customer.displayName}` : 'Сделка'}
      onClose={onClose}
      size="wide"
      footer={d && <DealDialogFooter d={d} id={id} refresh={refresh} />}
    >
      {d && <DealDialogBody d={d} label={label} refresh={refresh} />}
    </Modal>
  );
}

export function RetailInvoicePanel({ invoice: i, refresh }: { invoice: RetailInvoice; refresh: unknown[][] }) {
  return (
    <Panel
      title={`${purposeLabel[i.purpose] ?? i.purpose}: ${money(i.amount)} · оплачено ${money(i.paid)} · остаток ${money(i.outstanding)}`}
      actions={
        i.status === 'issued' &&
        i.outstanding.amountMinor !== '0' && (
          <ActionButton
            label="Внести оплату"
            refresh={refresh}
            fields={[
              { name: 'claimedAmount', label: 'Сумма', type: 'money', required: true, currency: i.amount.currency },
              { name: 'paidOn', label: 'Дата оплаты', type: 'date', required: true },
              { name: 'externalReference', label: 'Номер платёжки / чека', type: 'text', required: true },
              { name: 'file', label: 'Подтверждение (файл)', type: 'file', purpose: 'payment-evidence' },
            ]}
            onSubmit={(v) =>
              post(`/retail/invoices/${i.id}/evidence`, {
                claimedAmount: v.claimedAmount,
                paidOn: v.paidOn,
                externalReference: v.externalReference,
                attachmentBindingIds: v.file ? [v.file] : [],
              })
            }
          />
        )
      }
    >
      <Table
        rows={i.paymentEvidence}
        rowKey={(e) => e.id}
        empty="Оплат пока нет"
        columns={[
          { title: 'Сумма', render: (e) => money(e.amount) },
          { title: 'Дата', render: (e) => e.paidOn },
          {
            title: 'Документ',
            render: (e) => (
              <>
                {e.externalReference}{' '}
                {e.attachmentIds.map((f) => (
                  <a key={f} href={fileUrl(f)} target="_blank" rel="noreferrer">
                    файл
                  </a>
                ))}
              </>
            ),
          },
          {
            title: 'Статус',
            render: (e) => (
              <Badge tone={e.status === 'accepted' ? 'success' : e.status === 'rejected' ? 'danger' : 'warning'}>
                {
                  (
                    {
                      submitted: 'На проверке',
                      accepted: 'Принято',
                      rejected: `Отклонено: ${e.decisionReason}`,
                    } as Record<string, string>
                  )[e.status]
                }
              </Badge>
            ),
          },
          {
            title: '',
            render: (e) =>
              e.status === 'submitted' && (
                <div className="kit-row">
                  <ActionButton
                    label="Принять"
                    variant="primary"
                    refresh={refresh}
                    fields={[{ name: 'confirmation', label: 'Деньги поступили', type: 'checkbox' }]}
                    onSubmit={(v) => post(`/retail/evidence/${e.id}/accept`, v, { ifMatch: e.revision })}
                  />
                  <ActionButton
                    label="Отклонить"
                    fields={reason}
                    refresh={refresh}
                    onSubmit={(v) => post(`/retail/evidence/${e.id}/reject`, v, { ifMatch: e.revision })}
                  />
                </div>
              ),
          },
        ]}
      />
    </Panel>
  );
}
