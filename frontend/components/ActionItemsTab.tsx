'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Plus, Loader2, AlertCircle, ArrowRight, ArrowLeft, Trash2, Pencil } from 'lucide-react';
import {
  ActionItem,
  ActionItemStatus,
  DirectManager,
  listActionItems,
  updateActionItem,
  deleteActionItem,
  getDirectManager,
} from '@/lib/api/action-items';
import ActionItemModal from './ActionItemModal';
import ActionItemDescription from './ActionItemDescription';
import ActionItemDueDate from './ActionItemDueDate';
import { ACTION_ITEM_COLUMNS } from './actionItemColumns';

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
  currentUserId?: string; // used to gate the creator-only edit icon
}

export default function ActionItemsTab({
  teamId,
  assessmentPeriod,
  defaultDimensionId,
  teamMembers,
  canEdit,
  currentUserId,
}: ActionItemsTabProps) {
  const [items, setItems] = useState<ActionItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Set when the board itself could not be loaded; distinct from `error`, which
  // reports a failed action (status change, delete) on a board that did load.
  const [loadError, setLoadError] = useState(false);
  const [showModal, setShowModal] = useState(false);
  const [editingItem, setEditingItem] = useState<ActionItem | null>(null);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [directManager, setDirectManager] = useState<DirectManager | null>(null);

  // Identifies the latest load. A response from an older request (e.g. the
  // previous team after `teamId` changed) must not touch state.
  const loadSeq = useRef(0);

  const loadItems = useCallback(async () => {
    const seq = ++loadSeq.current;
    const isStale = () => seq !== loadSeq.current;
    try {
      setLoading(true);
      setError(null);
      setLoadError(false);
      const data = await listActionItems(teamId);
      if (isStale()) return;
      setItems(data);
    } catch {
      if (isStale()) return;
      setItems([]);
      setLoadError(true);
    } finally {
      if (!isStale()) setLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    loadItems();
  }, [loadItems]);

  useEffect(() => {
    if (!canEdit) return;
    getDirectManager(teamId).then(setDirectManager).catch(() => setDirectManager(null));
  }, [teamId, canEdit]);

  const transition = async (item: ActionItem, next: ActionItemStatus) => {
    setUpdatingId(item.id);
    setError(null);
    try {
      await updateActionItem(teamId, item.id, { status: next });
      setItems((prev) => prev.map((i) => (i.id === item.id ? { ...i, status: next } : i)));
    } catch {
      // The card keeps its old status; tell the user the move did not happen.
      setError('Failed to update action item status');
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

  // "Active" means every item that is not Done, On Hold included, matching the
  // backend's openCount. On hold is shown as a sub-count so nothing is counted twice.
  const doneCount = items.filter((i) => i.status === 'done').length;
  const onHoldCount = items.filter((i) => i.status === 'on_hold').length;
  const activeCount = items.length - doneCount;

  return (
    <div data-testid="action-items-tab">
      {/* Header row */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">Action Items</h2>
          {items.length > 0 && (
            <p className="text-xs text-gray-400 mt-0.5">
              {activeCount} active{onHoldCount > 0 && ` (${onHoldCount} on hold)`} · {doneCount} done
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

      {loadError && (
        <div
          className="flex items-center gap-3 text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-3"
          role="alert"
          data-testid="action-items-load-error"
        >
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>Failed to load action items</span>
          <button
            onClick={loadItems}
            className="ml-auto font-medium text-indigo-600 hover:text-indigo-800"
            data-testid="action-items-retry"
          >
            Retry
          </button>
        </div>
      )}
      {!loadError && items.length === 0 && (
        <div className="text-center py-4 text-gray-400" data-testid="action-items-empty">
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
      )}
      {!loadError && (
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {ACTION_ITEM_COLUMNS.map(({ status, label, icon, bg, border }) => {
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
                      className="bg-white rounded-lg border border-gray-200 p-4 shadow-sm"
                      data-testid="action-item-card"
                    >
                      {/* Dimension badge + edit icon */}
                      <div className="flex items-start justify-between gap-2">
                        {item.dimensionName ? (
                          <span className="inline-block mb-1.5 text-xs font-medium px-2 py-0.5 rounded-full bg-indigo-50 text-indigo-700 border border-indigo-100">
                            {item.dimensionName}
                          </span>
                        ) : <span />}
                        {canEdit && currentUserId && item.createdBy === currentUserId && item.status !== 'done' && (
                          <button
                            onClick={() => setEditingItem(item)}
                            className="text-gray-300 hover:text-indigo-500 flex-shrink-0"
                            aria-label="Edit action item"
                            data-testid={`edit-action-${item.id}`}
                          >
                            <Pencil className="w-3.5 h-3.5" />
                          </button>
                        )}
                      </div>

                      {/* Title */}
                      <p className="text-sm font-medium text-gray-900 leading-snug">{item.title}</p>

                      {/* Description */}
                      {item.description && <ActionItemDescription description={item.description} />}

                      {/* Meta row */}
                      <div className="flex items-center gap-2 mt-2 flex-wrap">
                        {item.assigneeName && (
                          <span className="text-xs text-gray-400 break-words min-w-0">→ {item.assigneeName}</span>
                        )}
                        <ActionItemDueDate dueDate={item.dueDate} status={item.status} />
                      </div>

                      {/* Actions */}
                      {canEdit && (
                        <div className="flex items-center gap-2 mt-2.5 pt-2 border-t border-gray-100">
                          {item.status === 'open' && (
                            <button
                              onClick={() => transition(item, 'in_progress')}
                              disabled={updatingId === item.id}
                              className="flex items-center gap-1 text-xs font-medium text-indigo-600 hover:text-indigo-800 disabled:opacity-50"
                              data-testid={`advance-action-${item.id}`}
                            >
                              {updatingId === item.id
                                ? <Loader2 className="w-3 h-3 animate-spin" />
                                : <ArrowRight className="w-3 h-3" />
                              }
                              Start
                            </button>
                          )}
                          {item.status === 'on_hold' && (
                            <button
                              onClick={() => transition(item, 'in_progress')}
                              disabled={updatingId === item.id}
                              className="flex items-center gap-1 text-xs font-medium text-indigo-600 hover:text-indigo-800 disabled:opacity-50"
                              data-testid={`advance-action-${item.id}`}
                            >
                              {updatingId === item.id
                                ? <Loader2 className="w-3 h-3 animate-spin" />
                                : <ArrowRight className="w-3 h-3" />
                              }
                              Resume
                            </button>
                          )}
                          {item.status === 'in_progress' && (
                            <>
                              <button
                                onClick={() => transition(item, 'on_hold')}
                                disabled={updatingId === item.id}
                                className="flex items-center gap-1 text-xs font-medium text-rose-500 hover:text-rose-700 disabled:opacity-50"
                                data-testid={`hold-action-${item.id}`}
                              >
                                <ArrowLeft className="w-3 h-3" />
                                Move to Hold
                              </button>
                              <button
                                onClick={() => transition(item, 'done')}
                                disabled={updatingId === item.id}
                                className="ml-auto flex items-center gap-1 text-xs font-medium text-indigo-600 hover:text-indigo-800 disabled:opacity-50"
                                data-testid={`advance-action-${item.id}`}
                              >
                                {updatingId === item.id
                                  ? <Loader2 className="w-3 h-3 animate-spin" />
                                  : <ArrowRight className="w-3 h-3" />
                                }
                                Mark Done
                              </button>
                            </>
                          )}
                          <button
                            onClick={() => handleDelete(item)}
                            disabled={updatingId === item.id}
                            className={item.status === 'in_progress' ? 'text-gray-300 hover:text-red-400 disabled:opacity-50' : 'ml-auto text-gray-300 hover:text-red-400 disabled:opacity-50'}
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
          directManager={directManager}
          onSaved={() => {
            setShowModal(false);
            loadItems();
          }}
          onClose={() => setShowModal(false)}
        />
      )}

      {/* Edit modal */}
      {editingItem && (
        <ActionItemModal
          teamId={teamId}
          assessmentPeriod={assessmentPeriod}
          teamMembers={teamMembers}
          directManager={directManager}
          editItem={editingItem}
          onSaved={() => {
            setEditingItem(null);
            loadItems();
          }}
          onClose={() => setEditingItem(null)}
        />
      )}
    </div>
  );
}
