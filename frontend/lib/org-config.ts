// SPDX-License-Identifier: Apache-2.0

import { OrganizationConfig, HierarchyLevel, User, Team } from './types';

// Default configuration with VP -> Director -> Manager -> Team Lead -> Team Member hierarchy
export const DEFAULT_ORG_CONFIG: OrganizationConfig = {
  id: 'default',
  companyName: 'Tech Corp',
  hierarchyLevels: [
    {
      id: 'level-1',
      name: 'Vice President',
      level: 1,
      color: '#7C3AED', // Purple
      permissions: {
        canViewAllTeams: true,
        canEditTeams: true,
        canManageUsers: true,
        canConfigureSystem: true,
        canViewReports: true,
        canExportData: true
      }
    },
    {
      id: 'level-2',
      name: 'Director',
      level: 2,
      color: '#2563EB', // Blue
      permissions: {
        canViewAllTeams: true,
        canEditTeams: true,
        canManageUsers: true,
        canConfigureSystem: false,
        canViewReports: true,
        canExportData: true
      }
    },
    {
      id: 'level-3',
      name: 'Manager',
      level: 3,
      color: '#059669', // Green
      permissions: {
        canViewAllTeams: false,
        canEditTeams: true,
        canManageUsers: false,
        canConfigureSystem: false,
        canViewReports: true,
        canExportData: true
      }
    },
    {
      id: 'level-4',
      name: 'Team Lead',
      level: 4,
      color: '#EA580C', // Orange
      permissions: {
        canViewAllTeams: false,
        canEditTeams: false,
        canManageUsers: false,
        canConfigureSystem: false,
        canViewReports: true,
        canExportData: true
      }
    },
    {
      id: 'level-5',
      name: 'Team Member',
      level: 5,
      color: '#6B7280', // Gray
      permissions: {
        canViewAllTeams: false,
        canEditTeams: false,
        canManageUsers: false,
        canConfigureSystem: false,
        canViewReports: false,
        canExportData: false
      }
    }
  ],
  teamMemberLevelId: 'level-5',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString()
};

// Store organization config (in production, this would be in a database)
let currentConfig: OrganizationConfig = DEFAULT_ORG_CONFIG;

// Increment this when DEFAULT_ORG_CONFIG changes (e.g. permission updates) to
// force-replace stale localStorage copies.
const ORG_CONFIG_VERSION = 2;

// Initialize from localStorage if available
if (typeof window !== 'undefined') {
  const stored = localStorage.getItem('orgConfig');
  const storedVersion = parseInt(localStorage.getItem('orgConfigVersion') || '0', 10);
  if (stored && storedVersion >= ORG_CONFIG_VERSION) {
    try {
      currentConfig = JSON.parse(stored);
    } catch (e) {
      console.error('Failed to load org config:', e);
    }
  } else {
    // Stale or missing version — reset to updated defaults
    localStorage.removeItem('orgConfig');
    localStorage.setItem('orgConfigVersion', String(ORG_CONFIG_VERSION));
  }
}

export function getOrgConfig(): OrganizationConfig {
  return currentConfig;
}

export function saveOrgConfig(config: OrganizationConfig): void {
  currentConfig = config;
  currentConfig.updatedAt = new Date().toISOString();
  
  if (typeof window !== 'undefined') {
    localStorage.setItem('orgConfig', JSON.stringify(currentConfig));
  }
}

export function addHierarchyLevel(level: HierarchyLevel): void {
  const config = getOrgConfig();
  
  // Adjust levels if inserting in between
  const existingLevels = config.hierarchyLevels.filter(l => l.level >= level.level);
  existingLevels.forEach(l => l.level++);
  
  config.hierarchyLevels.push(level);
  config.hierarchyLevels.sort((a, b) => a.level - b.level);
  
  saveOrgConfig(config);
}

export function updateHierarchyLevel(levelId: string, updates: Partial<HierarchyLevel>): void {
  const config = getOrgConfig();
  const index = config.hierarchyLevels.findIndex(l => l.id === levelId);
  
  if (index !== -1) {
    config.hierarchyLevels[index] = {
      ...config.hierarchyLevels[index],
      ...updates
    };
    saveOrgConfig(config);
  }
}

export function deleteHierarchyLevel(levelId: string): boolean {
  const config = getOrgConfig();
  
  // Don't delete if it's the team member level
  if (levelId === config.teamMemberLevelId) {
    return false;
  }
  
  // Check if any users are assigned to this level
  // In production, this would check the database
  // For now, we'll allow deletion
  
  config.hierarchyLevels = config.hierarchyLevels.filter(l => l.id !== levelId);
  
  // Reorder levels
  config.hierarchyLevels.forEach((level, index) => {
    level.level = index + 1;
  });
  
  saveOrgConfig(config);
  return true;
}

export function getHierarchyLevel(levelId: string): HierarchyLevel | undefined {
  return getOrgConfig().hierarchyLevels.find(l => l.id === levelId);
}

export function getUserPermissions(user: User): HierarchyLevel['permissions'] {
  if (user.isAdmin) {
    return {
      canViewAllTeams: true,
      canEditTeams: true,
      canManageUsers: true,
      canConfigureSystem: true,
      canViewReports: true,
      canExportData: true
    };
  }
  
  const level = getHierarchyLevel(user.hierarchyLevelId || '');
  if (level) {
    return level.permissions;
  }
  
  // Default to most restrictive permissions
  return {
    canViewAllTeams: false,
    canEditTeams: false,
    canManageUsers: false,
    canConfigureSystem: false,
    canViewReports: false,
    canExportData: false
  };
}

export function canUserAccessTeam(user: User, team: Team): boolean {
  const permissions = getUserPermissions(user);
  
  if (permissions.canViewAllTeams) {
    return true;
  }
  
  // Check if user is in the team
  if (team.members.includes(user.id)) {
    return true;
  }
  
  // Check if user is in the supervisor chain
  if (team.supervisorChain.some(s => s.userId === user.id)) {
    return true;
  }
  
  return false;
}

export function getUsersAtLevel(levelId: string, users: User[]): User[] {
  return users.filter(u => u.hierarchyLevelId === levelId);
}

export function getSubordinates(userId: string, users: User[]): User[] {
  const subordinates: User[] = [];
  const directReports = users.filter(u => u.reportsTo === userId);
  
  for (const report of directReports) {
    subordinates.push(report);
    subordinates.push(...getSubordinates(report.id, users));
  }
  
  return subordinates;
}