import { ActionItemStatus } from '@/lib/api/action-items';
import { formatDueDateLong, isDueDateOverdue } from '@/lib/date-format';

interface ActionItemDueDateProps {
  dueDate: string | null;
  status: ActionItemStatus;
}

/**
 * Shared due-date renderer for both the Team Lead board and the read-only
 * Manager/Director/VP pod board, so the overdue predicate and its red/⚠
 * treatment can't drift between the two (previously the pod board didn't
 * apply overdue styling at all).
 */
export default function ActionItemDueDate({ dueDate, status }: ActionItemDueDateProps) {
  const formatted = formatDueDateLong(dueDate);
  if (!formatted) return null;

  const overdue = isDueDateOverdue(dueDate) && status !== 'done';

  return (
    <span
      className={`text-xs font-medium whitespace-nowrap ${overdue ? 'text-red-600' : 'text-gray-400'}`}
      data-testid="action-item-due-date-text"
    >
      Due {formatted}
      {overdue && ' ⚠'}
    </span>
  );
}
