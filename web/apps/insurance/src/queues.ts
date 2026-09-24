/** Status filter of the insurer's applications, in the reference order. */
export const statusFilter: [string, string][] = [
  ['submitted', 'Новая'],
  ['review', 'На рассмотрении'],
  ['needs-info', 'Нужны сведения'],
  ['approved', 'Одобрена'],
  ['declined', 'Отклонена'],
];

export const inQueue = (queue: string, status: string) =>
  queue === 'new'
    ? status === 'submitted'
    : queue === 'done'
      ? status === 'approved' || status === 'declined'
      : status === queue;
