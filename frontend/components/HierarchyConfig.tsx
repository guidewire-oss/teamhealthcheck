'use client';

import { useState, useEffect } from 'react';
import { Plus, Edit2, Trash2, Save, AlertCircle, Eye, Users, FileText, type LucideIcon } from 'lucide-react';
import {
  listHierarchyLevels,
  createHierarchyLevel,
  updateHierarchyLevel,
  deleteHierarchyLevel,
  clearAdminCacheKeys,
  type HierarchyLevel,
  type HierarchyPermissions,
  type CreateHierarchyLevelRequest,
  type UpdateHierarchyLevelRequest,
} from '@/lib/api/admin';

interface LocalPermissions {
  canViewAllTeams: boolean;
  canEditTeams: boolean;
  canManageUsers: boolean;
  canTakeSurvey: boolean;
  canViewAnalytics: boolean;
}

interface EditFormData {
  name: string;
  permissions: LocalPermissions;
}

const EMPTY_PERMISSIONS: LocalPermissions = {
  canViewAllTeams: false,
  canEditTeams: false,
  canManageUsers: false,
  canTakeSurvey: false,
  canViewAnalytics: false,
};

const permissionLabels: Record<keyof LocalPermissions, string> = {
  canViewAllTeams: 'View All Teams',
  canEditTeams: 'Edit Teams',
  canManageUsers: 'Manage Users',
  canTakeSurvey: 'Take Survey',
  canViewAnalytics: 'View Analytics',
};

const permissionIcons: Record<keyof LocalPermissions, LucideIcon> = {
  canViewAllTeams: Eye,
  canEditTeams: Users,
  canManageUsers: Users,
  canTakeSurvey: FileText,
  canViewAnalytics: FileText,
};

const PERMISSION_KEYS = Object.keys(permissionLabels) as (keyof LocalPermissions)[];

export default function HierarchyConfig() {
  const [levels, setLevels] = useState<HierarchyLevel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editingLevel, setEditingLevel] = useState<string | null>(null);
  const [editFormData, setEditFormData] = useState<EditFormData | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
  const [newLevel, setNewLevel] = useState<{ name: string; permissions: LocalPermissions }>({
    name: '',
    permissions: { ...EMPTY_PERMISSIONS },
  });

  useEffect(() => {
    loadHierarchyLevels();
  }, []);

  const loadHierarchyLevels = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await listHierarchyLevels();
      setLevels(data);
    } catch (err) {
      console.error('Failed to load hierarchy levels:', err);
      setError(err instanceof Error ? err.message : 'Failed to load hierarchy levels');
    } finally {
      setLoading(false);
    }
  };

  const mapToBackendPermissions = (local: LocalPermissions): HierarchyPermissions => ({
    canViewAllTeams: local.canViewAllTeams,
    canEditTeams: local.canEditTeams,
    canManageUsers: local.canManageUsers,
    canTakeSurvey: local.canTakeSurvey,
    canViewAnalytics: local.canViewAnalytics,
  });

  const mapFromBackendPermissions = (backend: HierarchyPermissions): LocalPermissions => ({
    canViewAllTeams: backend.canViewAllTeams,
    canEditTeams: backend.canEditTeams,
    canManageUsers: backend.canManageUsers,
    canTakeSurvey: backend.canTakeSurvey,
    canViewAnalytics: backend.canViewAnalytics,
  });

  const handleAddLevel = async () => {
    if (!newLevel.name.trim()) return;

    try {
      setLoading(true);
      setError(null);

      const request: CreateHierarchyLevelRequest = {
        name: newLevel.name.trim(),
        permissions: mapToBackendPermissions(newLevel.permissions),
      };

      await createHierarchyLevel(request);
      clearAdminCacheKeys('hierarchy-levels');
      await loadHierarchyLevels();

      setShowAddForm(false);
      setNewLevel({ name: '', permissions: { ...EMPTY_PERMISSIONS } });
    } catch (err) {
      console.error('Failed to create hierarchy level:', err);
      setError(err instanceof Error ? err.message : 'Failed to create hierarchy level');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateLevel = async (levelId: string) => {
    if (!editFormData) return;

    try {
      setLoading(true);
      setError(null);

      const request: UpdateHierarchyLevelRequest = {
        name: editFormData.name.trim(),
        permissions: mapToBackendPermissions(editFormData.permissions),
      };

      await updateHierarchyLevel(levelId, request);
      clearAdminCacheKeys('hierarchy-levels');
      await loadHierarchyLevels();

      setEditingLevel(null);
      setEditFormData(null);
    } catch (err) {
      console.error('Failed to update hierarchy level:', err);
      setError(err instanceof Error ? err.message : 'Failed to update hierarchy level');
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteLevel = async (levelId: string) => {
    if (!confirm('Are you sure you want to delete this level? This action cannot be undone.')) {
      return;
    }

    try {
      setLoading(true);
      setError(null);

      await deleteHierarchyLevel(levelId);
      clearAdminCacheKeys('hierarchy-levels');
      await loadHierarchyLevels();
    } catch (err) {
      console.error('Failed to delete hierarchy level:', err);
      setError(err instanceof Error ? err.message : 'Failed to delete hierarchy level');
    } finally {
      setLoading(false);
    }
  };

  const startEdit = (level: HierarchyLevel) => {
    setError(null);
    setEditingLevel(level.id);
    setEditFormData({
      name: level.name,
      permissions: mapFromBackendPermissions(level.permissions),
    });
  };

  const cancelAdd = () => {
    setShowAddForm(false);
    setNewLevel({ name: '', permissions: { ...EMPTY_PERMISSIONS } });
  };

  const cancelEdit = () => {
    setEditingLevel(null);
    setEditFormData(null);
  };

  if (loading && levels.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-center">
          <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-indigo-600 mx-auto"></div>
          <p className="mt-4 text-sm text-gray-500">Loading hierarchy levels...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && (
        <div className="p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3">
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 shrink-0" />
          <div>
            <p className="font-medium text-red-900">Error</p>
            <p className="text-sm text-red-700">{error}</p>
          </div>
        </div>
      )}

      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">Hierarchy Configuration</h2>
          <p className="text-sm text-gray-500 mt-0.5">Define organizational levels and permissions</p>
        </div>
        <button
          data-testid="add-level-btn"
          onClick={() => {
            setError(null);
            setShowAddForm(true);
          }}
          disabled={loading}
          className="flex items-center justify-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus className="w-4 h-4" />
          Add Level
        </button>
      </div>

      {levels.length === 0 ? (
        <div className="text-center py-8 text-gray-500" data-testid="hierarchy-empty-state">
          No hierarchy levels yet. Click &ldquo;Add Level&rdquo; to create one.
        </div>
      ) : (
        <div className="bg-white rounded-xl shadow-sm border overflow-hidden" data-testid="hierarchy-list">
          <div className="hidden sm:flex items-center gap-4 px-6 py-3 bg-gray-50 text-xs font-medium text-gray-500 uppercase tracking-wider">
            <div className="w-10 shrink-0">#</div>
            <div className="w-48 shrink-0">Level</div>
            <div className="flex-1">Permissions</div>
            <div className="w-20 shrink-0 text-right">Actions</div>
          </div>

          <div className="divide-y divide-gray-100">
            {levels.map((level) => {
              const perms = mapFromBackendPermissions(level.permissions);
              const enabledKeys = PERMISSION_KEYS.filter((key) => perms[key]);

              return (
                <div
                  key={level.id}
                  data-testid="hierarchy-level-row"
                  className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4 px-4 sm:px-6 py-3 hover:bg-gray-50 transition-colors"
                >
                  <div className="flex items-center gap-3 sm:w-48 shrink-0 min-w-0">
                    <span
                      className="inline-flex items-center justify-center w-7 h-7 rounded-full bg-gray-100 text-gray-600 text-xs font-semibold shrink-0"
                      aria-label={`Position ${level.position}`}
                    >
                      {String(level.position).padStart(2, '0')}
                    </span>
                    <div className="min-w-0">
                      <p className="font-medium text-gray-900 truncate">{level.name}</p>
                      <p className="text-xs text-gray-500 sm:hidden">
                        {enabledKeys.length} permission{enabledKeys.length === 1 ? '' : 's'}
                      </p>
                    </div>
                  </div>

                  <div className="flex-1 min-w-0 flex flex-wrap gap-1.5 pl-10 sm:pl-0">
                    {enabledKeys.length > 0 ? (
                      enabledKeys.map((key) => {
                        const Icon = permissionIcons[key];
                        return (
                          <span
                            key={key}
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 text-xs"
                          >
                            <Icon className="w-3 h-3" />
                            {permissionLabels[key]}
                          </span>
                        );
                      })
                    ) : (
                      <span className="text-xs text-gray-400 italic">No permissions</span>
                    )}
                  </div>

                  <div className="flex items-center gap-1 shrink-0 self-end sm:self-center sm:ml-4">
                    <button
                      data-testid="edit-level-btn"
                      onClick={() => startEdit(level)}
                      disabled={loading}
                      title="Edit level"
                      aria-label={`Edit ${level.name}`}
                      className="p-2 text-indigo-600 hover:bg-indigo-50 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      <Edit2 className="w-4 h-4" />
                    </button>
                    <button
                      data-testid="delete-level-btn"
                      onClick={() => handleDeleteLevel(level.id)}
                      disabled={loading}
                      title="Delete level"
                      aria-label={`Delete ${level.name}`}
                      className="p-2 text-red-600 hover:bg-red-50 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {showAddForm && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          data-testid="create-level-form"
          role="dialog"
          aria-modal="true"
          aria-labelledby="add-level-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h3 id="add-level-modal-title" className="text-lg font-semibold text-gray-900 mb-4">
              Add New Hierarchy Level
            </h3>

            {error && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-600 mt-0.5 shrink-0" />
                <p className="text-sm text-red-700">{error}</p>
              </div>
            )}

            <div>
              <label htmlFor="new-level-name" className="block text-sm font-medium text-gray-700 mb-1">
                Level Name
              </label>
              <input
                id="new-level-name"
                data-testid="level-name-input"
                type="text"
                value={newLevel.name}
                onChange={(e) => setNewLevel({ ...newLevel, name: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="e.g., Senior Director"
                disabled={loading}
                autoFocus
              />
            </div>

            <div className="mt-4">
              <label className="block text-sm font-medium text-gray-700 mb-2">Permissions</label>
              <div className="grid grid-cols-2 gap-3">
                {PERMISSION_KEYS.map((key) => (
                  <label key={key} className="flex items-center gap-2 cursor-pointer">
                    <input
                      data-testid={`permission-${key}`}
                      type="checkbox"
                      checked={newLevel.permissions[key]}
                      onChange={(e) =>
                        setNewLevel({
                          ...newLevel,
                          permissions: { ...newLevel.permissions, [key]: e.target.checked },
                        })
                      }
                      className="w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"
                      disabled={loading}
                    />
                    <span className="text-sm text-gray-700">{permissionLabels[key]}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="flex gap-4 mt-6 justify-end">
              <button
                onClick={cancelAdd}
                disabled={loading}
                className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                data-testid="save-level-btn"
                onClick={handleAddLevel}
                disabled={loading || !newLevel.name.trim()}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Save className="w-4 h-4" />
                {loading ? 'Saving...' : 'Save Level'}
              </button>
            </div>
          </div>
        </div>
      )}

      {editingLevel && editFormData && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          data-testid="edit-level-form"
          role="dialog"
          aria-modal="true"
          aria-labelledby="edit-level-modal-title"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h3 id="edit-level-modal-title" className="text-lg font-semibold text-gray-900 mb-4">
              Edit Hierarchy Level
            </h3>

            {error && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-600 mt-0.5 shrink-0" />
                <p className="text-sm text-red-700">{error}</p>
              </div>
            )}

            <div>
              <label htmlFor="edit-level-name" className="block text-sm font-medium text-gray-700 mb-1">
                Level Name
              </label>
              <input
                id="edit-level-name"
                data-testid="edit-level-name-input"
                type="text"
                value={editFormData.name}
                onChange={(e) => setEditFormData(prev => prev ? { ...prev, name: e.target.value } : null)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                disabled={loading}
                autoFocus
              />
            </div>

            <div className="mt-4">
              <label className="block text-sm font-medium text-gray-700 mb-2">Permissions</label>
              <div className="grid grid-cols-2 gap-3">
                {PERMISSION_KEYS.map((key) => (
                  <label key={key} className="flex items-center gap-2 cursor-pointer">
                    <input
                      data-testid={`edit-permission-${key}`}
                      type="checkbox"
                      checked={editFormData.permissions[key]}
                      onChange={(e) => setEditFormData(prev => prev ? {
                        ...prev,
                        permissions: { ...prev.permissions, [key]: e.target.checked }
                      } : null)}
                      className="w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"
                      disabled={loading}
                    />
                    <span className="text-sm text-gray-700">{permissionLabels[key]}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="flex gap-4 mt-6 justify-end">
              <button
                onClick={cancelEdit}
                disabled={loading}
                className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                data-testid="save-edit-btn"
                onClick={() => handleUpdateLevel(editingLevel)}
                disabled={loading || !editFormData.name.trim()}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Save className="w-4 h-4" />
                {loading ? 'Saving...' : 'Save Changes'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
