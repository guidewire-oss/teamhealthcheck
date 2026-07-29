// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

'use client';

import { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { AlertCircle, Calendar, Users, TrendingUp, TrendingDown, Minus } from 'lucide-react';
import { authenticatedFetch } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { API_BASE_URL } from '@/lib/api/client';

// Types matching backend API response
interface DimensionScore {
  dimensionId: string;
  avgScore: number;
  responseCount: number;
}

interface HealthSession {
  sessionId: string;
  userId: string;
  userName: string;
  submittedAt: string;
  dimensions: DimensionScore[];
}

interface TeamResultsResponse {
  teamId: string;
  teamName: string;
  sessions: HealthSession[];
  aggregateScores: DimensionScore[];
  totalSessions: number;
}

export default function TeamResultsPage() {
  const router = useRouter();
  const { teamId } = useParams<{ teamId: string }>();
  const [teamData, setTeamData] = useState<TeamResultsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (teamId) {
      fetchTeamResults(teamId);
    }
  }, [teamId]);

  const fetchTeamResults = async (teamId: string) => {
    setLoading(true);
    setError(null);

    try {
      const response = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}`);

      if (!response.ok) {
        throw new Error(`Failed to fetch team results: ${response.statusText}`);
      }

      const data: TeamResultsResponse = await response.json();
      setTeamData(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred');
      console.error('Error fetching team results:', err);
    } finally {
      setLoading(false);
    }
  };

  const getHealthColor = (score: number) => {
    const percentage = (score / 3) * 100;
    if (percentage >= 66) return 'bg-green-100 text-green-800 border-green-300';
    if (percentage >= 33) return 'bg-yellow-100 text-yellow-800 border-yellow-300';
    return 'bg-red-100 text-red-800 border-red-300';
  };

  const formatHealthScore = (score: number) => {
    return score.toFixed(1);
  };

  const getDimensionName = (dimensionId: string) => {
    const dimension = HEALTH_DIMENSIONS.find(d => d.id === dimensionId);
    return dimension?.name || dimensionId;
  };

  const getTrendIcon = (score: number) => {
    const percentage = (score / 3) * 100;
    if (percentage >= 66) return <TrendingUp className="w-4 h-4 text-green-600" />;
    if (percentage >= 33) return <Minus className="w-4 h-4 text-yellow-600" />;
    return <TrendingDown className="w-4 h-4 text-red-600" />;
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading team results...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center p-4">
        <div className="bg-red-50 border border-red-300 rounded-xl p-6 max-w-lg flex items-start gap-4">
          <AlertCircle className="w-8 h-8 text-red-600 flex-shrink-0" />
          <div>
            <h3 className="font-semibold text-red-900 mb-2">Error Loading Team Results</h3>
            <p className="text-red-700">{error}</p>
            <button
              onClick={() => router.back()}
              className="mt-4 px-4 py-2 bg-red-100 text-red-700 rounded-lg hover:bg-red-200 transition-colors"
            >
              Go Back
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (!teamData) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <Users className="w-16 h-16 text-gray-400 mx-auto mb-4" />
          <h3 className="text-xl font-semibold text-gray-900 mb-2">Team Not Found</h3>
          <p className="text-gray-600 mb-4">The requested team could not be found.</p>
          <button
            onClick={() => router.back()}
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
          >
            Go Back
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <div className="bg-white shadow-sm border-b">
        <div className="container mx-auto px-4 py-6">
          <div className="flex justify-between items-center">
            <div>
              <h1 className="text-3xl font-bold text-gray-900">{teamData.teamName}</h1>
              <p className="text-gray-500 mt-1">Health Check Results</p>
            </div>
            <button
              onClick={() => router.back()}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors"
            >
              Back
            </button>
          </div>
        </div>
      </div>

      <div className="container mx-auto px-4 py-8">
        {/* Summary Stats */}
        <div className="bg-white rounded-xl shadow-sm border p-6 mb-6">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Overview</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="bg-indigo-50 rounded-lg p-4 border border-indigo-200">
              <div className="flex items-center gap-3">
                <Users className="w-8 h-8 text-indigo-600" />
                <div>
                  <p className="text-sm text-indigo-700">Total Submissions</p>
                  <p className="text-2xl font-bold text-indigo-900">{teamData.totalSessions}</p>
                </div>
              </div>
            </div>
            <div className="bg-blue-50 rounded-lg p-4 border border-blue-200">
              <div className="flex items-center gap-3">
                <Calendar className="w-8 h-8 text-blue-600" />
                <div>
                  <p className="text-sm text-blue-700">Dimensions Tracked</p>
                  <p className="text-2xl font-bold text-blue-900">{teamData.aggregateScores.length}</p>
                </div>
              </div>
            </div>
            <div className="bg-green-50 rounded-lg p-4 border border-green-200">
              <div className="flex items-center gap-3">
                <TrendingUp className="w-8 h-8 text-green-600" />
                <div>
                  <p className="text-sm text-green-700">Average Health</p>
                  <p className="text-2xl font-bold text-green-900">
                    {teamData.aggregateScores.length > 0
                      ? formatHealthScore(
                          teamData.aggregateScores.reduce((sum, d) => sum + d.avgScore, 0) /
                            teamData.aggregateScores.length
                        )
                      : 'N/A'}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Aggregate Dimension Scores */}
        <div className="bg-white rounded-xl shadow-sm border p-6 mb-6">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Aggregate Dimension Scores</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {teamData.aggregateScores.map((dimension) => (
              <div
                key={dimension.dimensionId}
                data-dimension={dimension.dimensionId}
                className="bg-gray-50 rounded-lg p-4 border hover:shadow-md transition-shadow"
              >
                <div className="flex justify-between items-start mb-2">
                  <div className="flex-1">
                    <h3 className="font-medium text-gray-900 mb-1">
                      {getDimensionName(dimension.dimensionId)}
                    </h3>
                    <p className="text-xs text-gray-500">
                      {dimension.responseCount} {dimension.responseCount === 1 ? 'response' : 'responses'}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    {getTrendIcon(dimension.avgScore)}
                    <span
                      data-display="score"
                      className={`text-xl font-bold px-3 py-1 rounded-lg border-2 ${getHealthColor(
                        dimension.avgScore
                      )}`}
                    >
                      {formatHealthScore(dimension.avgScore)}
                    </span>
                  </div>
                </div>
                <div className="mt-2 h-2 bg-gray-200 rounded-full overflow-hidden">
                  <div
                    className={`h-full transition-all ${
                      dimension.avgScore >= 2.5
                        ? 'bg-green-500'
                        : dimension.avgScore >= 1.5
                        ? 'bg-yellow-500'
                        : 'bg-red-500'
                    }`}
                    style={{ width: `${(dimension.avgScore / 3) * 100}%` }}
                  ></div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Individual Sessions */}
        <div className="bg-white rounded-xl shadow-sm border p-6">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Individual Submissions</h2>
          {teamData.sessions.length === 0 ? (
            <div className="text-center py-8">
              <Users className="w-12 h-12 text-gray-400 mx-auto mb-3" />
              <p className="text-gray-600">No submissions yet</p>
            </div>
          ) : (
            <div className="space-y-4">
              {teamData.sessions.map((session) => (
                <div
                  key={session.sessionId}
                  className="border rounded-lg p-4 hover:shadow-md transition-shadow"
                >
                  <div className="flex justify-between items-center mb-3">
                    <div>
                      <h3 className="font-semibold text-gray-900">
                        Session: <span className="text-indigo-600">{session.sessionId}</span>
                      </h3>
                      <p className="text-sm text-gray-600">
                        Submitted by: {session.userName} on{' '}
                        {new Date(session.submittedAt).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3 mt-3 pt-3 border-t">
                    {session.dimensions.map((dimension) => (
                      <div
                        key={dimension.dimensionId}
                        className="bg-gray-50 rounded p-2 text-center"
                      >
                        <p className="text-xs text-gray-600 mb-1">
                          {getDimensionName(dimension.dimensionId)}
                        </p>
                        <p
                          className={`text-lg font-bold px-2 py-1 rounded ${getHealthColor(
                            dimension.avgScore
                          )}`}
                        >
                          {formatHealthScore(dimension.avgScore)}
                        </p>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
