// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

// Configurable hierarchy level
export interface HierarchyLevel {
  id: string;
  name: string; // e.g., "Vice President", "Director", "Senior Manager"
  level: number; // 1 is highest (top of org), higher numbers are lower in hierarchy
  color: string; // For UI visualization
  permissions: {
    canViewAllTeams: boolean;
    canEditTeams: boolean;
    canManageUsers: boolean;
    canConfigureSystem: boolean;
    canViewReports: boolean;
    canExportData: boolean;
  };
}

export interface OrganizationConfig {
  id: string;
  companyName: string;
  hierarchyLevels: HierarchyLevel[];
  teamMemberLevelId: string; // ID of the level for regular team members
  createdAt: string;
  updatedAt: string;
}

export interface User {
  id: string;
  username: string;
  password: string;
  name: string;
  email?: string;
  hierarchyLevelId?: string; // References HierarchyLevel.id
  reportsTo?: string; // User ID of direct supervisor
  teamIds: string[]; // Can belong to multiple teams
  isAdmin?: boolean;
}

export interface Team {
  id: string;
  name: string;
  cadence: 'monthly' | 'quarterly' | 'half-yearly' | 'yearly';
  nextCheckDate: string;
  members: string[];
  supervisorChain: { // Chain of supervisors at different levels
    userId: string;
    levelId: string;
  }[];
  department?: string;
  division?: string;
  tags?: string[];
}

export interface OrganizationNode {
  id: string;
  user: User;
  level: HierarchyLevel;
  children: OrganizationNode[];
  teams: Team[];
  metrics?: {
    avgHealth: number;
    totalTeams: number;
    totalMembers: number;
    completionRate: number;
    trends: {
      improving: number;
      stable: number;
      declining: number;
    };
    dimensionScores: Map<string, number>;
  };
}

export interface HealthDimension {
  id: string;
  name: string;
  description: string;
  goodDescription: string;
  badDescription: string;
  isActive?: boolean; // Allow enabling/disabling dimensions
  weight?: number; // For weighted scoring
}

export interface HealthCheckResponse {
  dimensionId: string;
  score: 1 | 2 | 3; // 1 = red, 2 = yellow, 3 = green
  trend: 'improving' | 'stable' | 'declining';
  comment?: string;
}

export interface HealthCheckSession {
  id: string;
  teamId: string;
  userId: string;
  date: string;
  assessmentPeriod?: string; // e.g., "2024 - 1st Half"
  responses: HealthCheckResponse[];
  completed: boolean;
  createdAt?: string; // Timestamp from backend
}

export interface TeamHealthSummary {
  teamId: string;
  teamName: string;
  date: string;
  supervisorChain: {
    userId: string;
    userName: string;
    levelId: string;
    levelName: string;
  }[];
  dimensions: {
    dimensionId: string;
    name: string;
    averageScore: number;
    distribution: {
      red: number;
      yellow: number;
      green: number;
    };
    trend: 'improving' | 'stable' | 'declining';
  }[];
  overallScore?: number;
  participationRate?: number;
}

export interface HierarchicalSummary {
  nodeId: string;
  userName: string;
  levelId: string;
  levelName: string;
  directReports: number;
  totalTeams: number;
  totalMembers: number;
  healthMetrics: {
    overall: number;
    byDimension: {
      [dimensionId: string]: {
        score: number;
        trend: 'improving' | 'stable' | 'declining';
      };
    };
    participation: number;
    lastUpdated: string;
  };
  drillDownPath?: string[]; // Path of user IDs to drill down
  children?: HierarchicalSummary[];
  teams?: TeamHealthSummary[];
}