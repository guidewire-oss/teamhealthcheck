import { authenticatedFetch } from '@/lib/auth';
import { API_BASE_URL } from './client';

export interface ActionItem {
  id: string;
  teamId: string;
  dimensionId: string | null;
  dimensionName: string | null;
  createdBy: string;
  createdByName: string;
  assignedTo: string | null;
  assigneeName: string | null;
  title: string;
  description: string;
  status: 'open' | 'in_progress' | 'done';
  dueDate: string | null;
  assessmentPeriod: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateActionItemPayload {
  dimensionId?: string;
  assignedTo?: string;
  title: string;
  description?: string;
  dueDate?: string;
  assessmentPeriod?: string;
}

export interface UpdateActionItemPayload {
  status?: 'open' | 'in_progress' | 'done';
  title?: string;
  description?: string;
  dimensionId?: string;
  assignedTo?: string;
  dueDate?: string;
  assessmentPeriod?: string;
}

export async function listActionItems(teamId: string, status?: string): Promise<ActionItem[]> {
  const params = status ? `?status=${encodeURIComponent(status)}` : '';
  const res = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/action-items${params}`);
  if (!res.ok) throw new Error('Failed to fetch action items');
  const data = await res.json();
  return data.actionItems ?? [];
}

export async function createActionItem(teamId: string, payload: CreateActionItemPayload): Promise<{ id: string }> {
  const res = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/action-items`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error('Failed to create action item');
  return res.json();
}

export async function updateActionItem(teamId: string, id: string, payload: UpdateActionItemPayload): Promise<void> {
  const res = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/action-items/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error('Failed to update action item');
}

export async function deleteActionItem(teamId: string, id: string): Promise<void> {
  const res = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/action-items/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete action item');
}

export interface TeamActionSummary {
  teamId: string;
  teamName: string;
  openCount: number;
}

export async function listManagerTeamsActionSummary(managerId: string): Promise<TeamActionSummary[]> {
  const res = await authenticatedFetch(`${API_BASE_URL}/api/v1/managers/${managerId}/teams/action-items`);
  if (!res.ok) throw new Error('Failed to fetch action item summary');
  const data = await res.json();
  return data.teams ?? [];
}
