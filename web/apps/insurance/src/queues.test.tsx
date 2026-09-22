import { describe, expect, it } from 'vitest';
import { inQueue } from './queues';

describe('insurance queues', () => {
  it('groups statuses', () => {
    expect(inQueue('new', 'submitted')).toBe(true);
    expect(inQueue('done', 'declined')).toBe(true);
    expect(inQueue('review', 'needs-info')).toBe(false);
  });
});
