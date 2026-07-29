// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * API Response Types
 * TypeScript interfaces matching backend DTOs for type-safe API integration.
 * These types correspond to the Go structs in backend/interfaces/dto/
 */

// =============================================================================
// Authentication API Types (from auth_dto.go)
// =============================================================================

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  user: UserDTO;
}

export interface UserDTO {
  id: string;
  username: string;
  email: string;
  fullName: string;
  hierarchyLevel: string;
  teamIds: string[];
}

// =============================================================================
// Team Dashboard API Types (from team_dashboard_dto.go)
// =============================================================================

export interface TeamDashboardHealthSummary {
  teamId: string;
  teamName: string;
  assessmentPeriod?: string;
  dimensions: DimensionSummary[];
  overallHealth: number;
  submissionCount: number;
}

export interface ResponseDistribution {
  teamId: string;
  distribution: DimensionDistribution[];
}

export interface DimensionDistribution {
  dimensionId: string;
  red: number;
  yellow: number;
  green: number;
}

export interface IndividualResponses {
  teamId: string;
  responses: IndividualUserResponse[];
}

export interface IndividualUserResponse {
  sessionId: string;
  userId: string;
  userName: string;
  date: string;
  dimensions: IndividualDimensionResp[];
}

export interface IndividualDimensionResp {
  dimensionId: string;
  score: number;
  trend: string;
  comment: string;
}

export interface TrendData {
  teamId: string;
  periods: string[];
  dimensions: DimensionTrend[];
}

export interface DimensionTrend {
  dimensionId: string;
  scores: number[];
}

// =============================================================================
// Manager Dashboard API Types (from manager_dto.go)
// =============================================================================

export interface TeamHealthSummary {
  teamId: string;
  teamName: string;
  overallHealth: number;
  submissionCount: number;
  dimensions: DimensionSummary[];
  postWorkshopStatus?: 'submitted' | 'pending';
}

export interface DimensionSummary {
  dimensionId: string;
  avgScore: number;
  responseCount: number;
}

// =============================================================================
// Manager Endpoint Response Types
// =============================================================================

export interface ManagerTeamsHealthResponse {
  managerId: string;
  teams: TeamHealthSummary[];
  totalTeams: number;
  assessmentPeriod?: string;
}

export interface ManagerAggregatedRadarResponse {
  managerId: string;
  dimensions: DimensionSummary[];
  assessmentPeriod?: string;
}

export interface ManagerTrendsResponse {
  managerId: string;
  periods: string[];
  dimensions: DimensionTrend[];
}

// =============================================================================
// Common API Types
// =============================================================================

export interface ErrorResponse {
  error: string;
  message?: string;
}

export interface HealthCheckDimension {
  id: string;
  name: string;
  description: string;
  goodDescription: string;
  badDescription: string;
  isActive: boolean;
  weight: number;
}

export interface HealthCheckSubmission {
  teamId: string;
  responses: HealthCheckResponseItem[];
}

export interface HealthCheckResponseItem {
  dimensionId: string;
  score: number;
  trend: string;
  comment?: string;
}

// =============================================================================
// Team API Types
// =============================================================================

export interface TeamListResponse {
  teams: TeamInfo[];
}

export interface TeamInfo {
  id: string;
  name: string;
  description?: string;
  memberCount: number;
}

export interface TeamSessionsResponse {
  teamId: string;
  sessions: SessionSummary[];
}

export interface SessionSummary {
  id: string;
  userId: string;
  date: string;
  assessmentPeriod: string;
  surveyType?: 'individual' | 'post_workshop';
  completed: boolean;
}

export interface TeamSubmissionStatus {
  teamId: string;
  assessmentPeriod: string;
  totalMembers: number;
  submittedMembers: number;
  allSubmitted: boolean;
  postWorkshopExists: boolean;
}
