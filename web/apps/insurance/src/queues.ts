export const queues: [string, string][] = [['new', 'Новые'], ['review', 'На рассмотрении'], ['needs-info', 'Ждут продавца'], ['done', 'Решённые']];
export const inQueue = (queue: string, status: string) =>
  queue === 'new' ? status === 'submitted' : queue === 'done' ? status === 'approved' || status === 'declined' : status === queue;
