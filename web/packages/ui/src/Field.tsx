import { useId } from 'react';
import type { ComponentPropsWithRef, ReactNode } from 'react';
import '@justixauto/tokens/tokens.css';

export interface FieldProps extends Omit<ComponentPropsWithRef<'input'>, 'children'> {
  label: ReactNode;
  hint?: string;
  error?: string;
}

/** Validation is supplied by the owning form; no domain rules or persistence. */
export function Field({ label, hint, error, id, ...input }: FieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const describedBy = [input['aria-describedby'], hint && `${fieldId}-hint`, error && `${fieldId}-error`]
    .filter(Boolean).join(' ') || undefined;
  return <div className="jx-field">
    <style>{`.jx-field { display:grid;gap:6px;min-width:0 }
      .jx-field label { color:var(--text-secondary);font-size:12px;font-weight:600 }
      .jx-field input { box-sizing:border-box;width:100%;min-height:40px;padding:9px 11px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);color:var(--text);font:inherit }
      .jx-field input:focus { outline:var(--field-focus-outline);border-color:var(--primary) }
      .jx-field-error { color:var(--danger);font-size:12px }
      .jx-field-hint { color:var(--text-muted);font-size:11px }`}</style>
    <label htmlFor={fieldId}>{label}</label>
    <input {...input} id={fieldId} aria-describedby={describedBy}
      aria-invalid={error ? true : input['aria-invalid']} />
    {hint && <div id={`${fieldId}-hint`} className="jx-field-hint">{hint}</div>}
    {error && <div id={`${fieldId}-error`} className="jx-field-error" role="alert">{error}</div>}
  </div>;
}
