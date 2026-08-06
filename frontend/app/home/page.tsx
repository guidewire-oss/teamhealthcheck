'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { getCurrentUser, logout, authenticatedFetch } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { API_BASE_URL } from '@/lib/api/client';
import { getOrgConfig, getHierarchyLevel } from '@/lib/org-config';
// Assessment period import removed — period is team-specific, computed on survey page
import { LogOut, Building2, ChevronDown, ClipboardList, TrendingUp, Calendar, Clock, CalendarClock } from 'lucide-react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar } from 'recharts';
import { getTeamInfoCached, TeamInfo } from '@/lib/api/teams';
import OnboardingModal from '@/components/OnboardingModal';

function getNextSurveyDate(lastSurveyDate: string, cadence: string): Date {
  const last = new Date(lastSurveyDate);
  const next = new Date(last);
  switch (cadence) {
    case 'weekly':
      next.setDate(last.getDate() + 7);
      break;
    case 'biweekly':
      next.setDate(last.getDate() + 14);
      break;
    case 'monthly':
      next.setMonth(last.getMonth() + 1);
      break;
    case 'quarterly':
      next.setMonth(last.getMonth() + 3);
      break;
    case 'half-yearly':
      next.setMonth(last.getMonth() + 6);
      break;
    case 'yearly':
      next.setFullYear(last.getFullYear() + 1);
      break;
    default:
      next.setMonth(last.getMonth() + 1);
  }
  return next;
}

function formatRelativeDate(date: Date): string {
  const now = new Date();
  // Normalize both to start-of-day to compare calendar days, not timestamps
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const dateStart = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffMs = dateStart.getTime() - todayStart.getTime();
  const diffDays = Math.round(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays < 0) return 'Overdue';
  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Tomorrow';
  if (diffDays <= 7) return `In ${diffDays} days`;
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

interface SurveyHistoryEntry {
  sessionId: string;
  teamId: string;
  teamName: string;
  assessmentPeriod: string;
  date: string;
  completed: boolean;
  responses: {
    dimensionId: string;
    dimensionName: string;
    score: number;
    trend: string;
    comment: string;
  }[];
}

interface TrendDataPoint {
  period: string;
  [key: string]: string | number;
}

export default function MemberHomePage() {
  const router = useRouter();
  const [user, setUser] = useState<any>(null);
  const [showUserInfo, setShowUserInfo] = useState(false);
  const [surveyHistory, setSurveyHistory] = useState<SurveyHistoryEntry[]>([]);
  const [trendData, setTrendData] = useState<TrendDataPoint[]>([]);
  const [team, setTeam] = useState<TeamInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [brandingName, setBrandingName] = useState<string>('');
  const [brandingLogo, setBrandingLogo] = useState<string | null>(null);
  const [showOnboarding, setShowOnboarding] = useState(false);

  const currentPeriod = ''; // Period is team-specific; shown on survey page after team selection

  useEffect(() => {
    const currentUser = getCurrentUser();
    if (!currentUser) {
      router.push('/login');
      return;
    }
    setUser(currentUser);
    if (!localStorage.getItem(`onboarding_complete:${currentUser.id}`)) {
      setShowOnboarding(true);
    }
    fetchSurveyHistory(currentUser.id);

    fetch(`${API_BASE_URL}/api/v1/config`)
      .then(res => res.json())
      .then(data => {
        if (data.companyName) setBrandingName(data.companyName);
        if (data.logoURL) setBrandingLogo(data.logoURL);
      })
      .catch(() => {});

    // Fetch team info for cadence
    const teamId = currentUser.teamIds && currentUser.teamIds.length > 0 ? currentUser.teamIds[0] : null;
    if (teamId) {
      getTeamInfoCached(teamId).then(setTeam).catch(() => {});
    }
  }, [router]);

  const fetchSurveyHistory = async (userId: string) => {
    try {
      setLoading(true);
      console.log('[MemberHome] Fetching survey history for user:', userId);
      const response = await authenticatedFetch(`${API_BASE_URL}/api/v1/users/${userId}/survey-history`);
      console.log('[MemberHome] Response status:', response.status, response.ok);
      if (response.ok) {
        const data = await response.json();
        console.log('[MemberHome] Survey history data:', JSON.stringify(data).substring(0, 200));
        console.log('[MemberHome] surveyHistory array length:', data.surveyHistory?.length);
        const historyData = data.surveyHistory || [];
        console.log('[MemberHome] Setting surveyHistory state with', historyData.length, 'entries');
        setSurveyHistory(historyData);
        console.log('[MemberHome] surveyHistory state set successfully');

        // Transform history into trend data for chart
        try {
          if (historyData.length > 0) {
            const trendMap = new Map<string, TrendDataPoint>();

            historyData.forEach((entry: SurveyHistoryEntry) => {
              if (!trendMap.has(entry.assessmentPeriod)) {
                trendMap.set(entry.assessmentPeriod, { period: entry.assessmentPeriod });
              }
              const point = trendMap.get(entry.assessmentPeriod)!;

              entry.responses.forEach((r) => {
                // Use dimension name as key
                point[r.dimensionName] = r.score;
              });
            });

            // Sort by period and convert to array
            const sortedTrend = Array.from(trendMap.values()).sort((a, b) =>
              a.period.localeCompare(b.period)
            );
            setTrendData(sortedTrend);
            console.log('[MemberHome] Trend data set with', sortedTrend.length, 'periods');
          }
        } catch (trendError) {
          console.error('[MemberHome] Error transforming trend data:', trendError);
        }
      } else {
        console.log('[MemberHome] Response not OK:', response.status);
      }
    } catch (error) {
      console.error('[MemberHome] Failed to fetch survey history:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    router.push('/login');
  };

  const handleTakeSurvey = () => {
    router.push('/survey');
  };

  const getUserLevelName = () => {
    if (!user) return '';
    const orgConfig = getOrgConfig();
    const level = getHierarchyLevel(user.hierarchyLevel || user.hierarchyLevelId);
    return level?.name || 'Team Member';
  };

  // Get the most recent survey for summary
  const latestSurvey = surveyHistory.length > 0 ? surveyHistory[0] : null;

  // Prepare radar chart data from latest survey
  const radarData = latestSurvey?.responses.map(r => ({
    dimension: r.dimensionName,
    score: r.score,
    fullMark: 3,
  })) || [];

  // Colors for trend lines
  const dimensionColors = [
    '#3b82f6', '#ef4444', '#22c55e', '#f59e0b', '#8b5cf6',
    '#06b6d4', '#ec4899', '#84cc16', '#f97316', '#6366f1', '#14b8a6'
  ];

  if (!user) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header - consistent with other dashboards */}
      <header className="bg-white shadow-sm border-b border-gray-200">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="flex justify-between items-center">
            <div className="flex items-center space-x-3">
              {brandingLogo ? (
                <img src={brandingLogo} alt="Company logo" className="w-8 h-8 object-contain rounded" />
              ) : (
                <Building2 className="h-8 w-8 text-blue-600" />
              )}
              <div>
                <h1 className="text-xl font-bold text-gray-900">{brandingName || 'Team360'}</h1>
                <p className="text-sm text-gray-500">Member Home</p>
              </div>
            </div>
            <div className="relative">
              <button
                data-testid="user-menu-button"
                onClick={() => setShowUserInfo(!showUserInfo)}
                className="flex items-center space-x-2 text-gray-700 hover:text-gray-900"
              >
                <div className="w-8 h-8 bg-blue-100 rounded-full flex items-center justify-center">
                  <span className="text-blue-600 font-medium text-sm">
                    {user.fullName?.charAt(0) || user.username?.charAt(0) || 'U'}
                  </span>
                </div>
                <span className="text-sm font-medium">{user.fullName || user.username}</span>
                <ChevronDown className="h-4 w-4" />
              </button>
              {showUserInfo && (
                <div className="absolute right-0 mt-2 w-64 bg-white rounded-lg shadow-lg border border-gray-200 py-2 z-50">
                  <div className="px-4 py-2 border-b border-gray-100">
                    <p className="text-sm font-medium text-gray-900">{user.fullName || user.username}</p>
                    <p className="text-xs text-gray-500">{getUserLevelName()}</p>
                  </div>
                  <button
                    data-testid="logout-button"
                    onClick={handleLogout}
                    className="w-full px-4 py-2 text-left text-sm text-red-600 hover:bg-red-50 flex items-center space-x-2"
                  >
                    <LogOut className="h-4 w-4" />
                    <span>Logout</span>
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Welcome Section */}
        <div data-testid="welcome-message" className="mb-8">
          <h2 className="text-2xl font-bold text-gray-900">Welcome back, {user.fullName || user.username}!</h2>
          <p className="text-gray-600 mt-1">Track your team health check progress and insights.</p>
        </div>

        {/* Current Period CTA */}
        <div className="bg-gradient-to-r from-blue-600 to-blue-700 rounded-xl p-6 mb-8 text-white">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <Calendar className="h-5 w-5" />
                <span data-testid="current-period" className="text-sm font-medium opacity-90">Current Period</span>
              </div>
              <h3 className="text-xl font-bold mb-2">Ready to share your feedback?</h3>
              <p className="text-blue-100">Your input helps the team improve. Take a few minutes to complete the health check.</p>
            </div>
            <button
              data-testid="take-survey-btn"
              onClick={handleTakeSurvey}
              className="bg-white text-blue-600 px-6 py-3 rounded-lg font-semibold hover:bg-blue-50 transition-colors flex items-center space-x-2"
            >
              <ClipboardList className="h-5 w-5" />
              <span>Take Survey</span>
            </button>
          </div>
        </div>

        {/* Survey Schedule Info */}
        {!loading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
            <div data-testid="last-survey-info" className="bg-white rounded-xl shadow-sm border border-gray-200 p-5 flex items-start space-x-4">
              <div className="w-10 h-10 bg-gray-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <Clock className="h-5 w-5 text-gray-600" />
              </div>
              <div>
                <p className="text-sm font-medium text-gray-500">Last Survey</p>
                {latestSurvey ? (
                  <>
                    <p className="text-lg font-semibold text-gray-900">
                      {new Date(latestSurvey.date).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
                    </p>
                    <p className="text-xs text-gray-400 mt-0.5">{latestSurvey.assessmentPeriod}</p>
                  </>
                ) : (
                  <p className="text-lg font-semibold text-gray-400">No surveys yet</p>
                )}
              </div>
            </div>
            <div data-testid="next-survey-info" className="bg-white rounded-xl shadow-sm border border-gray-200 p-5 flex items-start space-x-4">
              <div className="w-10 h-10 bg-blue-50 rounded-lg flex items-center justify-center flex-shrink-0">
                <CalendarClock className="h-5 w-5 text-blue-600" />
              </div>
              <div>
                <p className="text-sm font-medium text-gray-500">Next Survey</p>
                {latestSurvey && team ? (
                  (() => {
                    const nextDate = getNextSurveyDate(latestSurvey.date, team.cadence);
                    const isOverdue = nextDate.getTime() < Date.now();
                    return (
                      <>
                        <p className={`text-lg font-semibold ${isOverdue ? 'text-red-600' : 'text-gray-900'}`}>
                          {nextDate.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
                        </p>
                        <p className={`text-xs mt-0.5 ${isOverdue ? 'text-red-400' : 'text-gray-400'}`}>
                          {formatRelativeDate(nextDate)} · {team.cadence.charAt(0).toUpperCase() + team.cadence.slice(1)} cadence
                        </p>
                      </>
                    );
                  })()
                ) : (
                  <p className="text-lg font-semibold text-gray-400">
                    {team ? 'Complete your first survey' : 'Loading...'}
                  </p>
                )}
              </div>
            </div>
          </div>
        )}

        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          </div>
        ) : surveyHistory.length === 0 ? (
          /* Empty State */
          <div data-testid="empty-state" className="text-center py-12 bg-white rounded-xl shadow-sm border border-gray-200">
            <ClipboardList className="h-16 w-16 text-gray-300 mx-auto mb-4" />
            <h3 className="text-lg font-medium text-gray-900 mb-2">No surveys yet</h3>
            <p className="text-gray-500 mb-6">Complete your first survey to start tracking your team&apos;s health.</p>
            <button
              onClick={handleTakeSurvey}
              className="bg-blue-600 text-white px-6 py-3 rounded-lg font-semibold hover:bg-blue-700 transition-colors inline-flex items-center space-x-2"
            >
              <ClipboardList className="h-5 w-5" />
              <span>Get Started</span>
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* Latest Survey Summary - Radar Chart */}
            {latestSurvey && (
              <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-lg font-semibold text-gray-900">Latest Survey Summary</h3>
                  <span className="text-sm text-gray-500">{latestSurvey.assessmentPeriod}</span>
                </div>
                <div data-testid="health-chart" className="h-80">
                  <ResponsiveContainer width="100%" height="100%">
                    <RadarChart data={radarData}>
                      <PolarGrid />
                      <PolarAngleAxis dataKey="dimension" tick={{ fontSize: 10 }} />
                      <PolarRadiusAxis domain={[0, 3]} tickCount={4} />
                      <Radar
                        name="Score"
                        dataKey="score"
                        stroke="#3b82f6"
                        fill="#3b82f6"
                        fillOpacity={0.5}
                      />
                      <Tooltip />
                    </RadarChart>
                  </ResponsiveContainer>
                </div>
              </div>
            )}

            {/* Trend Chart */}
            {trendData.length > 1 && (
              <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                <div className="flex items-center space-x-2 mb-4">
                  <TrendingUp className="h-5 w-5 text-blue-600" />
                  <h3 className="text-lg font-semibold text-gray-900">Your Progress Over Time</h3>
                </div>
                <div className="h-80">
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={trendData}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="period" tick={{ fontSize: 10 }} />
                      <YAxis domain={[0, 3]} tickCount={4} />
                      <Tooltip />
                      <Legend wrapperStyle={{ fontSize: '10px' }} />
                      {HEALTH_DIMENSIONS.map((dim, index) => (
                        <Line
                          key={dim.id}
                          type="monotone"
                          dataKey={dim.name}
                          stroke={dimensionColors[index % dimensionColors.length]}
                          strokeWidth={2}
                          dot={{ r: 3 }}
                        />
                      ))}
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              </div>
            )}

            {/* Survey History Table */}
            <div data-testid="survey-history" className="bg-white rounded-xl shadow-sm border border-gray-200 p-6 lg:col-span-2">
              <h3 className="text-lg font-semibold text-gray-900 mb-4">Survey History</h3>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="text-left text-sm text-gray-500 border-b border-gray-200">
                      <th className="pb-3 font-medium">Assessment Period</th>
                      <th className="pb-3 font-medium">Team</th>
                      <th className="pb-3 font-medium">Date</th>
                      <th className="pb-3 font-medium">Status</th>
                      <th className="pb-3 font-medium">Avg Score</th>
                    </tr>
                  </thead>
                  <tbody>
                    {surveyHistory.map((entry) => {
                      const avgScore = entry.responses.length > 0
                        ? (entry.responses.reduce((sum, r) => sum + r.score, 0) / entry.responses.length).toFixed(1)
                        : '-';
                      return (
                        <tr key={entry.sessionId} data-testid="history-entry" className="border-b border-gray-100 last:border-0">
                          <td className="py-3 text-sm text-gray-900">{entry.assessmentPeriod}</td>
                          <td className="py-3 text-sm text-gray-600">{entry.teamName}</td>
                          <td className="py-3 text-sm text-gray-600">{new Date(entry.date).toLocaleDateString()}</td>
                          <td className="py-3">
                            <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                              entry.completed ? 'bg-green-100 text-green-800' : 'bg-yellow-100 text-yellow-800'
                            }`}>
                              {entry.completed ? 'Completed' : 'In Progress'}
                            </span>
                          </td>
                          <td className="py-3">
                            <span className={`inline-flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium ${
                              parseFloat(avgScore) >= 2.5 ? 'bg-green-100 text-green-800' :
                              parseFloat(avgScore) >= 1.5 ? 'bg-yellow-100 text-yellow-800' :
                              'bg-red-100 text-red-800'
                            }`}>
                              {avgScore}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}
      </main>
      {showOnboarding && user && (
        <OnboardingModal
          userLevel={user.hierarchyLevelId}
          onDismiss={() => {
            localStorage.setItem(`onboarding_complete:${user.id}`, 'true');
            setShowOnboarding(false);
          }}
        />
      )}
    </div>
  );
}
