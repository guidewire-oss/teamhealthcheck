// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

"use client";

import { useState, useEffect } from "react";
import {
  Plus,
  Edit2,
  Trash2,
  Save,
  X,
  AlertCircle,
  ToggleLeft,
  ToggleRight,
} from "lucide-react";
import {
  getDimensions,
  createDimension,
  updateDimension,
  deleteDimension,
  HealthDimension,
  CreateDimensionRequest,
  UpdateDimensionRequest,
  clearAdminCache,
} from "@/lib/api/admin";

export default function DimensionConfig() {
  const [dimensions, setDimensions] = useState<HealthDimension[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Form state
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [editingDimension, setEditingDimension] = useState<HealthDimension | null>(null);
  const [formData, setFormData] = useState({
    id: "",
    name: "",
    description: "",
    goodDescription: "",
    badDescription: "",
    weight: 1.0,
    isActive: true,
  });
  const [formError, setFormError] = useState<string | null>(null);
  const [formSubmitting, setFormSubmitting] = useState(false);

  // Delete confirmation
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  // Toggle state
  const [togglingId, setTogglingId] = useState<string | null>(null);

  // Load dimensions on mount
  useEffect(() => {
    loadDimensions();
  }, []);

  const loadDimensions = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await getDimensions();
      setDimensions(response.dimensions);
    } catch (err: any) {
      console.error("Failed to load dimensions:", err);
      setError(err.message || "Failed to load dimensions");
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setFormData({
      id: "",
      name: "",
      description: "",
      goodDescription: "",
      badDescription: "",
      weight: 1.0,
      isActive: true,
    });
    setFormError(null);
    setShowCreateModal(false);
    setShowEditModal(false);
    setEditingDimension(null);
  };

  const handleShowCreateModal = () => {
    resetForm();
    setShowCreateModal(true);
  };

  const handleShowEditModal = (dimension: HealthDimension) => {
    setFormData({
      id: dimension.id,
      name: dimension.name,
      description: dimension.description || "",
      goodDescription: dimension.goodDescription,
      badDescription: dimension.badDescription,
      weight: dimension.weight,
      isActive: dimension.isActive,
    });
    setEditingDimension(dimension);
    setShowEditModal(true);
  };

  const generateId = (name: string): string => {
    return name
      .toLowerCase()
      .replace(/[^a-z0-9\s-]/g, "")
      .replace(/\s+/g, "-")
      .replace(/-+/g, "-")
      .trim();
  };

  const handleCreate = async () => {
    setFormError(null);
    setFormSubmitting(true);

    try {
      // Validation
      if (!formData.name.trim()) {
        throw new Error("Name is required");
      }
      if (!formData.goodDescription.trim()) {
        throw new Error("Good description is required");
      }
      if (!formData.badDescription.trim()) {
        throw new Error("Bad description is required");
      }

      const id = formData.id.trim() || generateId(formData.name);

      const request: CreateDimensionRequest = {
        id,
        name: formData.name.trim(),
        description: formData.description.trim() || undefined,
        goodDescription: formData.goodDescription.trim(),
        badDescription: formData.badDescription.trim(),
        isActive: formData.isActive,
        weight: formData.weight,
      };

      await createDimension(request);
      clearAdminCache();
      await loadDimensions();
      resetForm();
    } catch (err: any) {
      console.error("Failed to create dimension:", err);
      setFormError(err.message || "Failed to create dimension");
    } finally {
      setFormSubmitting(false);
    }
  };

  const handleUpdate = async () => {
    if (!editingDimension) return;

    setFormError(null);
    setFormSubmitting(true);

    try {
      // Validation
      if (!formData.name.trim()) {
        throw new Error("Name is required");
      }
      if (!formData.goodDescription.trim()) {
        throw new Error("Good description is required");
      }
      if (!formData.badDescription.trim()) {
        throw new Error("Bad description is required");
      }

      const request: UpdateDimensionRequest = {
        name: formData.name.trim(),
        description: formData.description.trim(),
        goodDescription: formData.goodDescription.trim(),
        badDescription: formData.badDescription.trim(),
        isActive: formData.isActive,
        weight: formData.weight,
      };

      await updateDimension(editingDimension.id, request);
      clearAdminCache();
      await loadDimensions();
      resetForm();
    } catch (err: any) {
      console.error("Failed to update dimension:", err);
      setFormError(err.message || "Failed to update dimension");
    } finally {
      setFormSubmitting(false);
    }
  };

  const handleToggleActive = async (dimension: HealthDimension) => {
    setTogglingId(dimension.id);
    try {
      await updateDimension(dimension.id, { isActive: !dimension.isActive });
      clearAdminCache();
      await loadDimensions();
    } catch (err: any) {
      console.error("Failed to toggle dimension:", err);
      setError(err.message || "Failed to toggle dimension");
    } finally {
      setTogglingId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteConfirmId) return;

    setDeletingId(deleteConfirmId);
    try {
      await deleteDimension(deleteConfirmId);
      clearAdminCache();
      await loadDimensions();
      setDeleteConfirmId(null);
    } catch (err: any) {
      console.error("Failed to delete dimension:", err);
      setError(err.message || "Failed to delete dimension");
    } finally {
      setDeletingId(null);
    }
  };

  if (loading) {
    return (
      <div className="text-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto mb-4"></div>
        <p className="text-gray-500">Loading dimensions...</p>
      </div>
    );
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h3 className="text-lg font-medium text-gray-900">
          Health Dimensions Configuration
        </h3>
        <button
          data-testid="add-dimension-btn"
          onClick={handleShowCreateModal}
          className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
        >
          <Plus className="w-4 h-4" />
          Add Dimension
        </button>
      </div>

      {error && (
        <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3">
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5" />
          <div>
            <p className="font-medium text-red-900">Error</p>
            <p className="text-sm text-red-700">{error}</p>
          </div>
        </div>
      )}

      <div className="space-y-3">
        {dimensions.map((dimension) => (
          <div
            key={dimension.id}
            className={`flex items-center justify-between p-4 border rounded-lg ${
              dimension.isActive ? "bg-white" : "bg-gray-50 opacity-75"
            }`}
            data-testid="dimension-row"
          >
            <div className="flex-1">
              <div className="flex items-center gap-3">
                <p className="font-medium text-gray-900">{dimension.name}</p>
                {!dimension.isActive && (
                  <span className="px-2 py-0.5 text-xs bg-gray-200 text-gray-600 rounded">
                    Inactive
                  </span>
                )}
                <span className="px-2 py-0.5 text-xs bg-indigo-100 text-indigo-700 rounded">
                  Weight: {dimension.weight}
                </span>
              </div>
              <p className="text-sm text-gray-500 mt-1">
                {dimension.description}
              </p>
              <div className="flex gap-4 mt-2 text-xs">
                <span className="text-green-600">
                  Good: {dimension.goodDescription}
                </span>
                <span className="text-red-600">
                  Bad: {dimension.badDescription}
                </span>
              </div>
            </div>
            <div className="flex items-center gap-2 ml-4">
              <button
                data-testid="toggle-dimension-btn"
                onClick={() => handleToggleActive(dimension)}
                disabled={togglingId === dimension.id}
                className={`p-2 rounded-lg transition-colors ${
                  dimension.isActive
                    ? "text-green-600 hover:bg-green-50"
                    : "text-gray-400 hover:bg-gray-100"
                } disabled:opacity-50`}
                title={dimension.isActive ? "Deactivate" : "Activate"}
              >
                {togglingId === dimension.id ? (
                  <div className="w-5 h-5 animate-spin rounded-full border-2 border-current border-t-transparent" />
                ) : dimension.isActive ? (
                  <ToggleRight className="w-5 h-5" />
                ) : (
                  <ToggleLeft className="w-5 h-5" />
                )}
              </button>
              <button
                data-testid="edit-dimension-btn"
                onClick={() => handleShowEditModal(dimension)}
                className="p-2 text-indigo-600 hover:bg-indigo-50 rounded-lg transition-colors"
              >
                <Edit2 className="w-4 h-4" />
              </button>
              <button
                data-testid="delete-dimension-btn"
                onClick={() => setDeleteConfirmId(dimension.id)}
                disabled={deletingId === dimension.id}
                className="p-2 text-red-600 hover:bg-red-50 rounded-lg transition-colors disabled:opacity-50"
              >
                {deletingId === dimension.id ? (
                  <div className="w-4 h-4 animate-spin rounded-full border-2 border-red-600 border-t-transparent" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
              </button>
            </div>
          </div>
        ))}
      </div>

      {dimensions.length === 0 && !loading && (
        <div className="text-center py-8 text-gray-500">
          No dimensions found. Add your first dimension to get started.
        </div>
      )}

      {/* Create Modal */}
      {showCreateModal && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          data-testid="dimension-create-modal"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">
              Add New Dimension
            </h3>

            {formError && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                <p className="text-sm text-red-700">{formError}</p>
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Name *
                </label>
                <input
                  type="text"
                  data-testid="dimension-name-input"
                  value={formData.name}
                  onChange={(e) =>
                    setFormData({ ...formData, name: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  placeholder="e.g., Communication"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  ID (optional, auto-generated from name)
                </label>
                <input
                  type="text"
                  data-testid="dimension-id-input"
                  value={formData.id}
                  onChange={(e) =>
                    setFormData({ ...formData, id: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  placeholder="e.g., communication"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Description
                </label>
                <textarea
                  data-testid="dimension-description-input"
                  value={formData.description}
                  onChange={(e) =>
                    setFormData({ ...formData, description: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                  placeholder="Brief description of what this dimension measures"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Good State Description *
                </label>
                <textarea
                  data-testid="dimension-good-description-input"
                  value={formData.goodDescription}
                  onChange={(e) =>
                    setFormData({ ...formData, goodDescription: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                  placeholder="e.g., We communicate clearly and effectively"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Bad State Description *
                </label>
                <textarea
                  data-testid="dimension-bad-description-input"
                  value={formData.badDescription}
                  onChange={(e) =>
                    setFormData({ ...formData, badDescription: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                  placeholder="e.g., Communication is a constant struggle"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Weight
                  </label>
                  <input
                    type="number"
                    data-testid="dimension-weight-input"
                    value={formData.weight}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        weight: parseFloat(e.target.value) || 1.0,
                      })
                    }
                    min="0"
                    max="10"
                    step="0.1"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  />
                </div>

                <div className="flex items-center">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.isActive}
                      onChange={(e) =>
                        setFormData({ ...formData, isActive: e.target.checked })
                      }
                      className="w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"
                    />
                    <span className="text-sm font-medium text-gray-700">
                      Active
                    </span>
                  </label>
                </div>
              </div>
            </div>

            <div className="flex gap-4 mt-6 justify-end">
              <button
                onClick={resetForm}
                disabled={formSubmitting}
                className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                data-testid="save-dimension-btn"
                onClick={handleCreate}
                disabled={formSubmitting}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50"
              >
                <Save className="w-4 h-4" />
                {formSubmitting ? "Creating..." : "Create Dimension"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {showEditModal && editingDimension && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
          data-testid="dimension-edit-modal"
        >
          <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">
              Edit Dimension
            </h3>

            {formError && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                <p className="text-sm text-red-700">{formError}</p>
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Name *
                </label>
                <input
                  type="text"
                  data-testid="dimension-name-input"
                  value={formData.name}
                  onChange={(e) =>
                    setFormData({ ...formData, name: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Description
                </label>
                <textarea
                  data-testid="dimension-description-input"
                  value={formData.description}
                  onChange={(e) =>
                    setFormData({ ...formData, description: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Good State Description *
                </label>
                <textarea
                  data-testid="dimension-good-description-input"
                  value={formData.goodDescription}
                  onChange={(e) =>
                    setFormData({ ...formData, goodDescription: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Bad State Description *
                </label>
                <textarea
                  data-testid="dimension-bad-description-input"
                  value={formData.badDescription}
                  onChange={(e) =>
                    setFormData({ ...formData, badDescription: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  rows={2}
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Weight
                  </label>
                  <input
                    type="number"
                    data-testid="dimension-weight-input"
                    value={formData.weight}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        weight: parseFloat(e.target.value) || 1.0,
                      })
                    }
                    min="0"
                    max="10"
                    step="0.1"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  />
                </div>

                <div className="flex items-center">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.isActive}
                      onChange={(e) =>
                        setFormData({ ...formData, isActive: e.target.checked })
                      }
                      className="w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"
                    />
                    <span className="text-sm font-medium text-gray-700">
                      Active
                    </span>
                  </label>
                </div>
              </div>
            </div>

            <div className="flex gap-4 mt-6 justify-end">
              <button
                onClick={resetForm}
                disabled={formSubmitting}
                className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                data-testid="save-dimension-btn"
                onClick={handleUpdate}
                disabled={formSubmitting}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50"
              >
                <Save className="w-4 h-4" />
                {formSubmitting ? "Saving..." : "Save Changes"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deleteConfirmId && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-md w-full mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">
              Deactivate Dimension
            </h3>
            <p className="text-gray-600 mb-4">
              Are you sure you want to deactivate the dimension{" "}
              <strong>
                {dimensions.find((d) => d.id === deleteConfirmId)?.name}
              </strong>
              ?
            </p>
            <p className="text-sm text-gray-500 mb-6">
              The dimension will be hidden from new surveys but historical data will be preserved. You can reactivate it later using the toggle button.
            </p>
            <div className="flex gap-4 justify-end">
              <button
                onClick={() => setDeleteConfirmId(null)}
                className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors"
              >
                Cancel
              </button>
              <button
                data-testid="confirm-delete-btn"
                onClick={handleDelete}
                disabled={deletingId !== null}
                className="px-4 py-2 bg-amber-600 text-white rounded-lg hover:bg-amber-700 transition-colors disabled:opacity-50"
              >
                {deletingId ? "Deactivating..." : "Deactivate"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
