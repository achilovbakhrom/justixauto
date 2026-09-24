import { useState } from 'react';
import type { ReactNode } from 'react';
import { Button, Modal, Notice, errorText, minorToMajor, money, toMinor, useRefresh } from '@justixauto/kit';
import { modelName, routeLabel, useModels } from './data';
import type { Terms } from './data';

export function TermsView({ terms, modelNameOf }: { terms: Terms; modelNameOf: (id: string) => string }) {
  return (
    <div className="kit-stack">
      <table className="kit-table">
        <thead>
          <tr>
            <th>Модель</th>
            <th>Кол-во</th>
            <th>Цена за ед.</th>
          </tr>
        </thead>
        <tbody>
          {terms.lines.map((l, i) => (
            <tr key={l.lineId ?? i}>
              <td>{modelNameOf(l.modelId)}</td>
              <td>{l.quantity}</td>
              <td>{money(l.unitPrice)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p>Маршрут: {routeLabel[terms.route] ?? terms.route}</p>
      {terms.deliveryTerms && <p>Поставка: {terms.deliveryTerms}</p>}
      {terms.warrantyTerms && <p>Гарантия: {terms.warrantyTerms}</p>}
      {terms.serviceTerms && <p>Сервис: {terms.serviceTerms}</p>}
      {terms.paymentSchedule.length > 0 && (
        <p>График оплаты: {terms.paymentSchedule.map((p) => `${money(p.amount)} до ${p.dueDate}`).join('; ')}</p>
      )}
    </div>
  );
}

interface LineDraft {
  modelId: string;
  quantity: string;
  price: string;
}

const cell = (child: ReactNode) => <td style={{ padding: 4 }}>{child}</td>;

function TermsLinesTable({
  lines,
  models,
  withPrices,
  setLine,
  setLines,
}: {
  lines: LineDraft[];
  models: ReturnType<typeof useModels>;
  withPrices: boolean;
  setLine: (i: number, patch: Partial<LineDraft>) => void;
  setLines: (update: (ls: LineDraft[]) => LineDraft[]) => void;
}) {
  return (
    <>
      <table className="kit-table">
        <thead>
          <tr>
            <th>Модель</th>
            <th>Кол-во</th>
            {withPrices && <th>Цена за ед.</th>}
            <th />
          </tr>
        </thead>
        <tbody>
          {lines.map((l, i) => (
            <tr key={i}>
              {cell(
                <select value={l.modelId} onChange={(e) => setLine(i, { modelId: e.target.value })} aria-label="Модель">
                  <option value="">—</option>
                  {(models.data ?? []).map((m) => (
                    <option key={m.id} value={m.id}>
                      {modelName(m)}
                    </option>
                  ))}
                </select>,
              )}
              {cell(
                <input
                  value={l.quantity}
                  onChange={(e) => setLine(i, { quantity: e.target.value })}
                  inputMode="numeric"
                  aria-label="Количество"
                  style={{ width: 80 }}
                />,
              )}
              {withPrices &&
                cell(
                  <input
                    value={l.price}
                    onChange={(e) => setLine(i, { price: e.target.value })}
                    inputMode="decimal"
                    placeholder="0.00"
                    aria-label="Цена"
                  />,
                )}
              {cell(
                lines.length > 1 && (
                  <Button variant="link" onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}>
                    Удалить
                  </Button>
                ),
              )}
            </tr>
          ))}
        </tbody>
      </table>
      <Button onClick={() => setLines((ls) => [...ls, { modelId: '', quantity: '1', price: '' }])}>
        Добавить строку
      </Button>
    </>
  );
}

function PaymentScheduleEditor({
  schedule,
  setSchedule,
}: {
  schedule: { amount: string; dueDate: string }[];
  setSchedule: (update: (s: { amount: string; dueDate: string }[]) => { amount: string; dueDate: string }[]) => void;
}) {
  return (
    <>
      <b>График оплаты (необязательно, сумма = итог)</b>
      {schedule.map((p, i) => (
        <div key={i} className="kit-row">
          <input
            value={p.amount}
            onChange={(e) => setSchedule((s) => s.map((x, j) => (j === i ? { ...x, amount: e.target.value } : x)))}
            placeholder="Сумма"
            aria-label="Сумма платежа"
          />
          <input
            type="date"
            value={p.dueDate}
            onChange={(e) => setSchedule((s) => s.map((x, j) => (j === i ? { ...x, dueDate: e.target.value } : x)))}
            aria-label="Дата платежа"
          />
          <Button variant="link" onClick={() => setSchedule((s) => s.filter((_, j) => j !== i))}>
            Удалить
          </Button>
        </div>
      ))}
      <Button onClick={() => setSchedule((s) => [...s, { amount: '', dueDate: '' }])}>Добавить платёж</Button>
    </>
  );
}

function ExtraFieldControl({
  f,
  more,
  setMore,
}: {
  f: ExtraField;
  more: Record<string, string | string[]>;
  setMore: (v: Record<string, string | string[]>) => void;
}) {
  if (f.multiple && f.options) {
    const picked = (more[f.name] as string[] | undefined) ?? [];
    return (
      <fieldset className="kit-field">
        <span>{f.label}</span>
        {f.options.map(([v, l]) => (
          <label key={v} className="kit-row">
            <input
              type="checkbox"
              checked={picked.includes(v)}
              onChange={(e) =>
                setMore({ ...more, [f.name]: e.target.checked ? [...picked, v] : picked.filter((x) => x !== v) })
              }
            />
            {l}
          </label>
        ))}
      </fieldset>
    );
  }
  return (
    <label className="kit-field">
      {f.label}
      {f.required ? ' *' : ''}
      {f.options ? (
        <select value={String(more[f.name] ?? '')} onChange={(e) => setMore({ ...more, [f.name]: e.target.value })}>
          <option value="">—</option>
          {f.options.map(([v, l]) => (
            <option key={v} value={v}>
              {l}
            </option>
          ))}
        </select>
      ) : (
        <textarea value={String(more[f.name] ?? '')} onChange={(e) => setMore({ ...more, [f.name]: e.target.value })} />
      )}
    </label>
  );
}

/**
 * Editor for the contract's CommercialTerms: model lines, one currency,
 * route, texts and an optional payment schedule that must add up to the total.
 */
function TermsDialog({
  title,
  initial,
  withPrices = true,
  onSubmit,
  onClose,
  extra,
}: {
  title: string;
  initial?: Terms | undefined;
  withPrices?: boolean;
  onClose: () => void;
  onSubmit: (terms: Terms, extra: Record<string, string | string[]>) => Promise<unknown>;
  extra?: ExtraField[];
}) {
  const models = useModels();
  const [currency, setCurrency] = useState(initial?.lines[0]?.unitPrice.currency ?? 'USD');
  const [lines, setLines] = useState<LineDraft[]>(
    initial?.lines.map((l) => ({
      modelId: l.modelId,
      quantity: l.quantity,
      price: minorToMajor(l.unitPrice.amountMinor),
    })) ?? [{ modelId: '', quantity: '1', price: '' }],
  );
  const [route, setRoute] = useState(initial?.route ?? 'local');
  const [texts, setTexts] = useState({
    deliveryTerms: initial?.deliveryTerms ?? '',
    warrantyTerms: initial?.warrantyTerms ?? '',
    serviceTerms: initial?.serviceTerms ?? '',
  });
  const [schedule, setSchedule] = useState<{ amount: string; dueDate: string }[]>(
    initial?.paymentSchedule.map((p) => ({ amount: minorToMajor(p.amount.amountMinor), dueDate: p.dueDate })) ?? [],
  );
  const [more, setMore] = useState<Record<string, string | string[]>>({});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const setLine = (i: number, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l, j) => (j === i ? { ...l, ...patch } : l)));

  async function submit() {
    setError('');
    const out: Terms = { lines: [], route, ...texts, paymentSchedule: [] };
    for (const l of lines) {
      const minor = withPrices ? toMinor(l.price) : '0';
      if (!l.modelId || minor === null) {
        setError('Заполните модель и цену в каждой строке');
        return;
      }
      out.lines.push({ modelId: l.modelId, quantity: l.quantity, unitPrice: { amountMinor: minor, currency } });
    }
    for (const p of schedule) {
      const minor = toMinor(p.amount);
      if (minor === null || !p.dueDate) {
        setError('Заполните сумму и дату каждого платежа');
        return;
      }
      out.paymentSchedule.push({ amount: { amountMinor: minor, currency }, dueDate: p.dueDate });
    }
    setBusy(true);
    try {
      await onSubmit(out, more);
      onClose();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title={title}
      onClose={onClose}
      size="wide"
      footer={
        <>
          <Button onClick={onClose}>Отмена</Button>
          <Button variant="primary" busy={busy} onClick={() => void submit()}>
            Сохранить
          </Button>
        </>
      }
    >
      {error && <Notice kind="danger">{error}</Notice>}
      <div className="kit-row">
        <label className="kit-field">
          Валюта
          <select value={currency} onChange={(e) => setCurrency(e.target.value)}>
            {['USD', 'UZS', 'EUR'].map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </label>
        <label className="kit-field">
          Маршрут
          <select value={route} onChange={(e) => setRoute(e.target.value)}>
            {Object.entries(routeLabel).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </label>
      </div>
      <TermsLinesTable lines={lines} models={models} withPrices={withPrices} setLine={setLine} setLines={setLines} />
      {withPrices && (
        <>
          <label className="kit-field">
            Условия поставки
            <textarea
              value={texts.deliveryTerms}
              onChange={(e) => setTexts({ ...texts, deliveryTerms: e.target.value })}
            />
          </label>
          <label className="kit-field">
            Гарантия
            <input
              value={texts.warrantyTerms}
              onChange={(e) => setTexts({ ...texts, warrantyTerms: e.target.value })}
            />
          </label>
          <label className="kit-field">
            Сервис
            <input value={texts.serviceTerms} onChange={(e) => setTexts({ ...texts, serviceTerms: e.target.value })} />
          </label>
          <PaymentScheduleEditor schedule={schedule} setSchedule={setSchedule} />
        </>
      )}
      {extra?.map((f) => (
        <ExtraFieldControl key={f.name} f={f} more={more} setMore={setMore} />
      ))}
    </Modal>
  );
}

/** A button opening TermsDialog; refreshes the given keys afterwards. */
/** Extra input next to the terms: text, a select, or checkboxes (multiple). */
export interface ExtraField {
  name: string;
  label: string;
  required?: boolean;
  options?: [string, string][];
  multiple?: boolean;
}

export function TermsButton(props: {
  label: string;
  initial?: Terms | undefined;
  withPrices?: boolean;
  variant?: 'primary';
  extra?: ExtraField[];
  refresh?: unknown[][];
  onSubmit: (t: Terms, extra: Record<string, string | string[]>) => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const reload = useRefresh();
  return (
    <>
      <Button variant={props.variant} onClick={() => setOpen(true)}>
        {props.label}
      </Button>
      {open && (
        <TermsDialog
          title={props.label}
          initial={props.initial}
          {...(props.withPrices === undefined ? {} : { withPrices: props.withPrices })}
          {...(props.extra ? { extra: props.extra } : {})}
          onClose={() => setOpen(false)}
          onSubmit={async (t, x) => {
            const r = await props.onSubmit(t, x);
            await reload(...(props.refresh ?? []));
            return r;
          }}
        />
      )}
    </>
  );
}
