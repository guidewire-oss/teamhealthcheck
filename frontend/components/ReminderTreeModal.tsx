'use client';

import { useEffect, useRef } from 'react';
import { X, Send } from 'lucide-react';
import { getStatusesInTree, type ReminderPlan, type ReminderTreeNode } from '@/lib/reminder-tree';
import { STATUS_DOT_CLASS, STATUS_LABEL } from '@/lib/status-colors';
import type { SurveyStatus } from '@/lib/api/survey-completion';

interface ReminderTreeModalProps {
  plan: ReminderPlan;
  onCancel: () => void;
  onSend: () => void;
}

const TITLE_ID = 'reminder-tree-title';

function getInitials(name: string): string {
  return name
    .split(' ')
    .map((part) => part[0])
    .join('')
    .toUpperCase();
}

/**
 * One node in the vertical "git graph" style reminder tree: a circular
 * avatar/dot on the connecting line, plus a label to its right. Children (if
 * any) render indented underneath, still threaded onto the same line.
 */
function TreeNode({ node, depth }: { node: ReminderTreeNode; depth: number }) {
  if (node.type === 'person') {
    const isRoot = depth === 0;
    return (
      <li className="relative pl-9">
        <span
          className={`absolute left-0 top-0.5 w-7 h-7 rounded-full flex items-center justify-center text-[11px] font-bold flex-shrink-0 ${
            isRoot ? 'bg-indigo-600 text-white' : 'bg-white text-indigo-700 border-2 border-indigo-400'
          }`}
        >
          {getInitials(node.name)}
        </span>
        <div className="flex items-center gap-2 min-h-7">
          <span className="text-sm font-semibold text-gray-900">{node.name}</span>
          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-wide bg-gray-100 text-gray-600">
            {node.level}
          </span>
        </div>
        {node.children.length > 0 && (
          <ul className="mt-3 ml-3.5 pl-5 border-l-2 border-indigo-200 space-y-3">
            {node.children.map((child) => (
              <TreeNode key={`${child.type}:${child.id}`} node={child} depth={depth + 1} />
            ))}
          </ul>
        )}
      </li>
    );
  }

  return (
    <li className="relative pl-9" data-testid="reminder-tree-team-node">
      <span
        className={`absolute left-1.5 top-1.5 w-3.5 h-3.5 rounded-full flex-shrink-0 ${STATUS_DOT_CLASS[node.status]}`}
      />
      <span className="text-sm text-gray-800">
        {node.name} <span className="text-gray-400">&mdash;</span> TL: {node.teamLeadName}
      </span>
    </li>
  );
}

/**
 * Explains the tree's status dots. Only lists the statuses that actually
 * appear in this particular tree, using the exact same background classes
 * as the status dots (and the main table's status chips) so a color always
 * means the same thing everywhere.
 */
function Legend({ statuses, align }: { statuses: SurveyStatus[]; align: 'left' | 'right' }) {
  if (statuses.length === 0) return null;

  return (
    <div data-testid="reminder-tree-legend">
      <div className={`text-[10px] font-bold uppercase tracking-wide text-gray-400 mb-1.5 ${align === 'right' ? 'text-right' : 'text-left'}`}>
        Legend
      </div>
      <div className={`flex flex-wrap gap-x-3 gap-y-1 ${align === 'right' ? 'justify-end' : 'justify-start'}`}>
        {statuses.map((status) => (
          <span key={status} className="flex items-center gap-1.5 text-xs text-gray-600 whitespace-nowrap">
            <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${STATUS_DOT_CLASS[status]}`} />
            {STATUS_LABEL[status]}
          </span>
        ))}
      </div>
    </div>
  );
}

export default function ReminderTreeModal({ plan, onCancel, onSend }: ReminderTreeModalProps) {
  const statuses = getStatusesInTree(plan.root);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel();
    };
    document.addEventListener('keydown', handleKeyDown);

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    // Move focus into the dialog on open, and restore it to whatever
    // triggered it (a Remind button) on close -- otherwise a keyboard/screen
    // reader user is left with focus on a control that's still "inside" a
    // now-invisible page, or dropped back to the top of the document.
    previouslyFocused.current = document.activeElement as HTMLElement | null;
    closeButtonRef.current?.focus();

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = previousOverflow;
      previouslyFocused.current?.focus();
    };
  }, [onCancel]);

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div
        className="bg-white rounded-xl shadow-xl max-w-lg w-full max-h-[85vh] overflow-y-auto"
        data-testid="reminder-tree-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={TITLE_ID}
      >
        <div className="p-6 border-b">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h2 id={TITLE_ID} className="text-lg font-semibold text-gray-900">
                Who will get this reminder?
              </h2>
              <p className="text-sm text-gray-500 mt-1">{plan.subtitle}</p>
              <div className="sm:hidden mt-3">
                <Legend statuses={statuses} align="left" />
              </div>
            </div>
            <div className="flex items-start gap-3 flex-shrink-0">
              <div className="hidden sm:block">
                <Legend statuses={statuses} align="right" />
              </div>
              <button
                ref={closeButtonRef}
                type="button"
                onClick={onCancel}
                className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-indigo-500 rounded"
                data-testid="reminder-tree-close"
                aria-label="Close"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
          </div>
        </div>

        <div className="p-6">
          <ul className="space-y-3" data-testid="reminder-tree">
            <TreeNode node={plan.root} depth={0} />
          </ul>
        </div>

        <div className="flex items-center justify-end gap-3 px-6 py-4 border-t bg-gray-50 rounded-b-xl">
          <button
            type="button"
            onClick={onCancel}
            data-testid="reminder-tree-cancel"
            className="px-4 py-2 text-sm font-semibold text-gray-700 border border-gray-300 rounded-lg hover:bg-gray-100 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onSend}
            data-testid="reminder-tree-send"
            className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors text-sm font-semibold focus:outline-none focus:ring-2 focus:ring-indigo-500"
          >
            <Send className="w-4 h-4" />
            Send reminder
          </button>
        </div>
      </div>
    </div>
  );
}
