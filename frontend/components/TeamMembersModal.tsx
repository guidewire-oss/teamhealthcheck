'use client';

import { useState, useEffect } from 'react';
import { X, AlertCircle, Loader2, Plus, Trash2, Search } from 'lucide-react';
import {
  getTeamMembers,
  addTeamMember,
  removeTeamMember,
  listUsers,
  type TeamMemberAdmin,
  type AdminUser,
} from '@/lib/api/admin';

interface TeamMembersModalProps {
  teamId: string;
  teamName: string;
  onClose: () => void;
}

export default function TeamMembersModal({
  teamId,
  teamName,
  onClose,
}: TeamMembersModalProps) {
  const [members, setMembers] = useState<TeamMemberAdmin[]>([]);
  const [allUsers, setAllUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [adding, setAdding] = useState(false);
  const [removingUserId, setRemovingUserId] = useState<string | null>(null);

  useEffect(() => {
    loadData();
  }, [teamId]);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [membersRes, usersRes] = await Promise.all([
        getTeamMembers(teamId),
        listUsers(),
      ]);
      setMembers(membersRes.members || []);
      setAllUsers(usersRes.users || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  const handleAddMember = async (userId: string) => {
    setAdding(true);
    setError(null);
    try {
      await addTeamMember(teamId, userId);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add member');
    } finally {
      setAdding(false);
    }
  };

  const handleRemoveMember = async (userId: string) => {
    setRemovingUserId(userId);
    setError(null);
    try {
      await removeTeamMember(teamId, userId);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove member');
    } finally {
      setRemovingUserId(null);
    }
  };

  const memberIds = new Set(members.map((m) => m.userId));
  const availableUsers = allUsers.filter(
    (u) => !memberIds.has(u.id) && u.fullName.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div
        className="bg-white rounded-xl shadow-xl max-w-2xl w-full max-h-[90vh] overflow-y-auto"
        data-testid="team-members-modal"
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              Manage Members
            </h2>
            <p className="text-sm text-gray-500 mt-1">{teamName}</p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
            data-testid="close-members-modal"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body */}
        <div className="p-6">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-indigo-600" />
              <span className="ml-2 text-gray-600">Loading...</span>
            </div>
          ) : (
            <>
              {error && (
                <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                  <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                  <p className="text-sm text-red-700">{error}</p>
                </div>
              )}

              {/* Current Members */}
              <h3 className="text-sm font-medium text-gray-700 mb-2">
                Current Members ({members.length})
              </h3>
              {members.length === 0 ? (
                <p className="text-sm text-gray-400 italic py-2" data-testid="no-members-msg">
                  No members in this team yet.
                </p>
              ) : (
                <div className="space-y-2 mb-6">
                  {members.map((member) => (
                    <div
                      key={member.userId}
                      className="flex items-center justify-between p-3 bg-gray-50 rounded-lg"
                      data-testid="member-row"
                    >
                      <div>
                        <p className="text-sm font-medium text-gray-900">
                          {member.userName}
                        </p>
                        <p className="text-xs text-gray-500">{member.email}</p>
                      </div>
                      <button
                        onClick={() => handleRemoveMember(member.userId)}
                        disabled={removingUserId === member.userId}
                        className="text-red-500 hover:text-red-700 disabled:opacity-50"
                        data-testid="remove-member-btn"
                        title="Remove member"
                      >
                        {removingUserId === member.userId ? (
                          <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                          <Trash2 className="w-4 h-4" />
                        )}
                      </button>
                    </div>
                  ))}
                </div>
              )}

              {/* Add Member */}
              <h3 className="text-sm font-medium text-gray-700 mb-2">
                Add Member
              </h3>
              <div className="relative mb-3">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                <input
                  type="text"
                  placeholder="Search users..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-full pl-9 pr-4 py-2 border border-gray-300 rounded-lg text-sm text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  data-testid="member-search-input"
                />
              </div>
              <div className="max-h-48 overflow-y-auto space-y-1">
                {availableUsers.length === 0 ? (
                  <p className="text-sm text-gray-400 italic py-2">
                    {searchQuery ? 'No matching users found.' : 'All users are already members.'}
                  </p>
                ) : (
                  availableUsers.slice(0, 10).map((user) => (
                    <div
                      key={user.id}
                      className="flex items-center justify-between p-2 hover:bg-gray-50 rounded-lg"
                      data-testid="available-user-row"
                    >
                      <div>
                        <p className="text-sm text-gray-900">{user.fullName}</p>
                        <p className="text-xs text-gray-500">{user.email}</p>
                      </div>
                      <button
                        onClick={() => handleAddMember(user.id)}
                        disabled={adding}
                        className="text-indigo-600 hover:text-indigo-800 disabled:opacity-50"
                        data-testid="add-member-btn"
                        title="Add to team"
                      >
                        <Plus className="w-4 h-4" />
                      </button>
                    </div>
                  ))
                )}
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end p-6 border-t">
          <button
            onClick={onClose}
            className="px-4 py-2 text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
