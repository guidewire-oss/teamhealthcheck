import { ListTodo } from 'lucide-react';
import { TeamActionSummary } from '@/lib/api/action-items';

interface ManagerActionSummaryProps {
  summary: TeamActionSummary[];
  loading: boolean;
  error: boolean;
  onSelectTeam: (teamId: string, teamName: string) => void;
}

const CELL = 'px-6 py-4 whitespace-nowrap text-sm text-gray-700';
const HEAD = 'px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider';

/**
 * Per-team action item counts for the Manager/Director/VP "Actions" tab.
 * Distinguishes loading, failed, empty and populated states so a failed
 * request is never shown as "no action items" or as zero counts.
 */
export default function ManagerActionSummary({ summary, loading, error, onSelectTeam }: ManagerActionSummaryProps) {
  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-12 text-red-600" role="alert" data-testid="manager-actions-error">
        <p className="font-medium">Couldn&apos;t load action items</p>
        <p className="text-sm mt-1 text-gray-500">Please refresh the page or try again in a moment.</p>
      </div>
    );
  }

  if (summary.length === 0) {
    return (
      <div className="text-center py-12 text-gray-400">
        <ListTodo className="w-10 h-10 mx-auto mb-3 text-gray-200" />
        <p className="font-medium text-gray-500">No action items found</p>
        <p className="text-sm mt-1">Team Leads create action items on their team dashboards.</p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-gray-200">
        <thead className="bg-gray-50">
          <tr>
            <th className={HEAD}>Team</th>
            <th className={HEAD}>Open</th>
            <th className={HEAD}>On Hold</th>
            <th className={HEAD}>In Progress</th>
            <th className={HEAD}>Completed</th>
          </tr>
        </thead>
        <tbody className="bg-white divide-y divide-gray-200">
          {summary.map((row) => {
            // A row without counts is unavailable, not zero.
            const counts = row.statusCounts;
            return (
              <tr
                key={row.teamId}
                data-testid="manager-action-row"
                onClick={() => onSelectTeam(row.teamId, row.teamName)}
                className="cursor-pointer hover:bg-gray-50"
              >
                <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-indigo-600 hover:underline">{row.teamName}</td>
                <td className={CELL}>{counts ? counts.open : '—'}</td>
                <td className={CELL}>{counts ? counts.onHold : '—'}</td>
                <td className={CELL}>{counts ? counts.inProgress : '—'}</td>
                <td className={CELL}>{counts ? counts.completed : '—'}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
