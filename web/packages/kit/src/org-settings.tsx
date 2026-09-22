import { useState } from 'react';
import { get } from './http';
import { ChangePassword, MFASetup, useSession } from './session';
import { useData } from './shell';
import { Badge, Button, Details, Notice, Page, Panel } from './ui';

interface Org { name: string; legalName: string; country: { label: string }; region: { label: string } | null; registration: string; email: string; phone: string; address: string; access: string }
const kindLabel: Record<string, string> = { seller: 'Продавец', bank: 'Банк', mfo: 'МФО', insurance: 'Страховая компания' };

/** Settings of a provider cabinet: organisation profile, employee and security (reference layout). */
export function OrgSettings() {
  const s = useSession();
  const id = s.company?.id ?? '';
  const q = useData(['company', id], () => get<Org>(`/identity/companies/${id}`), !!id);
  const [form, setForm] = useState<'mfa' | 'password' | null>(null);
  const c = q.data?.data;
  const fact = (label: string, value: string | undefined) => <div className="fact"><span>{label}</span><strong>{value || '—'}</strong></div>;
  return <Page title="Настройки" subtitle="Организация и рабочий аккаунт">
    <Panel title="Профиль организации" padded>
      <div className="facts-grid">
        {fact('Организация', c?.name)}
        {fact('Тип', s.company ? kindLabel[s.company.kind] : undefined)}
        {fact('Страна и регион', c ? [c.country.label, c.region?.label].filter(Boolean).join(', ') : undefined)}
        {fact('Сотрудник', s.view.user.displayName)}
        {fact('Роли', s.view.roles.map((r) => r.name).join(', '))}
        {fact('Реквизиты', c ? [c.legalName, c.registration && `Рег. № ${c.registration}`].filter(Boolean).join(' · ') : undefined)}
      </div>
    </Panel>
    <Notice>Реквизиты организации, сотрудников и их роли меняет администратор платформы JustixAuto. Подключение кабинета не означает интеграцию с API банка или страховой.</Notice>
    <Panel title="Безопасность" padded>
      <Details items={[['Двухфакторная защита', s.view.mfa.enrolled ? <Badge tone="success">Включена</Badge> : <Badge tone="warning">Выключена — нужна для решений</Badge>]]} />
      <div className="kit-row" style={{ marginTop: 14 }}>
        {!s.view.mfa.enrolled && <Button variant="primary" onClick={() => setForm('mfa')}>Включить двухфакторную защиту</Button>}
        <Button onClick={() => setForm('password')}>Сменить пароль</Button>
      </div>
      {form === 'mfa' && <div style={{ marginTop: 16 }}><MFASetup onDone={() => { void s.refresh(); setForm(null); }} /></div>}
      {form === 'password' && <div style={{ marginTop: 16, maxWidth: 420 }}><ChangePassword onDone={() => { void s.refresh(); setForm(null); }} /></div>}
    </Panel>
  </Page>;
}
