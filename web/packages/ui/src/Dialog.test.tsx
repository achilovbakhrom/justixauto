import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dialog } from './Dialog';
import type { DialogProps } from './Dialog';
import { Field } from './Field';
import { Button } from './Button';

afterEach(cleanup);

function Fixture({
  dirty = false,
  processing = false,
  onDismissBlocked,
  write = () => undefined,
}: Partial<Pick<DialogProps, 'dirty' | 'processing' | 'onDismissBlocked'>> & { write?: () => void }) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string>();
  const [name, setName] = useState('');
  return (
    <>
      <button>Outside</button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        trigger={<Button>Open form</Button>}
        scope="ins-workspace"
        title="Edit fixture"
        description="Synthetic form; no server command"
        closeLabel="Close"
        dirty={dirty}
        processing={processing}
        {...(onDismissBlocked ? { onDismissBlocked } : {})}
        footer={
          <>
            <Button
              disabled={processing}
              onClick={() => {
                if (!dirty && !processing) setOpen(false);
              }}
            >
              Cancel
            </Button>
            <Button type="submit" form="fixture-form" processing={processing}>
              Save
            </Button>
          </>
        }
      >
        <form
          id="fixture-form"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            if (!name.trim()) {
              setError('Name is required');
              return;
            }
            if (!processing) write();
          }}
        >
          <Field
            label="Name"
            name="name"
            hint="Enter a synthetic label"
            required
            value={name}
            {...(error ? { error } : {})}
            onChange={(event) => setName(event.target.value)}
          />
          <button type="button">Inside action</button>
        </form>
      </Dialog>
    </>
  );
}

describe('Dialog accessibility and dismissal boundaries', () => {
  it('names/describes the dialog, traps tab focus and restores the trigger', async () => {
    const user = userEvent.setup();
    render(<Fixture />);
    const trigger = screen.getByRole('button', { name: 'Open form' });
    await user.click(trigger);
    const dialog = screen.getByRole('dialog', { name: 'Edit fixture' });
    expect(document.getElementById(dialog.getAttribute('aria-describedby')!)?.textContent).toBe(
      'Synthetic form; no server command',
    );
    expect(dialog.contains(document.activeElement)).toBe(true);
    screen.getByRole('button', { name: 'Save' }).focus();
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Close' }));
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Save' }));
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it('preserves inside interaction and closes only on a clean outside pointer', async () => {
    const user = userEvent.setup();
    render(<Fixture />);
    await user.click(screen.getByRole('button', { name: 'Open form' }));
    await user.click(screen.getByRole('button', { name: 'Inside action' }));
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(screen.getByRole('dialog').closest('.jx-dialog-overlay')?.classList.contains('ins-workspace')).toBe(true);
    await user.click(document.querySelector('.jx-dialog-overlay')!);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it.each(['dirty', 'processing'] as const)('vetoes %s Escape/outside without losing edits', async (state) => {
    const user = userEvent.setup();
    const blocked = vi.fn();
    render(<Fixture {...{ [state]: true }} onDismissBlocked={blocked} />);
    await user.click(screen.getByRole('button', { name: 'Open form' }));
    await user.type(screen.getByRole('textbox', { name: 'Name' }), 'Unsaved label');
    await user.keyboard('{Escape}');
    expect(blocked).toHaveBeenCalledWith('escape', state);
    await user.click(document.querySelector('.jx-dialog-overlay')!);
    expect(blocked).toHaveBeenCalledWith('outside', state);
    expect((screen.getByRole('textbox', { name: 'Name' }) as HTMLInputElement).value).toBe('Unsaved label');
    expect(screen.getByRole('dialog').getAttribute('aria-busy')).toBe(state === 'processing' ? 'true' : null);
    if (state === 'dirty') {
      await user.click(screen.getByRole('button', { name: 'Close' }));
      expect(blocked).toHaveBeenCalledWith('close', 'dirty');
    } else {
      expect((screen.getByRole('button', { name: 'Close' }) as HTMLButtonElement).disabled).toBe(true);
    }
  });

  it('announces validation, preserves errors, and cancel before submit never writes', async () => {
    const user = userEvent.setup();
    const write = vi.fn();
    render(<Fixture write={write} />);
    await user.click(screen.getByRole('button', { name: 'Open form' }));
    await user.click(screen.getByRole('button', { name: 'Save' }));
    const input = screen.getByRole('textbox', { name: 'Name' });
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(
      input
        .getAttribute('aria-describedby')
        ?.split(' ')
        .map((id) => document.getElementById(id)?.textContent),
    ).toEqual(['Enter a synthetic label', 'Name is required']);
    expect(screen.getByRole('alert').textContent).toBe('Name is required');
    await user.type(input, 'Fixture');
    await user.click(screen.getByRole('button', { name: 'Inside action' }));
    expect(screen.getByRole('alert')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(write).not.toHaveBeenCalled();
  });

  it('does not submit through default buttons; explicit valid submit calls the owner once', async () => {
    const user = userEvent.setup();
    const write = vi.fn();
    render(<Fixture write={write} />);
    await user.click(screen.getByRole('button', { name: 'Open form' }));
    await user.type(screen.getByRole('textbox', { name: 'Name' }), 'Fixture');
    await user.click(screen.getByRole('button', { name: 'Inside action' }));
    expect(write).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(write).toHaveBeenCalledTimes(1);
  });

  it('makes processing submit unavailable and does not perform writes', async () => {
    const user = userEvent.setup();
    const write = vi.fn();
    render(<Fixture processing write={write} />);
    await user.click(screen.getByRole('button', { name: 'Open form' }));
    const save = screen.getByRole('button', { name: 'Save' });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    expect(save.getAttribute('aria-busy')).toBe('true');
    await user.click(save);
    expect(write).not.toHaveBeenCalled();
  });
});
