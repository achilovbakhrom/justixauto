import { describe, expect, it } from 'vitest';
import { decodeResponseJSON, defaultResponseJSONLimits, responseJSONLimits } from './responseJSON';
import type { ResponseJSONLimits, ResponseJSONNumeric } from './responseJSON';

const bytes = (source: string) => new TextEncoder().encode(source);
const decode = (source: string, numeric: ResponseJSONNumeric = 'safe-integers') =>
  decodeResponseJSON(bytes(source), numeric);

describe('strict raw response JSON', () => {
  it.each([
    'null',
    'true',
    'false',
    '""',
    '"text"',
    '[]',
    '{}',
    '[{},[],null,true,false]',
    '{"a":1,"b":["text",{"c":false}]}',
    ' \t\r\n { "a" : [ 1 , 2 ] } \t\r\n',
    '"\\"\\\\\\/\\b\\f\\n\\r\\t"',
    '"\\u0000\\u001f\\u007f"',
    '"😀\\ud83d\\ude00�"',
    '{"é":1,"é":2,"X":3,"x":4}',
  ])('decodes valid grammar: %s', (source) => {
    expect(decode(source)).toEqual(JSON.parse(source));
  });

  it.each([
    '',
    ' ',
    '\ufeff{}',
    '\u00a0{}',
    '{}\u00a0',
    'undefined',
    'NaN',
    'Infinity',
    '-Infinity',
    '+1',
    '01',
    '-01',
    '.1',
    '1.',
    '1e',
    '1e+',
    '1e-',
    '0x1',
    'true false',
    '{}[]',
    'nullx',
    '1 2',
    '[1,]',
    '{"a":1,}',
    '[,1]',
    '{,}',
    '[1 2]',
    '{"a" 1}',
    '{a:1}',
    "{'a':1}",
    '[}',
    '{]',
    '[[',
    '{"a":',
    '"unterminated',
    '"\\"',
    '"\\x41"',
    '"\\u000"',
    '"\\uQQQQ"',
    '"line\nfeed"',
    '"\u0000"',
    '"\\ud800"',
    '"\\udfff"',
    '"\\ud800x"',
    '"\\ud800\\ud800"',
    '"\\udc00\\ud800"',
    '{"\\ud800":1}',
    '{"x":1,"x":2}',
    '{"x":1,"\\u0078":2}',
    '{"a":{"x":1,"x":2}}',
    '[{"x":1,"\\u0078":2}]',
    '{"😀":1,"\\ud83d\\ude00":2}',
    '{"__proto__":1,"__proto__":2}',
  ])('rejects malformed grammar or lost identity: %s', (source) => {
    for (const profile of ['safe-integers', 'finite-json'] as const) {
      expect(() => decode(source, profile)).toThrow('Invalid response JSON');
    }
  });

  it.each([
    [0xc0, 0xaf],
    [0xc1, 0xbf],
    [0xe0, 0x80, 0xaf],
    [0xed, 0xa0, 0x80],
    [0xf4, 0x90, 0x80, 0x80],
    [0xf5, 0x80, 0x80, 0x80],
    [0x80],
    [0xc2],
    [0xe2, 0x82],
    [0xf0, 0x9f, 0x98],
    [0xff],
  ])('rejects malformed UTF-8 bytes %j', (...invalidBytes) => {
    for (const profile of ['safe-integers', 'finite-json'] as const) {
      expect(() => decodeResponseJSON(Uint8Array.from([34, ...invalidBytes, 34]), profile)).toThrow(SyntaxError);
      expect(() => decodeResponseJSON(Uint8Array.from([123, 34, ...invalidBytes, 34, 58, 49, 125]), profile)).toThrow(
        SyntaxError,
      );
    }
  });

  it('creates only own data properties, without altering prototypes', () => {
    const result = decode('{"__proto__":{"polluted":true},"constructor":1,"toString":2,"hasOwnProperty":3}') as Record<
      string,
      unknown
    >;
    expect(Object.getPrototypeOf(result)).toBeNull();
    expect(Object.hasOwn(result, '__proto__')).toBe(true);
    expect(Object.getOwnPropertyDescriptor(result, '__proto__')).toMatchObject({
      value: { polluted: true },
      enumerable: true,
      writable: true,
      configurable: true,
    });
    expect(Object.getOwnPropertyDescriptor(result, '__proto__')?.get).toBeUndefined();
    expect(Object.hasOwn({}, 'polluted')).toBe(false);
    expect(result.constructor).toBe(1);
    expect(result.toString).toBe(2);
  });

  it('does not confuse equal names in separate objects or normalize Unicode', () => {
    expect(decode('[{"x":1},{"x":2}]')).toEqual([{ x: 1 }, { x: 2 }]);
    expect(Object.keys(decode('{"é":1,"é":2}') as object)).toEqual(['é', 'é']);
  });

  it('does not coerce schema strings or scalar types', () => {
    expect(decode('["9007199254740992","1.25",true,null]')).toEqual(['9007199254740992', '1.25', true, null]);
  });

  it('does not include body content in an error', () => {
    try {
      decode('{"oneTimeSecret":"private-value",}');
      expect.fail('Malformed body accepted');
    } catch (error) {
      expect(error).toBeInstanceOf(SyntaxError);
      expect(String(error)).toBe('SyntaxError: Invalid response JSON');
    }
  });
});

describe('exact safe integer lexemes', () => {
  it.each([
    ['0', 0],
    ['-0', -0],
    ['1.0', 1],
    ['1e0', 1],
    ['10e-1', 1],
    ['1000.00e-3', 1],
    ['0.001e3', 1],
    ['-0.000e-999999999999999999', -0],
    ['0e999999999999999999999999999', 0],
    ['1e+000000000000000000000000000', 1],
    ['9007199254740991', Number.MAX_SAFE_INTEGER],
    ['-9007199254740991', Number.MIN_SAFE_INTEGER],
    ['90071992547409910e-1', Number.MAX_SAFE_INTEGER],
    ['9.007199254740991e15', Number.MAX_SAFE_INTEGER],
    ['1.00000000000000000', 1],
    ['123000e-3', 123],
  ] as const)('accepts exact integer %s', (source, expected) => {
    expect(Object.is(decode(source), expected)).toBe(true);
  });

  it.each([
    '0.1',
    '-0.1',
    '1.0000000000000001',
    '9007199254740990.9',
    '9007199254740991.1',
    '9007199254740992',
    '-9007199254740992',
    '90071992547409910',
    '90071992547409910e-2',
    '1e-324',
    '-1e-9999999',
    '1e9999999999999999999999',
    '1e-9999999999999999999999',
    '999999999999999999999999999999999999',
  ])('rejects before rounding %s', (source) => {
    expect(() => decode(source)).toThrow(SyntaxError);
  });

  it('bounds enormous exponent work while allowing exact zero and cancellation', () => {
    const huge = '9'.repeat(100_000);
    expect(decode(`0e${huge}`)).toBe(0);
    expect(Object.is(decode(`-0e-${huge}`), -0)).toBe(true);
    expect(() => decode(`1e${huge}`)).toThrow(SyntaxError);
    expect(() => decode(`1e-${huge}`)).toThrow(SyntaxError);
    expect(decode(`1${'0'.repeat(100_000)}e-100000`)).toBe(1);
    expect(decode(`0.${'0'.repeat(100_000)}1e100001`)).toBe(1);
  });

  it('agrees with an independent exact rational oracle across signed decimal/exponent cases', () => {
    for (let coefficient = -117; coefficient <= 117; coefficient += 3) {
      for (let fraction = 0; fraction <= 4; fraction++) {
        for (let exponent = -5; exponent <= 5; exponent++) {
          const absolute = String(Math.abs(coefficient)).padStart(fraction + 1, '0');
          const mantissa = fraction ? `${absolute.slice(0, -fraction)}.${absolute.slice(-fraction)}` : absolute;
          const source = `${coefficient < 0 ? '-' : ''}${mantissa}e${exponent}`;
          const shift = exponent - fraction;
          const numerator = BigInt(coefficient) * (shift > 0 ? 10n ** BigInt(shift) : 1n);
          const denominator = shift < 0 ? 10n ** BigInt(-shift) : 1n;
          if (numerator % denominator === 0n) {
            expect(decode(source), source).toBe(Number(numerator / denominator));
          } else {
            expect(() => decode(source), source).toThrow(SyntaxError);
          }
        }
      }
    }
  });

  it.each(['0.125', '-1.75', '1.0000000000000001', '1e-324', '9007199254740992'])(
    'preserves finite legacy Number behavior for %s',
    (source) => {
      expect(Object.is(decode(source, 'finite-json'), JSON.parse(source))).toBe(true);
    },
  );

  it.each(['1e309', '-1e99999'])('rejects nonfinite legacy overflow %s', (source) => {
    expect(() => decode(source, 'finite-json')).toThrow(SyntaxError);
  });
});

describe('resource limits', () => {
  it('freezes defaults and captures a separate immutable caller configuration', () => {
    expect(defaultResponseJSONLimits).toEqual({ maxResponseBytes: 8 * 1024 * 1024, maxJSONDepth: 128 });
    expect(Object.isFrozen(defaultResponseJSONLimits)).toBe(true);
    const caller = { maxResponseBytes: 64, maxJSONDepth: 2 };
    const captured = responseJSONLimits(caller);
    caller.maxResponseBytes = 1;
    caller.maxJSONDepth = 1;
    expect(captured).toEqual({ maxResponseBytes: 64, maxJSONDepth: 2 });
    expect(Object.isFrozen(captured)).toBe(true);
  });

  it.each([0, -1, 0.5, Number.NaN, Number.POSITIVE_INFINITY, Number.MAX_SAFE_INTEGER + 1, null, '2'])(
    'rejects invalid construction limit %s',
    (bad) => {
      for (const field of ['maxResponseBytes', 'maxJSONDepth'] as const) {
        const limits = { [field]: bad } as Partial<ResponseJSONLimits>;
        expect(() => responseJSONLimits(limits)).toThrow(TypeError);
        expect(() => decodeResponseJSON(bytes('0'), 'safe-integers', limits)).toThrow(TypeError);
      }
    },
  );

  it('checks bytes, including UTF-8 width and the exact boundary, before decoding', () => {
    expect(decodeResponseJSON(bytes('"😀"'), 'safe-integers', { maxResponseBytes: 6 })).toBe('😀');
    expect(() => decodeResponseJSON(bytes('"😀"'), 'safe-integers', { maxResponseBytes: 5 })).toThrow(SyntaxError);
    const source = bytes(`"${'x'.repeat(defaultResponseJSONLimits.maxResponseBytes - 2)}"`);
    expect((decodeResponseJSON(source, 'safe-integers') as string).length).toBe(source.length - 2);
    expect(() => decodeResponseJSON(new Uint8Array(source.length + 1), 'safe-integers')).toThrow(SyntaxError);
  });

  it('counts container nesting rather than strings, siblings or scalar values', () => {
    expect(decodeResponseJSON(bytes('[1,{"x":[]}]'), 'safe-integers', { maxJSONDepth: 3 })).toEqual([1, { x: [] }]);
    expect(() => decodeResponseJSON(bytes('[1,{"x":[]}]'), 'safe-integers', { maxJSONDepth: 2 })).toThrow(SyntaxError);
    expect(decodeResponseJSON(bytes('[1,2,3,"[[["]'), 'safe-integers', { maxJSONDepth: 1 })).toEqual([1, 2, 3, '[[[']);
    expect(() => decode('['.repeat(128) + '0' + ']'.repeat(128))).not.toThrow();
    expect(() => decode('['.repeat(129) + '0' + ']'.repeat(129))).toThrow(SyntaxError);
  });

  it('uses explicit frames for a larger approved depth without JS stack overflow', () => {
    const depth = 20_000;
    expect(() =>
      decodeResponseJSON(bytes('['.repeat(depth) + '0' + ']'.repeat(depth)), 'safe-integers', { maxJSONDepth: depth }),
    ).not.toThrow();
  });

  it('rejects an unknown numeric profile', () => {
    expect(() => decode('0', 'rounded' as ResponseJSONNumeric)).toThrow(TypeError);
  });
});
