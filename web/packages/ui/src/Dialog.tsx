import * as RadixDialog from '@radix-ui/react-dialog';
import type { ReactElement, ReactNode } from 'react';
import '@justixauto/tokens/tokens.css';

export type DialogScope = 'dealer-shell' | 'finance-workspace' | 'ins-workspace' | 'admin-shell';
export type DismissReason = 'escape' | 'outside' | 'close';

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** A native, ref-forwarding trigger enables Radix focus restoration. */
  trigger: ReactElement;
  scope: DialogScope;
  title: ReactNode;
  description: ReactNode;
  closeLabel: string;
  children: ReactNode;
  footer?: ReactNode;
  icon?: ReactNode;
  size?: 'default' | 'compact' | 'wide';
  dirty?: boolean;
  processing?: boolean;
  /** Dirty dismissal is vetoed; the page owns any approved discard confirmation. */
  onDismissBlocked?: (reason: DismissReason, state: 'dirty' | 'processing') => void;
}

const styles = `
  .jx-dialog-overlay { position:fixed;inset:0;z-index:var(--dialog-overlay-z-index);display:grid;grid-template-columns:minmax(0,1fr);place-items:center;padding:var(--dialog-overlay-padding);background:var(--dialog-backdrop);box-sizing:border-box;font-family:var(--font-family);font-size:var(--font-size);line-height:var(--line-height);color:var(--text) }
  .jx-dialog { box-sizing:border-box;width:var(--dialog-width);max-width:100%;max-height:var(--dialog-max-height);overflow:auto;background:var(--surface);border:1px solid var(--border);border-radius:var(--dialog-radius);box-shadow:var(--shadow-overlay) }
  .jx-dialog[data-size=compact] { width:var(--dialog-compact-width) }
  .jx-dialog[data-size=wide] { width:var(--dialog-wide-width) }
  .jx-dialog-header { display:grid;grid-template-columns:1fr 36px;gap:12px;align-items:start;padding:var(--dialog-header-padding) }
  .jx-dialog-header[data-icon=true] { grid-template-columns:40px 1fr 36px }
  .jx-dialog-icon { width:40px;height:40px;display:grid;place-items:center;border-radius:7px;color:var(--primary);background:var(--primary-soft) }
  .jx-dialog-title { margin:0;font-size:18px;line-height:24px;overflow-wrap:anywhere }
  .jx-dialog-description { margin:3px 0 0;color:var(--text-secondary);font-size:13px;overflow-wrap:anywhere }
  .jx-dialog-close { width:36px;height:36px;display:grid;place-items:center;border:0;border-radius:var(--radius);background:none;color:inherit;font:inherit;cursor:pointer }
  .jx-dialog-close:hover:not(:disabled) { background:var(--surface-subtle) }
  .jx-dialog-close:disabled { cursor:not-allowed;opacity:.55 }
  .jx-dialog-close:focus-visible { outline:var(--field-focus-outline) }
  .jx-dialog-body { padding:var(--dialog-body-padding) }
  .jx-dialog-footer { display:flex;justify-content:flex-end;gap:10px;padding:var(--dialog-footer-padding);border-top:1px solid var(--border) }
  .ins-workspace .jx-dialog, .admin-shell .jx-dialog { border-color:var(--shell-border) }
  .ins-workspace .jx-dialog-header, .admin-shell .jx-dialog-header { align-items:center;border-bottom:1px solid var(--shell-border) }
  .ins-workspace .jx-dialog-title, .admin-shell .jx-dialog-title { font-size:23px;line-height:normal }
  .ins-workspace .jx-dialog-footer, .admin-shell .jx-dialog-footer { position:sticky;bottom:0;background:var(--surface);gap:12px;border-color:var(--shell-border) }
  @media(max-width:600px) { .ins-workspace .jx-dialog-footer { flex-wrap:wrap } }
`;

/** Modal presentation only. Processing never implies that a server command can be cancelled. */
export function Dialog({ open, onOpenChange, trigger, scope, title, description,
  closeLabel, children, footer, icon, size = 'default', dirty = false,
  processing = false, onDismissBlocked }: DialogProps) {
  function requestDismiss(reason: DismissReason) {
    if (processing || dirty) {
      onDismissBlocked?.(reason, processing ? 'processing' : 'dirty');
      return;
    }
    onOpenChange(false);
  }

  return <RadixDialog.Root open={open} onOpenChange={(next) => {
    if (next) onOpenChange(true);
    else requestDismiss('close');
  }}>
    <RadixDialog.Trigger asChild>{trigger}</RadixDialog.Trigger>
    <RadixDialog.Portal>
      <style>{styles}</style>
      <RadixDialog.Overlay className={`jx-dialog-overlay ${scope}`}>
        <RadixDialog.Content className="jx-dialog" data-size={size} aria-busy={processing || undefined}
          onEscapeKeyDown={(event) => { event.preventDefault(); requestDismiss('escape'); }}
          onPointerDownOutside={(event) => { event.preventDefault(); requestDismiss('outside'); }}
          onInteractOutside={(event) => event.preventDefault()}>
          <header className="jx-dialog-header" data-icon={!!icon}>
            {icon && <div className="jx-dialog-icon" aria-hidden="true">{icon}</div>}
            <div><RadixDialog.Title className="jx-dialog-title">{title}</RadixDialog.Title>
              <RadixDialog.Description className="jx-dialog-description">{description}</RadixDialog.Description></div>
            <button type="button" className="jx-dialog-close" aria-label={closeLabel} disabled={processing}
              onClick={() => requestDismiss('close')}>
              {scope === 'dealer-shell' || scope === 'finance-workspace'
                ? <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" /></svg>
                : <span aria-hidden="true">×</span>}
            </button>
          </header>
          <div className="jx-dialog-body">{children}</div>
          {footer && <footer className="jx-dialog-footer">{footer}</footer>}
        </RadixDialog.Content>
      </RadixDialog.Overlay>
    </RadixDialog.Portal>
  </RadixDialog.Root>;
}
