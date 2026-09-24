import type { ComponentPropsWithRef } from 'react';
import '@justixauto/tokens/tokens.css';

export interface ButtonProps extends ComponentPropsWithRef<'button'> {
  variant?: 'primary' | 'secondary' | 'danger';
  processing?: boolean;
}

/** A native button: defaulting to button prevents accidental form writes. */
export function Button({
  variant = 'secondary',
  processing = false,
  disabled,
  type = 'button',
  style,
  children,
  ...props
}: ButtonProps) {
  return (
    <>
      <style>{`.jx-button { height:38px;padding:0 14px;display:inline-flex;align-items:center;justify-content:center;gap:8px;border:1px solid transparent;border-radius:var(--radius);font:inherit;font-weight:600;cursor:pointer;white-space:nowrap }
      .jx-button:disabled { cursor:not-allowed;opacity:.55 }
      .jx-button[data-variant=primary] { color:#fff;background:var(--primary) }
      .jx-button[data-variant=primary]:hover:not(:disabled) { background:var(--primary-hover) }
      .jx-button[data-variant=secondary] { color:var(--text);background:var(--surface);border-color:var(--border-strong) }
      .jx-button[data-variant=secondary]:hover:not(:disabled) { background:var(--surface-subtle) }
      .jx-button[data-variant=danger] { color:#fff;background:var(--danger) }
      .jx-button:focus-visible { outline:var(--field-focus-outline);outline-offset:2px }`}</style>
      <button
        {...props}
        className={['jx-button', props.className].filter(Boolean).join(' ')}
        type={type}
        data-variant={variant}
        disabled={disabled || processing}
        aria-busy={processing || undefined}
        style={style}
      >
        {children}
      </button>
    </>
  );
}
