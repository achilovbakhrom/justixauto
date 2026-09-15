export type ResponseJSONNumeric = 'finite-json' | 'safe-integers';

export interface ResponseJSONLimits {
  readonly maxResponseBytes: number;
  readonly maxJSONDepth: number;
}

export const defaultResponseJSONLimits: Readonly<ResponseJSONLimits> = Object.freeze({
  maxResponseBytes: 8 * 1024 * 1024,
  maxJSONDepth: 128,
});

/** Capture trusted construction settings once; response headers never set limits. */
export function responseJSONLimits(
  limits: Partial<ResponseJSONLimits> = {},
): Readonly<ResponseJSONLimits> {
  const maxResponseBytes = limits.maxResponseBytes === undefined
    ? defaultResponseJSONLimits.maxResponseBytes : limits.maxResponseBytes;
  const maxJSONDepth = limits.maxJSONDepth === undefined
    ? defaultResponseJSONLimits.maxJSONDepth : limits.maxJSONDepth;
  if (!Number.isSafeInteger(maxResponseBytes) || maxResponseBytes <= 0
    || !Number.isSafeInteger(maxJSONDepth) || maxJSONDepth <= 0) {
    throw new TypeError('Invalid response JSON limits');
  }
  return Object.freeze({ maxResponseBytes, maxJSONDepth });
}

const invalid = (): never => { throw new SyntaxError('Invalid response JSON'); };
const numberToken = /-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/y;

function safeInteger(token: string): number {
  const negative = token[0] === '-';
  const unsigned = negative ? token.slice(1) : token;
  const exponentAt = unsigned.search(/[eE]/);
  const mantissa = exponentAt < 0 ? unsigned : unsigned.slice(0, exponentAt);
  const dot = mantissa.indexOf('.');
  const fractionLength = dot < 0 ? 0 : mantissa.length - dot - 1;
  const digits = mantissa.replace('.', '').replace(/^0+/, '');
  // Even an enormous exponent cannot change an exact zero.
  if (digits.length === 0) return negative ? -0 : 0;
  let exponent = 0;
  if (exponentAt >= 0) {
    const raw = unsigned.slice(exponentAt + 1);
    const start = raw[0] === '-' || raw[0] === '+' ? 1 : 0;
    // Beyond this bound no fractional/trailing digits can cancel the exponent.
    // Saturation avoids BigInt/powers or allocations proportional to its value.
    const bound = token.length + 17;
    for (let index = start; index < raw.length; index++) {
      exponent = Math.min(bound, exponent * 10 + raw.charCodeAt(index) - 48);
    }
    if (raw[0] === '-') exponent = -exponent;
  }
  let end = digits.length;
  while (digits[end - 1] === '0') end--;
  const shift = exponent - fractionLength + digits.length - end;
  if (shift < 0 || end + shift > 16) return invalid();
  const integer = digits.slice(0, end) + '0'.repeat(shift);
  if (integer.length === 16 && integer > '9007199254740991') return invalid();
  const value = Number(integer);
  return negative ? -value : value;
}

type Frame =
  | { kind: 'array'; value: unknown[]; state: 'first' | 'next' | 'after' }
  | { kind: 'object'; value: Record<string, unknown>; keys: Set<string>; state: 'first' | 'next' | 'after' };

/** Strict raw-byte JSON. Throws payload-free errors; never returns partial data. */
export function decodeResponseJSON(
  bytes: Uint8Array,
  numeric: ResponseJSONNumeric,
  limits: Partial<ResponseJSONLimits> = defaultResponseJSONLimits,
): unknown {
  const { maxResponseBytes, maxJSONDepth } = responseJSONLimits(limits);
  if (numeric !== 'finite-json' && numeric !== 'safe-integers') {
    throw new TypeError('Invalid response JSON numeric profile');
  }
  if (bytes.byteLength > maxResponseBytes) return invalid();
  let source: string;
  try {
    // ignoreBOM=true preserves U+FEFF, which the JSON grammar then rejects.
    source = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
  } catch {
    return invalid();
  }
  let position = 0;
  const frames: Frame[] = [];
  const whitespace = () => {
    while (source[position] === ' ' || source[position] === '\t'
      || source[position] === '\n' || source[position] === '\r') position++;
  };
  const string = (): string => {
    if (source[position++] !== '"') return invalid();
    const parts: string[] = [];
    let start = position;
    let closed = false;
    while (position < source.length) {
      const character = source[position++]!;
      if (character === '"') {
        parts.push(source.slice(start, position - 1));
        closed = true;
        break;
      }
      if (character.charCodeAt(0) < 32) return invalid();
      if (character !== '\\') continue;
      parts.push(source.slice(start, position - 1));
      const escape = source[position++];
      if (escape === 'u') {
        const hex = source.slice(position, position + 4);
        if (!/^[0-9a-fA-F]{4}$/.test(hex)) return invalid();
        parts.push(String.fromCharCode(Number.parseInt(hex, 16)));
        position += 4;
      } else {
        switch (escape) {
          case '"': case '\\': case '/': parts.push(escape); break;
          case 'b': parts.push('\b'); break;
          case 'f': parts.push('\f'); break;
          case 'n': parts.push('\n'); break;
          case 'r': parts.push('\r'); break;
          case 't': parts.push('\t'); break;
          default: return invalid();
        }
      }
      start = position;
    }
    if (!closed) return invalid();
    const decoded = parts.join('');
    for (let index = 0; index < decoded.length; index++) {
      const unit = decoded.charCodeAt(index);
      if (unit >= 0xd800 && unit <= 0xdbff) {
        const low = decoded.charCodeAt(++index);
        if (!(low >= 0xdc00 && low <= 0xdfff)) return invalid();
      } else if (unit >= 0xdc00 && unit <= 0xdfff) return invalid();
    }
    return decoded;
  };
  const value = (): unknown => {
    whitespace();
    const character = source[position];
    if (character === '"') return string();
    if (character === '{' || character === '[') {
      if (frames.length >= maxJSONDepth) return invalid();
      position++;
      const frame: Frame = character === '['
        ? { kind: 'array', value: [], state: 'first' }
        : { kind: 'object', value: Object.create(null) as Record<string, unknown>, keys: new Set(), state: 'first' };
      frames.push(frame);
      return frame.value;
    }
    for (const [literal, result] of [['true', true], ['false', false], ['null', null]] as const) {
      if (source.startsWith(literal, position)) {
        position += literal.length;
        return result;
      }
    }
    numberToken.lastIndex = position;
    const match = numberToken.exec(source);
    if (!match) return invalid();
    position = numberToken.lastIndex;
    if (numeric === 'safe-integers') return safeInteger(match[0]);
    const result = Number(match[0]);
    return Number.isFinite(result) ? result : invalid();
  };
  const result = value();
  // Explicit frames support configured depths without relying on the JS stack.
  while (frames.length > 0) {
    const frame = frames[frames.length - 1]!;
    whitespace();
    const close = frame.kind === 'array' ? ']' : '}';
    if (source[position] === close && frame.state !== 'next') {
      position++;
      frames.pop();
      continue;
    }
    if (frame.state === 'after') {
      if (source[position++] !== ',') return invalid();
      frame.state = 'next';
      continue;
    }
    frame.state = 'after';
    if (frame.kind === 'array') {
      frame.value.push(value());
    } else {
      const key = string();
      if (frame.keys.has(key)) return invalid();
      frame.keys.add(key);
      whitespace();
      if (source[position++] !== ':') return invalid();
      Object.defineProperty(frame.value, key, {
        value: value(), enumerable: true, configurable: true, writable: true,
      });
    }
  }
  whitespace();
  if (position !== source.length) return invalid();
  return result;
}
