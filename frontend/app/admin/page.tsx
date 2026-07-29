// SPDX-License-Identifier: Apache-2.0

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { getCurrentUser, logout } from "@/lib/auth";
// Removed HEALTH_DIMENSIONS - now using DimensionConfig component with database
import {
  listUsers,
  createUser,
  updateUser,
  deleteUser,
  listHierarchyLevels,
  listAdminTeams,
  createTeam,
  updateTeam as updateAdminTeam,
  deleteTeam,
  clearAdminCache,
  getBrandingSettings,
  updateBrandingSettings,
  getNotificationSettings,
  updateNotificationSettings,
  getRetentionPolicy,
  updateRetentionPolicy,
  AdminUser,
  HierarchyLevel,
  AdminTeam,
  CreateUserRequest,
  UpdateUserRequest,
} from "@/lib/api/admin";
import {
  Settings,
  Users,
  Calendar,
  Plus,
  Edit2,
  Trash2,
  LogOut,
  Shield,
  Save,
  X,
  Building2,
  AlertCircle,
  GitBranch,
} from "lucide-react";
import HierarchyConfig from "@/components/HierarchyConfig";
import DimensionConfig from "@/components/DimensionConfig";
import SupervisorChainModal from "@/components/SupervisorChainModal";
import TeamMembersModal from "@/components/TeamMembersModal";

export default function AdminPage() {
  const router = useRouter();
  const [user, setUser] = useState<any>(null);
  const [activeTab, setActiveTab] = useState<
    "hierarchy" | "teams" | "users" | "settings"
  >("hierarchy");

  // Teams tab state
  const [showNewTeamForm, setShowNewTeamForm] = useState(false);
  const [editingTeam, setEditingTeam] = useState<AdminTeam | null>(null);
  const [teams, setTeams] = useState<AdminTeam[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [teamsError, setTeamsError] = useState<string | null>(null);
  const [teamFormData, setTeamFormData] = useState({
    name: "",
    teamLeadId: "",
    cadence: "monthly",
    distributionListEmail: "",
  });
  const [teamFormLoading, setTeamFormLoading] = useState(false);
  const [teamFormError, setTeamFormError] = useState<string | null>(null);
  const [deletingTeamId, setDeletingTeamId] = useState<string | null>(null);
  const [supervisorChainTeam, setSupervisorChainTeam] = useState<AdminTeam | null>(null);
  const [membersTeam, setMembersTeam] = useState<AdminTeam | null>(null);
  const [availableTeamLeads, setAvailableTeamLeads] = useState<AdminUser[]>([]);
  const [teamLeadsLoading, setTeamLeadsLoading] = useState(false);
  const [teamSearchQuery, setTeamSearchQuery] = useState("");

  const [showNewUserForm, setShowNewUserForm] = useState(false);
  const [editingUser, setEditingUser] = useState<AdminUser | null>(null);

  // Users tab state
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [hierarchyLevels, setHierarchyLevels] = useState<HierarchyLevel[]>([]);
  const [adminTeams, setAdminTeams] = useState<AdminTeam[]>([]);
  const [userFormData, setUserFormData] = useState({
    fullName: "",
    username: "",
    email: "",
    password: "",
    hierarchyLevel: "",
    reportsTo: "",
    authType: "local" as "local" | "sso",
  });
  const [userFormError, setUserFormError] = useState<string | null>(null);
  const [userFormSubmitting, setUserFormSubmitting] = useState(false);
  const [deleteConfirmUserId, setDeleteConfirmUserId] = useState<string | null>(
    null,
  );
  const [userSearchQuery, setUserSearchQuery] = useState("");
  const [userRoleFilter, setUserRoleFilter] = useState("");

  // Settings tab state
  const [emailEnabled, setEmailEnabled] = useState(false);
  const [slackEnabled, setSlackEnabled] = useState(false);
  const [notifyOnSubmission, setNotifyOnSubmission] = useState(false);
  const [smtpConfigured, setSmtpConfigured] = useState(false);
  const [retentionMonths, setRetentionMonths] = useState(12);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [settingsSuccess, setSettingsSuccess] = useState(false);

  // Branding state
  const [brandingCompanyName, setBrandingCompanyName] = useState("My Company");
  const [brandingLogoURL, setBrandingLogoURL] = useState("");

  useEffect(() => {
    const currentUser = getCurrentUser();
    if (!currentUser) {
      router.push("/login");
    } else if (!currentUser.isAdmin) {
      router.push("/survey");
    } else {
      setUser(currentUser);
    }
  }, [router]);

  // Fetch teams and team leads when activeTab changes to 'teams'
  useEffect(() => {
    if (activeTab === "teams") {
      fetchAdminTeams();
      fetchTeamLeads();
    }
  }, [activeTab]);

  // Fetch users, hierarchy levels, and teams when activeTab changes to 'users'
  useEffect(() => {
    if (activeTab === "users") {
      loadUsersData();
    }
  }, [activeTab]);

  // Fetch settings and teams when activeTab changes to 'settings'
  useEffect(() => {
    if (activeTab === "settings") {
      loadSettings();
      if (teams.length === 0) fetchAdminTeams();
    }
  }, [activeTab]);

  const loadSettings = async () => {
    setSettingsLoading(true);
    setSettingsError(null);
    try {
      const [notifSettings, retention, branding] = await Promise.all([
        getNotificationSettings(),
        getRetentionPolicy(),
        getBrandingSettings(),
      ]);
      setEmailEnabled(notifSettings.emailEnabled);
      setSlackEnabled(notifSettings.slackEnabled);
      setNotifyOnSubmission(notifSettings.notifyOnSubmission);
      setSmtpConfigured(notifSettings.smtpConfigured);
      setRetentionMonths(retention.keepSessionsMonths);
      setBrandingCompanyName(branding.companyName || "My Company");
      setBrandingLogoURL(branding.logoURL || "");
    } catch (err) {
      setSettingsError("Failed to load settings");
    } finally {
      setSettingsLoading(false);
    }
  };

  const saveSettings = async () => {
    setSettingsSaving(true);
    setSettingsError(null);
    setSettingsSuccess(false);
    try {
      await Promise.all([
        updateBrandingSettings({
          companyName: brandingCompanyName,
          logoURL: brandingLogoURL,
        }),
        updateNotificationSettings({
          emailEnabled,
          slackEnabled,
          notifyOnSubmission,
        }),
        updateRetentionPolicy({ keepSessionsMonths: retentionMonths }),
      ]);
      setSettingsSuccess(true);
      setTimeout(() => setSettingsSuccess(false), 3000);
    } catch (err) {
      setSettingsError("Failed to save settings");
    } finally {
      setSettingsSaving(false);
    }
  };

  const handleLogoUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (file.size > 500 * 1024) {
      setSettingsError("Logo file must be less than 500KB");
      return;
    }

    const allowedLogoTypes = ["image/png", "image/jpeg", "image/webp"];
    if (!allowedLogoTypes.includes(file.type)) {
      setSettingsError("Please upload a PNG, JPEG, or WebP image file");
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      setBrandingLogoURL(reader.result as string);
      setSettingsError(null);
    };
    reader.readAsDataURL(file);
  };

  const loadUsersData = async () => {
    setUsersLoading(true);
    setUsersError(null);

    try {
      const [usersData, levelsData, teamsData] = await Promise.all([
        listUsers(),
        listHierarchyLevels(),
        listAdminTeams(),
      ]);

      setUsers(usersData.users);
      setHierarchyLevels(levelsData);
      setAdminTeams(teamsData.teams);
    } catch (err: any) {
      console.error("Failed to load users data:", err);
      setUsersError(err.message || "Failed to load users. Please try again.");
    } finally {
      setUsersLoading(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    router.push("/login");
  };

  // Teams tab functions
  const fetchAdminTeams = async () => {
    setTeamsLoading(true);
    setTeamsError(null);
    try {
      const response = await listAdminTeams();
      setTeams(response.teams);
    } catch (err: any) {
      console.error("Failed to load teams:", err);
      setTeamsError(err.message || "Failed to load teams");
    } finally {
      setTeamsLoading(false);
    }
  };

  const fetchTeamLeads = async () => {
    setTeamLeadsLoading(true);
    try {
      const response = await listUsers();
      // Filter to managers, team leads, and directors for team lead dropdown
      const eligibleUsers = response.users.filter((u) =>
        ["level-3", "level-4", "level-2"].includes(u.hierarchyLevel),
      );
      setAvailableTeamLeads(eligibleUsers);
    } catch (err) {
      console.error("Failed to load team leads:", err);
    } finally {
      setTeamLeadsLoading(false);
    }
  };

  const handleShowCreateTeamForm = () => {
    setTeamFormData({ name: "", teamLeadId: "", cadence: "monthly", distributionListEmail: "" });
    setTeamFormError(null);
    setShowNewTeamForm(true);
    setEditingTeam(null);
  };

  const handleShowEditTeamForm = (team: AdminTeam) => {
    setTeamFormData({
      name: team.name,
      teamLeadId: team.teamLeadId || "",
      cadence: team.cadence,
      distributionListEmail: team.distributionListEmail || "",
    });
    setTeamFormError(null);
    setEditingTeam(team);
    setShowNewTeamForm(false);
  };

  const handleCancelTeamForm = () => {
    setShowNewTeamForm(false);
    setEditingTeam(null);
    setTeamFormData({ name: "", teamLeadId: "", cadence: "monthly", distributionListEmail: "" });
    setTeamFormError(null);
  };

  const isValidEmail = (email: string) => /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/.test(email);

  const handleCreateTeam = async () => {
    if (!teamFormData.name.trim()) {
      setTeamFormError("Team name is required");
      return;
    }
    const dlEmail = teamFormData.distributionListEmail.trim();
    if (dlEmail && !isValidEmail(dlEmail)) {
      setTeamFormError("Invalid email format for distribution list");
      return;
    }

    setTeamFormLoading(true);
    setTeamFormError(null);
    try {
      const newTeam = await createTeam({
        name: teamFormData.name.trim(),
        teamLeadId: teamFormData.teamLeadId || null,
        cadence: teamFormData.cadence,
        distributionListEmail: dlEmail || null,
      });

      // Clear cache and refresh teams list
      clearAdminCache();
      await fetchAdminTeams();

      // Close form (supervisor chain is auto-derived from user hierarchy)
      handleCancelTeamForm();
    } catch (err: any) {
      console.error("Failed to create team:", err);
      setTeamFormError(err.message || "Failed to create team");
    } finally {
      setTeamFormLoading(false);
    }
  };

  const handleUpdateTeam = async () => {
    if (!editingTeam) return;

    if (!teamFormData.name.trim()) {
      setTeamFormError("Team name is required");
      return;
    }
    const dlEmail = teamFormData.distributionListEmail.trim();
    if (dlEmail && !isValidEmail(dlEmail)) {
      setTeamFormError("Invalid email format for distribution list");
      return;
    }

    setTeamFormLoading(true);
    setTeamFormError(null);
    try {
      await updateAdminTeam(editingTeam.id, {
        name: teamFormData.name.trim(),
        teamLeadId: teamFormData.teamLeadId || null,
        cadence: teamFormData.cadence,
        distributionListEmail: dlEmail || null,
      });

      // Clear cache and refresh teams list
      clearAdminCache();
      await fetchAdminTeams();

      // Close form
      handleCancelTeamForm();
    } catch (err: any) {
      console.error("Failed to update team:", err);
      setTeamFormError("Failed to save team changes. Please try again or contact support if the issue persists.");
    } finally {
      setTeamFormLoading(false);
    }
  };

  const handleDeleteTeam = async (teamId: string) => {
    if (
      !confirm(
        "Are you sure you want to delete this team? This action cannot be undone.",
      )
    ) {
      return;
    }

    setDeletingTeamId(teamId);
    try {
      await deleteTeam(teamId);

      // Clear cache and refresh teams list
      clearAdminCache();
      await fetchAdminTeams();
    } catch (err: any) {
      console.error("Failed to delete team:", err);
      alert(err.message || "Failed to delete team");
    } finally {
      setDeletingTeamId(null);
    }
  };

  // Helper function to get role badge color
  const getRoleBadgeColor = (hierarchyLevel: string): string => {
    const level = hierarchyLevels.find((l) => l.id === hierarchyLevel);
    if (!level) return "bg-gray-100 text-gray-700";

    const name = level.name.toLowerCase();
    if (name.includes("vp")) return "bg-purple-100 text-purple-700";
    if (name.includes("director")) return "bg-blue-100 text-blue-700";
    if (name.includes("manager")) return "bg-green-100 text-green-700";
    if (name.includes("lead")) return "bg-yellow-100 text-yellow-700";
    return "bg-gray-100 text-gray-700";
  };

  // Get user's teams
  const getUserTeams = (userId: string): string => {
    const userTeams = adminTeams.filter((team) =>
      users.find((u) => u.id === userId)?.teamIds.includes(team.id),
    );

    if (userTeams.length === 0) return "-";
    if (userTeams.length === 1) return userTeams[0].name;
    return `${userTeams.length} teams`;
  };

  // Reset user form
  const resetUserForm = () => {
    setUserFormData({
      fullName: "",
      username: "",
      email: "",
      password: "",
      hierarchyLevel: "",
      reportsTo: "",
      authType: "local",
    });
    setUserFormError(null);
    setShowNewUserForm(false);
    setEditingUser(null);
  };

  // Handle add user button click
  const handleAddUser = () => {
    resetUserForm();
    setShowNewUserForm(true);
  };

  // Handle edit user button click
  const handleEditUser = (userToEdit: AdminUser) => {
    setUserFormData({
      fullName: userToEdit.fullName,
      username: userToEdit.username,
      email: userToEdit.email,
      password: "",
      hierarchyLevel: userToEdit.hierarchyLevel,
      reportsTo: userToEdit.reportsTo || "",
      authType: userToEdit.authType || "local",
    });
    setUserFormError(null);
    setEditingUser(userToEdit);
    setShowNewUserForm(false);
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  // Handle user form submit
  const handleSaveUser = async () => {
    setUserFormError(null);
    setUserFormSubmitting(true);

    try {
      // Validation
      if (!userFormData.fullName.trim()) {
        throw new Error("Full name is required");
      }
      if (!userFormData.username.trim()) {
        throw new Error("Username is required");
      }
      if (!userFormData.email.trim()) {
        throw new Error("Email is required");
      }
      if (
        userFormData.authType === "local" &&
        (!editingUser || editingUser.authType === "sso") &&
        !userFormData.password.trim()
      ) {
        throw new Error("Password is required for local users");
      }
      if (!userFormData.hierarchyLevel) {
        throw new Error("Role is required");
      }

      if (editingUser) {
        // Update existing user
        const updateData: UpdateUserRequest = {
          fullName: userFormData.fullName,
          username: userFormData.username,
          email: userFormData.email,
          hierarchyLevel: userFormData.hierarchyLevel,
          reportsTo: userFormData.reportsTo || null,
          authType: userFormData.authType,
        };

        // Only include password if it was changed (and user is local)
        if (userFormData.authType === "local" && userFormData.password.trim()) {
          updateData.password = userFormData.password;
        }

        await updateUser(editingUser.id, updateData);
      } else {
        // Create new user
        const createData: CreateUserRequest = {
          id: userFormData.username, // Use username as ID
          fullName: userFormData.fullName,
          username: userFormData.username,
          email: userFormData.email,
          hierarchyLevel: userFormData.hierarchyLevel,
          reportsTo: userFormData.reportsTo || null,
          authType: userFormData.authType,
          ...(userFormData.authType === "local" ? { password: userFormData.password } : {}),
        };
        await createUser(createData);
      }

      // Clear cache and reload users
      clearAdminCache();
      await loadUsersData();
      resetUserForm();
    } catch (err: any) {
      console.error("Failed to save user:", err);
      setUserFormError(err.message || "Failed to save user. Please try again.");
    } finally {
      setUserFormSubmitting(false);
    }
  };

  // Handle delete user
  const handleDeleteUser = async (userId: string) => {
    // Prevent deleting self or admin user
    if (userId === user?.id) {
      alert("You cannot delete your own account");
      return;
    }

    const userToDelete = users.find((u) => u.id === userId);
    if (userToDelete?.username === "admin") {
      alert("Cannot delete the admin user");
      return;
    }

    setDeleteConfirmUserId(userId);
  };

  const confirmDeleteUser = async () => {
    if (!deleteConfirmUserId) return;

    try {
      await deleteUser(deleteConfirmUserId);
      clearAdminCache();
      await loadUsersData();
      setDeleteConfirmUserId(null);
    } catch (err: any) {
      console.error("Failed to delete user:", err);
      alert(err.message || "Failed to delete user. Please try again.");
    }
  };

  // Get potential supervisors (users at higher hierarchy levels)
  const getPotentialSupervisors = (): AdminUser[] => {
    if (!userFormData.hierarchyLevel) return [];

    const selectedLevel = hierarchyLevels.find(
      (l) => l.id === userFormData.hierarchyLevel,
    );
    if (!selectedLevel) return [];

    return users.filter((u) => {
      const userLevel = hierarchyLevels.find((l) => l.id === u.hierarchyLevel);
      if (!userLevel) return false;
      // Only show users at higher positions (lower position number)
      return userLevel.position < selectedLevel.position;
    });
  };

  if (!user) return null;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="bg-white shadow-sm border-b">
        <div className="container mx-auto px-4 py-4">
          <div className="flex justify-between items-center">
            <div className="flex items-center gap-3">
              <Shield className="w-8 h-8 text-indigo-600" />
              <div>
                <h1 className="text-2xl font-bold text-gray-900">
                  Admin Dashboard
                </h1>
                <p className="text-gray-500">System Administration</p>
              </div>
            </div>
            <div className="flex items-center gap-4">
              <div className="text-right">
                <p className="text-sm text-gray-500">Administrator</p>
                <p className="font-semibold text-gray-900">{user.name}</p>
              </div>
              <button
                onClick={handleLogout}
                className="flex items-center gap-2 px-4 py-2 bg-red-50 text-red-600 rounded-lg hover:bg-red-100 transition-colors"
              >
                <LogOut className="w-4 h-4" />
                Logout
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className="container mx-auto px-4 py-8">
        <div className="flex gap-4 mb-8 border-b">
          <button
            data-testid="hierarchy-tab"
            onClick={() => setActiveTab("hierarchy")}
            className={`px-6 py-3 font-medium transition-colors border-b-2 ${
              activeTab === "hierarchy"
                ? "text-indigo-600 border-indigo-600"
                : "text-gray-500 border-transparent hover:text-gray-700"
            }`}
          >
            <div className="flex items-center gap-2">
              <Building2 className="w-5 h-5" />
              Hierarchy
            </div>
          </button>
          <button
            data-testid="teams-tab"
            onClick={() => setActiveTab("teams")}
            className={`px-6 py-3 font-medium transition-colors border-b-2 ${
              activeTab === "teams"
                ? "text-indigo-600 border-indigo-600"
                : "text-gray-500 border-transparent hover:text-gray-700"
            }`}
          >
            <div className="flex items-center gap-2">
              <Users className="w-5 h-5" />
              Teams
            </div>
          </button>
          <button
            data-testid="users-tab"
            onClick={() => setActiveTab("users")}
            className={`px-6 py-3 font-medium transition-colors border-b-2 ${
              activeTab === "users"
                ? "text-indigo-600 border-indigo-600"
                : "text-gray-500 border-transparent hover:text-gray-700"
            }`}
          >
            <div className="flex items-center gap-2">
              <Users className="w-5 h-5" />
              Users
            </div>
          </button>
          <button
            data-testid="settings-tab"
            onClick={() => setActiveTab("settings")}
            className={`px-6 py-3 font-medium transition-colors border-b-2 ${
              activeTab === "settings"
                ? "text-indigo-600 border-indigo-600"
                : "text-gray-500 border-transparent hover:text-gray-700"
            }`}
          >
            <div className="flex items-center gap-2">
              <Settings className="w-5 h-5" />
              Settings
            </div>
          </button>
        </div>

        {activeTab === "hierarchy" && <HierarchyConfig />}

        {activeTab === "teams" && (
          <div>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-xl font-semibold text-gray-900">
                Manage Teams
              </h2>
              <button
                data-testid="add-team-btn"
                onClick={handleShowCreateTeamForm}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
              >
                <Plus className="w-5 h-5" />
                Add Team
              </button>
            </div>

            {/* Error message */}
            {teamsError && (
              <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3">
                <AlertCircle className="w-5 h-5 text-red-600 mt-0.5" />
                <div>
                  <p className="font-medium text-red-900">
                    Error loading teams
                  </p>
                  <p className="text-sm text-red-700">{teamsError}</p>
                </div>
              </div>
            )}

            {/* Create team modal */}
            {showNewTeamForm && (
              <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" data-testid="team-create-modal">
                <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="text-lg font-semibold text-gray-900 mb-4">
                    New Team
                  </h3>

                  {teamFormError && (
                    <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                      <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                      <p className="text-sm text-red-700">{teamFormError}</p>
                    </div>
                  )}

                  <div className="grid grid-cols-1 gap-4">
                    <div>
                      <label htmlFor="team-name" className="block text-sm font-medium text-gray-700 mb-2">Team Name *</label>
                      <input type="text" id="team-name" data-testid="team-name-input" value={teamFormData.name} onChange={(e) => setTeamFormData({ ...teamFormData, name: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" placeholder="Enter team name" disabled={teamFormLoading} />
                    </div>
                    <div>
                      <label htmlFor="team-lead" className="block text-sm font-medium text-gray-700 mb-2">Team Lead</label>
                      <select id="team-lead" data-testid="team-lead-select" value={teamFormData.teamLeadId} onChange={(e) => setTeamFormData({ ...teamFormData, teamLeadId: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" disabled={teamFormLoading || teamLeadsLoading}>
                        <option value="">No team lead</option>
                        {availableTeamLeads.map((user) => (<option key={user.id} value={user.id}>{user.fullName} ({user.username})</option>))}
                      </select>
                    </div>
                    <div>
                      <label htmlFor="cadence" className="block text-sm font-medium text-gray-700 mb-2">Cadence *</label>
                      <select id="cadence" data-testid="team-cadence-select" value={teamFormData.cadence} onChange={(e) => setTeamFormData({ ...teamFormData, cadence: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" disabled={teamFormLoading}>
                        <option value="monthly">Monthly</option>
                        <option value="quarterly">Quarterly</option>
                        <option value="half-yearly">Half-Yearly</option>
                        <option value="yearly">Yearly</option>
                      </select>
                    </div>
                  </div>

                  <div className="mt-4">
                    <label htmlFor="distributionListEmail" className="block text-sm font-medium text-gray-700 mb-2">
                      Distribution List Email
                    </label>
                    <input
                      type="email"
                      id="distributionListEmail"
                      data-testid="team-dl-email-input"
                      value={teamFormData.distributionListEmail}
                      onChange={(e) => setTeamFormData({...teamFormData, distributionListEmail: e.target.value})}
                      placeholder="team-dl@company.com"
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      disabled={teamFormLoading}
                    />
                    <p className="text-xs text-gray-500 mt-1">
                      Post-workshop survey summaries will be sent to this address.
                    </p>
                  </div>

                  <div className="flex gap-4 mt-6">
                    <button data-testid="save-team-btn" onClick={handleCreateTeam} disabled={teamFormLoading} className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                      <Save className="w-4 h-4" />
                      {teamFormLoading ? "Creating..." : "Create Team"}
                    </button>
                    <button onClick={handleCancelTeamForm} disabled={teamFormLoading} className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50">
                      <X className="w-4 h-4" />
                      Cancel
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Edit team modal */}
            {editingTeam && (
              <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" data-testid="team-edit-modal">
                <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="text-lg font-semibold text-gray-900 mb-4">
                    Edit Team
                  </h3>

                  {teamFormError && (
                    <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                      <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                      <p className="text-sm text-red-700">{teamFormError}</p>
                    </div>
                  )}

                  <div className="grid grid-cols-1 gap-4">
                    <div>
                      <label htmlFor="edit-team-name" className="block text-sm font-medium text-gray-700 mb-2">Team Name *</label>
                      <input type="text" id="edit-team-name" data-testid="team-name-input" value={teamFormData.name} onChange={(e) => setTeamFormData({ ...teamFormData, name: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" placeholder="Enter team name" disabled={teamFormLoading} />
                    </div>
                    <div>
                      <label htmlFor="edit-team-lead" className="block text-sm font-medium text-gray-700 mb-2">Team Lead</label>
                      <select id="edit-team-lead" data-testid="team-lead-select" value={teamFormData.teamLeadId} onChange={(e) => setTeamFormData({ ...teamFormData, teamLeadId: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" disabled={teamFormLoading || teamLeadsLoading}>
                        <option value="">No team lead</option>
                        {availableTeamLeads.map((user) => (<option key={user.id} value={user.id}>{user.fullName} ({user.username})</option>))}
                      </select>
                    </div>
                    <div>
                      <label htmlFor="edit-cadence" className="block text-sm font-medium text-gray-700 mb-2">Cadence *</label>
                      <select id="edit-cadence" data-testid="team-cadence-select" value={teamFormData.cadence} onChange={(e) => setTeamFormData({ ...teamFormData, cadence: e.target.value })} className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent" disabled={teamFormLoading}>
                        <option value="monthly">Monthly</option>
                        <option value="quarterly">Quarterly</option>
                        <option value="half-yearly">Half-Yearly</option>
                        <option value="yearly">Yearly</option>
                      </select>
                    </div>
                  </div>

                  <div className="mt-4">
                    <label htmlFor="edit-distributionListEmail" className="block text-sm font-medium text-gray-700 mb-2">
                      Distribution List Email
                    </label>
                    <input
                      type="email"
                      id="edit-distributionListEmail"
                      data-testid="team-dl-email-edit-input"
                      value={teamFormData.distributionListEmail}
                      onChange={(e) => setTeamFormData({...teamFormData, distributionListEmail: e.target.value})}
                      placeholder="team-dl@company.com"
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      disabled={teamFormLoading}
                    />
                    <p className="text-xs text-gray-500 mt-1">
                      Post-workshop survey summaries will be sent to this address.
                    </p>
                  </div>

                  <div className="flex gap-4 mt-6">
                    <button data-testid="save-team-btn" onClick={handleUpdateTeam} disabled={teamFormLoading} className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                      <Save className="w-4 h-4" />
                      {teamFormLoading ? "Saving..." : "Save Changes"}
                    </button>
                    <button onClick={handleCancelTeamForm} disabled={teamFormLoading} className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50">
                      <X className="w-4 h-4" />
                      Cancel
                    </button>
                  </div>
                </div>
              </div>
            )}

            {!teamsLoading && teams.length > 0 && (
              <div className="mb-4">
                <input
                  type="text"
                  data-testid="team-search-input"
                  value={teamSearchQuery}
                  onChange={(e) => setTeamSearchQuery(e.target.value)}
                  className="w-full max-w-md px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  placeholder="Search teams by name or team lead..."
                />
              </div>
            )}

            {teamsLoading ? (
              <div className="text-center py-8">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto mb-4"></div>
                <p className="text-gray-500">Loading teams...</p>
              </div>
            ) : teams.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                No teams found. Add your first team to get started.
              </div>
            ) : (
              <div className="bg-white rounded-xl shadow-sm border overflow-hidden">
                <table className="w-full" data-testid="teams-list">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Team Name
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Cadence
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Created
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Members
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Team Lead
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {teams.filter((team) => {
                      if (!teamSearchQuery) return true;
                      const q = teamSearchQuery.toLowerCase();
                      return team.name.toLowerCase().includes(q) || (team.teamLeadName || '').toLowerCase().includes(q);
                    }).map((team) => (
                      <tr key={team.id} data-testid="team-row">
                        <td className="px-6 py-4">
                          <div className="font-semibold text-gray-900">
                            {team.name}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-1 text-sm text-gray-500">
                            <Calendar className="w-4 h-4" />
                            {team.cadence}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="text-sm text-gray-500">
                            {team.createdAt ? new Date(team.createdAt).toLocaleDateString() : "-"}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-1 text-sm text-gray-500">
                            <Users className="w-4 h-4" />
                            {team.memberCount}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="text-sm text-gray-500">
                            {team.teamLeadName || "Not assigned"}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex gap-2">
                            <button
                              data-testid="edit-team-btn"
                              onClick={() => handleShowEditTeamForm(team)}
                              className="text-indigo-600 hover:text-indigo-900"
                              disabled={deletingTeamId === team.id}
                              title="Edit team"
                            >
                              <Edit2 className="w-4 h-4" />
                            </button>
                            <button
                              data-testid="manage-members-btn"
                              onClick={() => setMembersTeam(team)}
                              className="text-blue-600 hover:text-blue-900"
                              disabled={deletingTeamId === team.id}
                              title="Manage members"
                            >
                              <Users className="w-4 h-4" />
                            </button>
                            <button
                              data-testid="manage-hierarchy-btn"
                              onClick={() => setSupervisorChainTeam(team)}
                              className="text-emerald-600 hover:text-emerald-900"
                              disabled={deletingTeamId === team.id}
                              title="View hierarchy"
                            >
                              <GitBranch className="w-4 h-4" />
                            </button>
                            <button
                              data-testid="delete-team-btn"
                              onClick={() => handleDeleteTeam(team.id)}
                              className="text-red-600 hover:text-red-900 disabled:opacity-50"
                              disabled={deletingTeamId === team.id}
                            >
                              {deletingTeamId === team.id ? (
                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-red-600"></div>
                              ) : (
                                <Trash2 className="w-4 h-4" />
                              )}
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {/* Supervisor Chain Modal (read-only, auto-derived from user hierarchy) */}
            {supervisorChainTeam && (
              <SupervisorChainModal
                teamId={supervisorChainTeam.id}
                teamName={supervisorChainTeam.name}
                onClose={() => setSupervisorChainTeam(null)}
              />
            )}

            {/* Team Members Modal */}
            {membersTeam && (
              <TeamMembersModal
                teamId={membersTeam.id}
                teamName={membersTeam.name}
                onClose={() => {
                  setMembersTeam(null);
                  fetchAdminTeams(); // Refresh member counts
                }}
              />
            )}
          </div>
        )}

        {activeTab === "users" && (
          <div>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-xl font-semibold text-gray-900">
                Manage Users
              </h2>
              <button
                data-testid="add-user-btn"
                onClick={handleAddUser}
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
              >
                <Plus className="w-5 h-5" />
                Add User
              </button>
            </div>

            {usersError && (
              <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-6">
                {usersError}
              </div>
            )}

            {showNewUserForm && (
              <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" data-testid="user-create-modal">
              <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
                <h3 className="text-lg font-semibold text-gray-900 mb-4">
                  New User
                </h3>

                {userFormError && (
                  <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4">
                    {userFormError}
                  </div>
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Full Name *
                    </label>
                    <input
                      type="text"
                      data-testid="user-fullname-input"
                      value={userFormData.fullName}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          fullName: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter full name"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Username *
                    </label>
                    <input
                      type="text"
                      data-testid="user-username-input"
                      value={userFormData.username}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          username: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter username"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Email *
                    </label>
                    <input
                      type="email"
                      data-testid="user-email-input"
                      value={userFormData.email}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          email: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter email"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Authentication Type
                    </label>
                    <div className="flex gap-4">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="radio"
                          name="create-auth-type"
                          data-testid="auth-type-local"
                          checked={userFormData.authType === "local"}
                          onChange={() =>
                            setUserFormData({ ...userFormData, authType: "local" })
                          }
                          className="text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="text-sm text-gray-700">Local (Password)</span>
                      </label>
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="radio"
                          name="create-auth-type"
                          data-testid="auth-type-sso"
                          checked={userFormData.authType === "sso"}
                          onChange={() =>
                            setUserFormData({ ...userFormData, authType: "sso", password: "" })
                          }
                          className="text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="text-sm text-gray-700">SSO (Identity Provider)</span>
                      </label>
                    </div>
                  </div>
                  {userFormData.authType === "local" && (
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        Password *
                      </label>
                      <input
                        type="password"
                        data-testid="user-password-input"
                        value={userFormData.password}
                        onChange={(e) =>
                          setUserFormData({
                            ...userFormData,
                            password: e.target.value,
                          })
                        }
                        className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                        placeholder="Enter password"
                      />
                    </div>
                  )}
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Role *
                    </label>
                    <select
                      data-testid="user-role-select"
                      value={userFormData.hierarchyLevel}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          hierarchyLevel: e.target.value,
                          reportsTo: "",
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    >
                      <option value="">Select Role</option>
                      {hierarchyLevels.map((level) => (
                        <option key={level.id} value={level.id}>
                          {level.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Reports To
                    </label>
                    <select
                      data-testid="user-reportsto-select"
                      value={userFormData.reportsTo}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          reportsTo: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      disabled={!userFormData.hierarchyLevel}
                    >
                      <option value="">None</option>
                      {getPotentialSupervisors().map((supervisor) => (
                        <option key={supervisor.id} value={supervisor.id}>
                          {supervisor.fullName} (
                          {
                            hierarchyLevels.find(
                              (l) => l.id === supervisor.hierarchyLevel,
                            )?.name
                          }
                          )
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
                <div className="flex gap-4 mt-6">
                  <button
                    data-testid="save-user-btn"
                    onClick={handleSaveUser}
                    disabled={userFormSubmitting}
                    className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Save className="w-4 h-4" />
                    {userFormSubmitting ? "Saving..." : "Save User"}
                  </button>
                  <button
                    onClick={resetUserForm}
                    disabled={userFormSubmitting}
                    className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <X className="w-4 h-4" />
                    Cancel
                  </button>
                </div>
              </div>
              </div>
            )}

            {editingUser && (
              <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" data-testid="user-edit-modal">
              <div className="bg-white rounded-lg p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
                <h3 className="text-lg font-semibold text-gray-900 mb-4">
                  Edit User
                </h3>

                {userFormError && (
                  <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4">
                    {userFormError}
                  </div>
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Full Name *
                    </label>
                    <input
                      type="text"
                      data-testid="user-fullname-input"
                      value={userFormData.fullName}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          fullName: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter full name"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Username *
                    </label>
                    <input
                      type="text"
                      data-testid="user-username-input"
                      value={userFormData.username}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          username: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter username"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Email *
                    </label>
                    <input
                      type="email"
                      data-testid="user-email-input"
                      value={userFormData.email}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          email: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      placeholder="Enter email"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Authentication Type
                    </label>
                    <div className="flex gap-4">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="radio"
                          name="edit-auth-type"
                          data-testid="auth-type-local"
                          checked={userFormData.authType === "local"}
                          onChange={() =>
                            setUserFormData({ ...userFormData, authType: "local" })
                          }
                          className="text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="text-sm text-gray-700">Local (Password)</span>
                      </label>
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="radio"
                          name="edit-auth-type"
                          data-testid="auth-type-sso"
                          checked={userFormData.authType === "sso"}
                          onChange={() =>
                            setUserFormData({ ...userFormData, authType: "sso", password: "" })
                          }
                          className="text-indigo-600 focus:ring-indigo-500"
                        />
                        <span className="text-sm text-gray-700">SSO (Identity Provider)</span>
                      </label>
                    </div>
                  </div>
                  {userFormData.authType === "local" && (
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        Password (leave blank to keep current)
                      </label>
                      <input
                        type="password"
                        data-testid="user-password-input"
                        value={userFormData.password}
                        onChange={(e) =>
                          setUserFormData({
                            ...userFormData,
                            password: e.target.value,
                          })
                        }
                        className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                        placeholder="Enter new password"
                      />
                    </div>
                  )}
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Role *
                    </label>
                    <select
                      data-testid="user-role-select"
                      value={userFormData.hierarchyLevel}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          hierarchyLevel: e.target.value,
                          reportsTo: "",
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    >
                      <option value="">Select Role</option>
                      {hierarchyLevels.map((level) => (
                        <option key={level.id} value={level.id}>
                          {level.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Reports To
                    </label>
                    <select
                      data-testid="user-reportsto-select"
                      value={userFormData.reportsTo}
                      onChange={(e) =>
                        setUserFormData({
                          ...userFormData,
                          reportsTo: e.target.value,
                        })
                      }
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      disabled={!userFormData.hierarchyLevel}
                    >
                      <option value="">None</option>
                      {getPotentialSupervisors().map((supervisor) => (
                        <option key={supervisor.id} value={supervisor.id}>
                          {supervisor.fullName} (
                          {
                            hierarchyLevels.find(
                              (l) => l.id === supervisor.hierarchyLevel,
                            )?.name
                          }
                          )
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
                <div className="flex gap-4 mt-6">
                  <button
                    data-testid="save-user-btn"
                    onClick={handleSaveUser}
                    disabled={userFormSubmitting}
                    className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Save className="w-4 h-4" />
                    {userFormSubmitting ? "Saving..." : "Save Changes"}
                  </button>
                  <button
                    onClick={resetUserForm}
                    disabled={userFormSubmitting}
                    className="flex items-center gap-2 px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <X className="w-4 h-4" />
                    Cancel
                  </button>
                </div>
              </div>
              </div>
            )}

            {!usersLoading && users.length > 0 && (
              <div className="mb-4 flex gap-4 items-center">
                <input
                  type="text"
                  data-testid="user-search-input"
                  value={userSearchQuery}
                  onChange={(e) => setUserSearchQuery(e.target.value)}
                  className="w-full max-w-md px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                  placeholder="Search by name, username, or email..."
                />
                <select
                  data-testid="user-role-filter"
                  value={userRoleFilter}
                  onChange={(e) => setUserRoleFilter(e.target.value)}
                  className="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                >
                  <option value="">All Roles</option>
                  {hierarchyLevels.map((level) => (
                    <option key={level.id} value={level.id}>{level.name}</option>
                  ))}
                </select>
              </div>
            )}

            {usersLoading ? (
              <div className="text-center py-8">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto mb-4"></div>
                <p className="text-gray-500">Loading users...</p>
              </div>
            ) : users.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                No users found. Add your first user to get started.
              </div>
            ) : (
              <div className="bg-white rounded-xl shadow-sm border overflow-hidden">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Name
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Username
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Email
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Role
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Teams
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {users.filter((userItem) => {
                      const q = userSearchQuery.toLowerCase();
                      const matchesSearch = !q || userItem.username.toLowerCase().includes(q) || userItem.fullName.toLowerCase().includes(q) || userItem.email.toLowerCase().includes(q);
                      const matchesRole = !userRoleFilter || userItem.hierarchyLevel === userRoleFilter;
                      return matchesSearch && matchesRole;
                    }).map((userItem) => (
                      <tr key={userItem.id} data-testid="user-row">
                        <td className="px-6 py-4">
                          <div className="font-medium text-gray-900">
                            {userItem.fullName}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="text-sm text-gray-500">
                            {userItem.username}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="text-sm text-gray-500">
                            {userItem.email}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`px-2 py-1 rounded-full text-sm ${getRoleBadgeColor(userItem.hierarchyLevel)}`}
                            data-testid="role-badge"
                          >
                            {hierarchyLevels.find(
                              (l) => l.id === userItem.hierarchyLevel,
                            )?.name || userItem.hierarchyLevel}
                          </span>
                          {userItem.authType === "sso" && (
                            <span
                              className="ml-2 px-2 py-1 rounded-full text-xs bg-blue-100 text-blue-700"
                              data-testid="sso-badge"
                            >
                              SSO
                            </span>
                          )}
                        </td>
                        <td className="px-6 py-4">
                          <div className="text-sm text-gray-500">
                            {getUserTeams(userItem.id)}
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex gap-2">
                            <button
                              data-testid="edit-user-btn"
                              onClick={() => handleEditUser(userItem)}
                              className="text-indigo-600 hover:text-indigo-900"
                            >
                              <Edit2 className="w-4 h-4" />
                            </button>
                            <button
                              data-testid="delete-user-btn"
                              onClick={() => handleDeleteUser(userItem.id)}
                              disabled={
                                userItem.id === user?.id ||
                                userItem.username === "admin"
                              }
                              className={`${
                                userItem.id === user?.id ||
                                userItem.username === "admin"
                                  ? "text-gray-400 cursor-not-allowed"
                                  : "text-red-600 hover:text-red-900"
                              }`}
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {/* Delete Confirmation Modal */}
            {deleteConfirmUserId && (
              <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
                <div className="bg-white rounded-lg p-6 max-w-md w-full mx-4">
                  <h3 className="text-lg font-semibold text-gray-900 mb-4">
                    Confirm Delete
                  </h3>
                  <p className="text-gray-600 mb-6">
                    Are you sure you want to delete user{" "}
                    <strong>
                      {
                        users.find((u) => u.id === deleteConfirmUserId)
                          ?.fullName
                      }
                    </strong>
                    ? This action cannot be undone.
                  </p>
                  <div className="flex gap-4 justify-end">
                    <button
                      onClick={() => setDeleteConfirmUserId(null)}
                      className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={confirmDeleteUser}
                      className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
                    >
                      Delete User
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}

        {activeTab === "settings" && (
          <div className="space-y-6">
            <div className="bg-white rounded-xl shadow-sm border p-6">
              <h2 className="text-xl font-semibold text-gray-900 mb-6">
                System Settings
              </h2>

              <div className="space-y-6">
                <div data-testid="dimensions-settings">
                  <DimensionConfig />
                </div>

                {settingsLoading ? (
                  <div className="text-center py-4 text-gray-500">Loading settings...</div>
                ) : (
                  <>
                <div data-testid="branding-settings">
                  <h3 className="text-lg font-medium text-gray-900 mb-4">
                    Company Branding
                  </h3>
                  <div className="space-y-4">
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        Company Name
                      </label>
                      <input
                        type="text"
                        data-testid="branding-company-name"
                        value={brandingCompanyName}
                        onChange={(e) => setBrandingCompanyName(e.target.value)}
                        className="w-full max-w-md px-4 py-2 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                        placeholder="Enter company name"
                        maxLength={100}
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        Company Logo
                      </label>
                      <div className="flex items-center gap-4">
                        {brandingLogoURL ? (
                          <div className="flex items-center gap-3" data-testid="branding-logo-preview">
                            <img
                              src={brandingLogoURL}
                              alt="Company logo"
                              className="w-12 h-12 object-contain rounded border border-gray-200"
                            />
                            <button
                              type="button"
                              data-testid="branding-logo-remove"
                              onClick={() => setBrandingLogoURL("")}
                              className="text-sm text-red-600 hover:text-red-800"
                            >
                              Remove
                            </button>
                          </div>
                        ) : (
                          <div className="w-12 h-12 bg-gray-100 rounded border border-gray-200 flex items-center justify-center">
                            <Building2 className="w-6 h-6 text-gray-400" />
                          </div>
                        )}
                        <label
                          data-testid="branding-logo-upload"
                          className="cursor-pointer px-4 py-2 text-sm text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 transition-colors"
                        >
                          {brandingLogoURL ? "Change Logo" : "Upload Logo"}
                          <input
                            type="file"
                            accept="image/*"
                            className="hidden"
                            onChange={handleLogoUpload}
                          />
                        </label>
                        <span className="text-xs text-gray-500">Max 500KB</span>
                      </div>
                    </div>
                  </div>
                </div>

                {!smtpConfigured && (
                  <div className="flex items-start gap-3 p-4 bg-amber-50 border border-amber-200 rounded-lg">
                    <AlertCircle className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
                    <div>
                      <p className="text-sm font-medium text-amber-800">SMTP Not Configured</p>
                      <p className="text-sm text-amber-700 mt-1">
                        Email notifications are disabled. Set <code className="bg-amber-100 px-1 rounded">SMTP_HOST</code> and related environment variables to enable email delivery.
                      </p>
                    </div>
                  </div>
                )}

                <div data-testid="team-dl-settings">
                  <h3 className="text-lg font-medium text-gray-900 mb-4">
                    Team Distribution Lists
                  </h3>
                  <p className="text-sm text-gray-500 mb-3">
                    Teams with a distribution list email will receive post-workshop survey summaries.
                  </p>
                  <div className="border rounded-lg overflow-hidden">
                    <table className="w-full text-sm">
                      <thead className="bg-gray-50">
                        <tr>
                          <th className="text-left px-4 py-2 font-medium text-gray-600">Team</th>
                          <th className="text-left px-4 py-2 font-medium text-gray-600">Distribution List Email</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y">
                        {teams.length === 0 ? (
                          <tr><td colSpan={2} className="px-4 py-3 text-gray-400 text-center">No teams configured</td></tr>
                        ) : teams.map((t) => (
                          <tr key={t.id}>
                            <td className="px-4 py-2 font-medium text-gray-900">{t.name}</td>
                            <td className="px-4 py-2">
                              {t.distributionListEmail ? (
                                <span className="text-gray-700">{t.distributionListEmail}</span>
                              ) : (
                                <span className="text-gray-400">Not set</span>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div data-testid="notifications-settings">
                <h3 className="text-lg font-medium text-gray-900 mb-4">
                  Notification Settings
                </h3>
                <div className="space-y-4">
                  <label className="flex items-center gap-3">
                    <input
                      type="checkbox"
                      className="w-4 h-4 text-indigo-600 rounded"
                      data-testid="email-enabled-checkbox"
                      checked={emailEnabled}
                      onChange={(e) => setEmailEnabled(e.target.checked)}
                    />
                    <span className="text-gray-700">Send email reminders for upcoming health checks</span>
                  </label>
                  <label className="flex items-center gap-3">
                    <input
                      type="checkbox"
                      className="w-4 h-4 text-indigo-600 rounded"
                      data-testid="slack-enabled-checkbox"
                      checked={slackEnabled}
                      onChange={(e) => setSlackEnabled(e.target.checked)}
                    />
                    <span className="text-gray-700">Notify managers when team health declines</span>
                  </label>
                  <label className="flex items-center gap-3">
                    <input
                      type="checkbox"
                      className="w-4 h-4 text-indigo-600 rounded"
                      data-testid="weekly-digest-checkbox"
                      checked={notifyOnSubmission}
                      onChange={(e) => setNotifyOnSubmission(e.target.checked)}
                    />
                    <span className="text-gray-700">Send weekly summary reports</span>
                  </label>
                </div>
              </div>

              <div data-testid="retention-settings">
                <h3 className="text-lg font-medium text-gray-900 mb-4">
                  Data Retention Policy
                </h3>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Keep health check data for
                    </label>
                    <select
                      className="w-full px-4 py-2 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                      data-testid="retention-months-select"
                      value={retentionMonths}
                      onChange={(e) => setRetentionMonths(Number(e.target.value))}
                    >
                      <option value={6}>6 months</option>
                      <option value={12}>1 year</option>
                      <option value={24}>2 years</option>
                      <option value={120}>Forever</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Export format
                    </label>
                    <select className="w-full px-4 py-2 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent">
                      <option>CSV</option>
                      <option>JSON</option>
                      <option>Excel</option>
                    </select>
                  </div>
                </div>
              </div>

              {settingsError && (
                <div className="text-red-600 text-sm" data-testid="settings-error">{settingsError}</div>
              )}
              {settingsSuccess && (
                <div className="text-green-600 text-sm" data-testid="settings-success">Settings saved successfully</div>
              )}

              <div className="flex gap-4 pt-6 border-t">
                <button
                  onClick={saveSettings}
                  disabled={settingsSaving}
                  data-testid="save-settings-btn"
                  className="px-6 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50"
                >
                  {settingsSaving ? "Saving..." : "Save Settings"}
                </button>
                <button
                  onClick={loadSettings}
                  className="px-6 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300 transition-colors"
                >
                  Cancel
                </button>
              </div>
                  </>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
