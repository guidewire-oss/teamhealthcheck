'use client';

import { CheckCircle, Clock, ArrowRight, PauseCircle } from 'lucide-react';
import { ActionItem, ActionItemStatus } from '@/lib/api/action-items';
import ActionItemDescription from './ActionItemDescription';
import ActionItemDueDate from './ActionItemDueDate';

interface PodActionBoardProps {
  items: ActionItem[];
}

const COLUMNS: { status: ActionItemStatus; label: string; icon: React.ReactNode; bg: string; border: string }[] = [
  { status: 'open', label: 'Open', icon: <Clock className="w-4 h-4 text-gray-500" />, bg: 'bg-gray-50', border: 'border-gray-200' },
  { status: 'on_hold', label: 'On Hold', icon: <PauseCircle className="w-4 h-4 text-rose-400" />, bg: 'bg-rose-50', border: 'border-rose-200' },
  { status: 'in_progress', label: 'In Progress', icon: <ArrowRight className="w-4 h-4 text-blue-500" />, bg: 'bg-blue-50', border: 'border-blue-200' },
  { status: 'done', label: 'Done', icon: <CheckCircle className="w-4 h-4 text-green-500" />, bg: 'bg-green-50', border: 'border-green-200' },
];

/**
 * Read-only four-status action board for Manager/Director/VP pod drill-down.
 * No create, status-transition, edit, or delete controls are rendered here.
 */
export default function PodActionBoard({ items }: PodActionBoardProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4" data-testid="pod-action-board">
      {COLUMNS.map(({ status, label, icon, bg, border }) => {
        const colItems = items.filter((i) => i.status === status);
        return (
          <div key={status} className={`rounded-xl border ${border} ${bg} flex flex-col`}>
            <div className={`flex items-center gap-2 px-4 py-3 border-b ${border}`}>
              {icon}
              <span className="text-sm font-semibold text-gray-700">{label}</span>
              <span className="ml-auto text-xs font-medium text-gray-400 bg-white border border-gray-200 rounded-full px-2 py-0.5">
                {colItems.length}
              </span>
            </div>
            <div className="flex flex-col gap-2 p-3 flex-1">
              {colItems.length === 0 && (
                <p className="text-xs text-gray-300 text-center py-4">No items</p>
              )}
              {colItems.map((item) => (
                <div
                  key={item.id}
                  className="bg-white rounded-lg border border-gray-200 p-4 shadow-sm"
                  data-testid="pod-action-item-card"
                >
                  {item.dimensionName && (
                    <span className="inline-block mb-1.5 text-xs font-medium px-2 py-0.5 rounded-full bg-indigo-50 text-indigo-700 border border-indigo-100">
                      {item.dimensionName}
                    </span>
                  )}
                  <p className="text-sm font-medium text-gray-900 leading-snug">{item.title}</p>
                  {item.description && <ActionItemDescription description={item.description} />}
                  <div className="flex items-center gap-2 mt-2 flex-wrap">
                    {item.assigneeName && (
                      <span className="text-xs text-gray-400 break-words min-w-0">→ {item.assigneeName}</span>
                    )}
                    <ActionItemDueDate dueDate={item.dueDate} status={item.status} />
                  </div>
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}
