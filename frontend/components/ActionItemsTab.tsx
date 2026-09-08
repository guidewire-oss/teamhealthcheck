'use client';

import { useState, useEffect, useCallback } from 'react';
import { Plus, Loader2, AlertCircle, CheckCircle, Clock, ArrowRight, Trash2 } from 'lucide-react';
import { ActionItem, listActionItems, updateActionItem, deleteActionItem } from '@/lib/api/action-items';
import ActionItemModal from './ActionItemModal';

interface TeamMember {
  id: string;
  name: string;
}

interface ActionItemsTabProps {
  teamId: string;
  assessmentPeriod: string;
  defaultDimensionId?: string; // worst-performing dimension to pre-fill
  teamMembers: TeamMember[];
  canEdit: boolean; // Team Lead and above
}

const COLUMNS: { status: ActionItem['status']; label: string; icon: React.ReactNode; bg: string; border: string }[] = [
  {
    status: 'open',
    label: 'Open',
    icon: <Clock className="w-4 h-4 text-gray-500" />,
    bg: 'bg-gray-50',
    border: 'border-gray-200',
  },
  {
    status: 'in_progress',
    label: 'In Progress',
    icon: <ArrowRight className="w-4 h-4 text-blue-500" />,
    bg: 'bg-blue-50',
    border: 'border-blue-200',
  },
  {
    status: 'done',
    label: 'Done',
    icon: <CheckCircle className="w-4 h-4 text-green-500" />,
    bg: 'bg-green-50',
    border: 'border-green-200',
  },
];

const NEXT_STATUS: Record<ActionItem['status'], ActionItem['status'] | null> = {
  open: 'in_progress',
  in_progress: 'done',
  done: null,
};

const STATUS_BTN_LABEL: Record<ActionItem['status'], string | null> = {
  open: 'Start',
  in_progress: 'Mark done',
  done: null,
};

function isOverdue(dueDate: string | null): boolean {
  if (!dueDate) return false;
  // Parse YYYY-MM-DD as a local date (not UTC) to avoid off-by-one in non-UTC time zones.
  const [year, month, day] = dueDate.split('-').map(Number);
  const due = new Date(year, month - 1, day);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return due < today;
}

export default function ActionItemsTab({
  teamId,
  assessmentPeriod,
  defaultDimensionId,
  teamMembers,
  canEdit,
}: ActionItemsTabProps) {
  const [items, setItems] = useState<ActionItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  const loadItems = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await listActionItems(teamId);
      setItems(data);
    } catch {
      setError('Failed to load action items');
    } finally {
      setLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    loadItems();
  }, [loadItems]);

  const handleAdvance = async (item: ActionItem) => {
    const next = NEXT_STATUS[item.status];
    if (!next) return;
    setUpdatingId(item.id);
    try {
      await updateActionItem(teamId, item.id, { status: next });
      setItems((prev) => prev.map((i) => (i.id === item.id ? { ...i, status: next } : i)));
    } catch {
      // silently show old state; user can retry
    } finally {
      setUpdatingId(null);
    }
  };

  const handleDelete = async (item: ActionItem) => {
    if (!confirm(`Delete "${item.title}"?`)) return;
    setUpdatingId(item.id);
    try {
      await deleteActionItem(teamId, item.id);
      setItems((prev) => prev.filter((i) => i.id !== item.id));
    } catch {
      setError('Failed to delete action item');
    } finally {
      setUpdatingId(null);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16" data-testid="action-items-loading">
        <Loader2 className="w-6 h-6 animate-spin text-indigo-600" />
        <span className="ml-2 text-gray-500 text-sm">Loading action items…</span>
      </div>
    );
  }

  const openCount = items.filter((i) => i.status === 'open').length;
  const inProgressCount = items.filter((i) => i.status === 'in_progress').length;

  return (
    <div data-testid="action-items-tab">
      {/* Header row */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">Action Items</h2>
          {items.length > 0 && (
            <p className="text-xs text-gray-400 mt-0.5">
              {openCount + inProgressCount} active · {items.filter((i) => i.status === 'done').length} done
            </p>
          )}
        </div>
        {canEdit && (
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-1.5 px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
            data-testid="add-action-item-btn"
          >
            <Plus className="w-4 h-4" />
            Add action
          </button>
        )}
      </div>

      {error && (
        <div className="mb-4 flex items-center gap-2 text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          {error}
        </div>
      )}

      {items.length === 0 ? (
        <div className="text-center py-16 text-gray-400" data-testid="action-items-empty">
          <CheckCircle className="w-10 h-10 mx-auto mb-3 text-gray-200" />
          <p className="font-medium text-gray-500">No action items yet</p>
          {canEdit && (
            <p className="text-sm mt-1">
              After reviewing the dashboard, click{' '}
              <button
                onClick={() => setShowModal(true)}
                className="text-indigo-600 font-medium hover:underline"
              >
                + Add action
              </button>{' '}
              to track improvements.
            </p>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {COLUMNS.map(({ status, label, icon, bg, border }) => {
            const colItems = items.filter((i) => i.status === status);
            return (
              <div key={status} className={`rounded-xl border ${border} ${bg} flex flex-col`}>
                {/* Column header */}
                <div className={`flex items-center gap-2 px-4 py-3 border-b ${border}`}>
                  {icon}
                  <span className="text-sm font-semibold text-gray-700">{label}</span>
                  <span className="ml-auto text-xs font-medium text-gray-400 bg-white border border-gray-200 rounded-full px-2 py-0.5">
                    {colItems.length}
                  </span>
                </div>

                {/* Cards */}
                <div className="flex flex-col gap-2 p-3 flex-1">
                  {colItems.length === 0 && (
                    <p className="text-xs text-gray-300 text-center py-4">No items</p>
                  )}
                  {colItems.map((item) => (
                    <div
                      key={item.id}
                      className="bg-white rounded-lg border border-gray-200 p-3 shadow-sm"
                      data-testid="action-item-card"
                    >
                      {/* Dimension badge */}
                      {item.dimensionName && (
                        <span className="inline-block mb-1.5 text-xs font-medium px-2 py-0.5 rounded-full bg-indigo-50 text-indigo-700 border border-indigo-100">
                          {item.dimensionName}
                        </span>
                      )}

                      {/* Title */}
                      <p className="text-sm font-medium text-gray-900 leading-snug">{item.title}</p>

                      {/* Description */}
                      {item.description && (
                        <p className="text-xs text-gray-500 mt-1 line-clamp-2">{item.description}</p>
                      )}

                      {/* Meta row */}
                      <div className="flex items-center gap-2 mt-2 flex-wrap">
                        {item.assigneeName && (
                          <span className="text-xs text-gray-400">→ {item.assigneeName}</span>
                        )}
                        {item.dueDate && (
                          <span
                            className={`text-xs font-medium ${
                              isOverdue(item.dueDate) && item.status !== 'done'
                                ? 'text-red-600'
                                : 'text-gray-400'
                            }`}
                          >
                            Due {(() => { const [y, m, d] = item.dueDate.split('-').map(Number); return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }); })()}
                            {isOverdue(item.dueDate) && item.status !== 'done' && ' ⚠'}
                          </span>
                        )}
                      </div>

                      {/* Actions */}
                      {canEdit && (
                        <div className="flex items-center gap-2 mt-2.5 pt-2 border-t border-gray-100">
                          {STATUS_BTN_LABEL[item.status] && (
                            <button
                              onClick={() => handleAdvance(item)}
                              disabled={updatingId === item.id}
                              className="flex items-center gap-1 text-xs font-medium text-indigo-600 hover:text-indigo-800 disabled:opacity-50"
                              data-testid={`advance-action-${item.id}`}
                            >
                              {updatingId === item.id
                                ? <Loader2 className="w-3 h-3 animate-spin" />
                                : <ArrowRight className="w-3 h-3" />
                              }
                              {STATUS_BTN_LABEL[item.status]}
                            </button>
                          )}
                          <button
                            onClick={() => handleDelete(item)}
                            disabled={updatingId === item.id}
                            className="ml-auto text-gray-300 hover:text-red-400 disabled:opacity-50"
                            aria-label="Delete"
                            data-testid={`delete-action-${item.id}`}
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Create modal */}
      {showModal && (
        <ActionItemModal
          teamId={teamId}
          assessmentPeriod={assessmentPeriod}
          defaultDimensionId={defaultDimensionId}
          teamMembers={teamMembers}
          onSaved={() => {
            setShowModal(false);
            loadItems();
          }}
          onClose={() => setShowModal(false)}
        />
      )}
    </div>
  );
}
