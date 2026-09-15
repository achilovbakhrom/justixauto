// Developer-only presentation fixture. No business API, storage or demo credentials.
import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Dialog, type DialogScope } from '/web/packages/ui/src/Dialog.tsx';
import { Field } from '/web/packages/ui/src/Field.tsx';
import { Button } from '/web/packages/ui/src/Button.tsx';

function Fixture() {
  const [open, setOpen] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [value, setValue] = useState('');
  const [error, setError] = useState<string>();
  const [blocked, setBlocked] = useState('');
  const [writes, setWrites] = useState(0);
  const scope = (new URLSearchParams(location.search).get('scope') || 'dealer-shell') as DialogScope;
  return <main className={scope} style={{fontFamily:'var(--font-family)',fontSize:14,padding:32}}>
    <h1>T-034 — synthetic component fixture</h1><p>Shared primitives only; no full app or business operation.</p>
    <label><input type="checkbox" checked={dirty} onChange={e=>setDirty(e.target.checked)}/>Dirty guard</label>{' '}
    <label><input type="checkbox" checked={processing} onChange={e=>setProcessing(e.target.checked)}/>Processing guard</label>
    <p data-testid="writes">Synthetic writes: {writes}</p><p role="status">{blocked}</p>
    <Dialog scope={scope} open={open} onOpenChange={setOpen} trigger={<Button>Открыть форму</Button>}
      title="Проверка общей формы" description="Синтетический пример полей и клавиатурного управления."
      closeLabel="Закрыть" dirty={dirty} processing={processing}
      onDismissBlocked={(reason,state)=>setBlocked(`${state}: ${reason}`)}
      footer={<><Button disabled={processing} onClick={()=>{if(dirty)setBlocked('dirty: cancel');else setOpen(false)}}>Отмена</Button>
        <Button variant="primary" type="submit" form="fixture-form" processing={processing}>Сохранить</Button></>}>
      <form id="fixture-form" noValidate onSubmit={event=>{event.preventDefault();if(!value.trim())setError('Введите название');else if(!processing)setWrites(n=>n+1)}}>
        <Field label="Название" name="label" value={value} required hint="Только синтетические данные" error={error}
          onChange={event=>setValue(event.target.value)}/>
        <p>Длинная поясняющая подпись для проверки переноса текста при узком окне браузера.</p>
      </form>
    </Dialog>
  </main>;
}
createRoot(document.getElementById('root')!).render(<Fixture/>);
