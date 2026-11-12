import { CheckCircle, Clock, ArrowRight, PauseCircle } from 'lucide-react';
import { ActionItemStatus } from '@/lib/api/action-items';

export interface ActionItemColumn {
  status: ActionItemStatus;
  label: string;
  icon: React.ReactNode;
  bg: string;
  border: string;
}

/** Board columns shared by the Team Lead board and the read-only pod board. */
export const ACTION_ITEM_COLUMNS: ActionItemColumn[] = [
  { status: 'open', label: 'Open', icon: <Clock className="w-4 h-4 text-gray-500" />, bg: 'bg-gray-50', border: 'border-gray-200' },
  { status: 'on_hold', label: 'On Hold', icon: <PauseCircle className="w-4 h-4 text-rose-400" />, bg: 'bg-rose-50', border: 'border-rose-200' },
  { status: 'in_progress', label: 'In Progress', icon: <ArrowRight className="w-4 h-4 text-blue-500" />, bg: 'bg-blue-50', border: 'border-blue-200' },
  { status: 'done', label: 'Done', icon: <CheckCircle className="w-4 h-4 text-green-500" />, bg: 'bg-green-50', border: 'border-green-200' },
];
