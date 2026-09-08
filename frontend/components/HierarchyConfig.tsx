'use client';

import { useState, useEffect } from 'react';
import { Plus, Edit2, Trash2, Save, X, ChevronUp, ChevronDown, Shield, Eye, Users, FileText, Download, Settings } from 'lucide-react';
import {
  listHierarchyLevels,
  createHierarchyLevel,
  updateHierarchyLevel,
  updateHierarchyPosition,
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
  canConfigureSystem: boolean;
  canViewReports: boolean;
  canExportData: boolean;
  canTakeSurvey: boolean;
  canViewAnalytics: boolean;
}

interface EditFormData {
  name: string;
  color: string;
  permissions: LocalPermissions;
}

export default function HierarchyConfig() {
  const [levels, setLevels] = useState<HierarchyLevel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editingLevel, setEditingLevel] = useState<string | null>(null);
  const [editFormData, setEditFormData] = useState<EditFormData | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
  const [newLevel, setNewLevel] = useState<{
    name: string;
    color: string;
    permissions: LocalPermissions;
  }>({
    name: '',
    color: '#6366F1',
    permissions: {
      canViewAllTeams: false,
      canEditTeams: false,
      canManageUsers: false,
      canConfigureSystem: false,
      canViewReports: false,
      canExportData: false,
      canTakeSurvey: false,
      canViewAnalytics: false,
    }
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
    // Frontend-only permissions (not in backend yet)
    canConfigureSystem: false,
    canViewReports: false,
    canExportData: false,
  });

  const handleAddLevel = async () => {
    if (!newLevel.name.trim()) return;

    try {
      setLoading(true);
      setError(null);

      const request: CreateHierarchyLevelRequest = {
        name: newLevel.name.trim(),
        position: levels.length + 1,
        permissions: mapToBackendPermissions(newLevel.permissions),
      };

      await createHierarchyLevel(request);
      clearAdminCacheKeys('hierarchy-levels');
      await loadHierarchyLevels();

      setShowAddForm(false);
      setNewLevel({
        name: '',
        color: '#6366F1',
        permissions: {
          canViewAllTeams: false,
          canEditTeams: false,
          canManageUsers: false,
          canConfigureSystem: false,
          canViewReports: false,
          canExportData: false,
          canTakeSurvey: false,
          canViewAnalytics: false,
        }
      });
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

  const moveLevel = async (index: number, direction: 'up' | 'down') => {
    const level = levels[index];
    let newPosition = level.position;

    if (direction === 'up' && index > 0) {
      newPosition = levels[index - 1].position;
    } else if (direction === 'down' && index < levels.length - 1) {
      newPosition = levels[index + 1].position;
    } else {
      return; // Can't move
    }

    try {
      setLoading(true);
      setError(null);

      await updateHierarchyPosition(level.id, { position: newPosition });
      clearAdminCacheKeys('hierarchy-levels');
      await loadHierarchyLevels();
    } catch (err) {
      console.error('Failed to reorder hierarchy level:', err);
      setError(err instanceof Error ? err.message : 'Failed to reorder hierarchy level');
    } finally {
      setLoading(false);
    }
  };

  const startEdit = (level: HierarchyLevel) => {
    setEditingLevel(level.id);
    setEditFormData({
      name: level.name,
      color: '#6366F1', // Backend doesn't support color yet
      permissions: mapFromBackendPermissions(level.permissions),
    });
  };

  const PermissionIcon = ({ permission, label }: { permission: keyof LocalPermissions, label: string }) => {
    const icons: Record<keyof LocalPermissions, typeof Shield> = {
      canViewAllTeams: Eye,
      canEditTeams: Users,
      canManageUsers: Users,
      canConfigureSystem: Settings,
      canViewReports: FileText,
      canExportData: Download,
      canTakeSurvey: FileText,
      canViewAnalytics: FileText,
    };
    const Icon = icons[permission] || Shield;

    return (
      <div className="flex items-center gap-2 text-sm">
        <Icon className="w-4 h-4" />
        <span>{label}</span>
      </div>
    );
  };

  const permissionLabels: Record<keyof LocalPermissions, string> = {
    canViewAllTeams: 'View All Teams',
    canEditTeams: 'Edit Teams',
    canManageUsers: 'Manage Users',
    canConfigureSystem: 'Configure System',
    canViewReports: 'View Reports',
    canExportData: 'Export Data',
    canTakeSurvey: 'Take Survey',
    canViewAnalytics: 'View Analytics',
  };

  if (loading && levels.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto"></div>
          <p className="mt-4 text-gray-600">Loading hierarchy levels...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4">
          <div className="flex items-center gap-2 text-red-800">
            <X className="w-5 h-5" />
            <span className="font-medium">Error: {error}</span>
          </div>
        </div>
      )}

      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Hierarchy Configuration</h2>
          <p className="text-gray-500 mt-1">Define organizational levels and permissions</p>
        </div>
        <button
          data-testid="add-level-btn"
          onClick={() => setShowAddForm(true)}
          disabled={loading}
          className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus className="w-5 h-5" />
          Add Level
        </button>
      </div>

      {showAddForm && (
        <div className="bg-white p-6 rounded-xl shadow-sm border" data-testid="create-level-form">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Add New Hierarchy Level</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Level Name</label>
              <input
                data-testid="level-name-input"
                type="text"
                value={newLevel.name}
                onChange={(e) => setNewLevel({ ...newLevel, name: e.target.value })}
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500"
                placeholder="e.g., Senior Director"
                disabled={loading}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Color</label>
              <div className="flex gap-2">
                <input
                  type="color"
                  value={newLevel.color}
                  onChange={(e) => setNewLevel({ ...newLevel, color: e.target.value })}
                  className="w-20 h-10 border border-gray-300 rounded cursor-pointer"
                  disabled={loading}
                />
                <input
                  type="text"
                  value={newLevel.color}
                  onChange={(e) => setNewLevel({ ...newLevel, color: e.target.value })}
                  className="flex-1 px-4 py-2 border border-gray-300 rounded-lg"
                  disabled={loading}
                />
              </div>
            </div>
          </div>

          <div className="mt-4">
            <label className="block text-sm font-medium text-gray-700 mb-2">Permissions</label>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
              {(Object.entries(permissionLabels) as [keyof LocalPermissions, string][]).map(([key, label]) => (
                <label key={key} className="flex items-center gap-2">
                  <input
                    data-testid={`permission-${key}`}
                    type="checkbox"
                    checked={newLevel.permissions[key]}
                    onChange={(e) => setNewLevel({
                      ...newLevel,
                      permissions: {
                        ...newLevel.permissions,
                        [key]: e.target.checked
                      }
                    })}
                    className="w-4 h-4 text-indigo-600 rounded"
                    disabled={loading}
                  />
                  <span className="text-sm">{label}</span>
                </label>
              ))}
            </div>
          </div>

          <div className="flex gap-4 mt-6">
            <button
              data-testid="save-level-btn"
              onClick={handleAddLevel}
              disabled={loading || !newLevel.name.trim()}
              className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Save className="w-4 h-4" />
              {loading ? 'Saving...' : 'Save Level'}
            </button>
            <button
              onClick={() => {
                setShowAddForm(false);
                setNewLevel({
                  name: '',
                  color: '#6366F1',
                  permissions: {
                    canViewAllTeams: false,
                    canEditTeams: false,
                    canManageUsers: false,
                    canConfigureSystem: false,
                    canViewReports: false,
                    canExportData: false,
                    canTakeSurvey: false,
                    canViewAnalytics: false,
                  }
                });
              }}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <X className="w-4 h-4" />
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="space-y-4" data-testid="hierarchy-list">
        {levels.map((level, index) => (
          <div key={level.id} className="bg-white p-6 rounded-xl shadow-sm border" data-testid="hierarchy-level-row">
            {editingLevel === level.id ? (
              <div className="space-y-4" data-testid="edit-level-form">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">Level Name</label>
                    <input
                      data-testid="edit-level-name-input"
                      type="text"
                      value={editFormData?.name || ''}
                      onChange={(e) => setEditFormData(prev => prev ? { ...prev, name: e.target.value } : null)}
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500"
                      disabled={loading}
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">Color</label>
                    <div className="flex gap-2">
                      <input
                        type="color"
                        value={editFormData?.color || '#6366F1'}
                        onChange={(e) => setEditFormData(prev => prev ? { ...prev, color: e.target.value } : null)}
                        className="w-20 h-10 border border-gray-300 rounded cursor-pointer"
                        disabled={loading}
                      />
                      <input
                        type="text"
                        value={editFormData?.color || '#6366F1'}
                        onChange={(e) => setEditFormData(prev => prev ? { ...prev, color: e.target.value } : null)}
                        className="flex-1 px-4 py-2 border border-gray-300 rounded-lg"
                        disabled={loading}
                      />
                    </div>
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Permissions</label>
                  <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                    {(Object.entries(permissionLabels) as [keyof LocalPermissions, string][]).map(([key, label]) => (
                      <label key={key} className="flex items-center gap-2">
                        <input
                          data-testid={`edit-permission-${key}`}
                          type="checkbox"
                          checked={editFormData?.permissions[key] || false}
                          onChange={(e) => setEditFormData(prev => prev ? {
                            ...prev,
                            permissions: { ...prev.permissions, [key]: e.target.checked }
                          } : null)}
                          className="w-4 h-4 text-indigo-600 rounded"
                          disabled={loading}
                        />
                        <span className="text-sm">{label}</span>
                      </label>
                    ))}
                  </div>
                </div>

                <div className="flex gap-4">
                  <button
                    data-testid="save-edit-btn"
                    onClick={() => handleUpdateLevel(level.id)}
                    disabled={loading || !editFormData?.name.trim()}
                    className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Save className="w-4 h-4" />
                    {loading ? 'Saving...' : 'Save Changes'}
                  </button>
                  <button
                    onClick={() => {
                      setEditingLevel(null);
                      setEditFormData(null);
                    }}
                    disabled={loading}
                    className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <X className="w-4 h-4" />
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex justify-between items-start">
                <div className="flex-1">
                  <div className="flex items-center gap-4 mb-2">
                    <div
                      className="w-4 h-4 rounded bg-indigo-600"
                    />
                    <h3 className="text-lg font-semibold text-gray-900">
                      Position {level.position}: {level.name}
                    </h3>
                  </div>

                  <div className="grid grid-cols-2 md:grid-cols-3 gap-3 mt-4">
                    {(Object.entries(mapFromBackendPermissions(level.permissions)) as [keyof LocalPermissions, boolean][])
                      .filter(([_, enabled]) => enabled)
                      .map(([key]) => (
                        <PermissionIcon
                          key={key}
                          permission={key}
                          label={permissionLabels[key]}
                        />
                      ))}
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <button
                    data-testid="move-up-btn"
                    onClick={() => moveLevel(index, 'up')}
                    disabled={index === 0 || loading}
                    aria-label="Move level up"
                    className={`p-2 rounded-lg transition-colors ${
                      index === 0 || loading
                        ? 'text-gray-300 cursor-not-allowed'
                        : 'text-gray-600 hover:bg-gray-100'
                    }`}
                  >
                    <ChevronUp className="w-5 h-5" />
                  </button>
                  <button
                    data-testid="move-down-btn"
                    onClick={() => moveLevel(index, 'down')}
                    disabled={index === levels.length - 1 || loading}
                    aria-label="Move level down"
                    className={`p-2 rounded-lg transition-colors ${
                      index === levels.length - 1 || loading
                        ? 'text-gray-300 cursor-not-allowed'
                        : 'text-gray-600 hover:bg-gray-100'
                    }`}
                  >
                    <ChevronDown className="w-5 h-5" />
                  </button>
                  <button
                    data-testid="edit-level-btn"
                    onClick={() => startEdit(level)}
                    disabled={loading}
                    className="p-2 text-indigo-600 hover:bg-indigo-50 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Edit2 className="w-5 h-5" />
                  </button>
                  <button
                    data-testid="delete-level-btn"
                    onClick={() => handleDeleteLevel(level.id)}
                    disabled={loading}
                    className="p-2 text-red-600 hover:bg-red-50 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Trash2 className="w-5 h-5" />
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}