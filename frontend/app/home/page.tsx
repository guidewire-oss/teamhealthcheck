'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { getCurrentUser, logout, authenticatedFetch } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { API_BASE_URL } from '@/lib/api/client';
import { getOrgConfig, getHierarchyLevel } from '@/lib/org-config';
import { getAssessmentPeriod, getSelectablePeriods, parseAssessmentPeriod, toCadence, formatPeriodLabel, formatMonthYear } from '@/lib/assessment-period';
import { LogOut, Building2, ChevronDown, ClipboardList, TrendingUp, Calendar, Clock, CalendarClock, X, AlertCircle } from 'lucide-react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar } from 'recharts';
import { getTeamInfoCached, TeamInfo } from '@/lib/api/teams';
import { checkSurveyEligibility } from '@/lib/api/health-checks';
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
  return formatMonthYear(date);
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
  /** User-facing H1/H2 label for `period`; never a quarter label. */
  label: string;
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
  const [assessmentPeriod, setAssessmentPeriod] = useState<string>('');
  const [autoAssessmentPeriod, setAutoAssessmentPeriod] = useState<string>('');
  // True while the pre-open eligibility check is in flight.
  const [checkingEligibility, setCheckingEligibility] = useState(false);
  // Set when the eligibility check finds the selected period ineligible; renders an info modal
  // instead of opening the survey. `reason` distinguishes an actual duplicate submission from
  // a period that simply isn't open right now (e.g. from a previous year) -- these must never
  // share the "you already submitted this" wording, since only "duplicate" means that.
  const [blockedInfo, setBlockedInfo] = useState<{
    reason: 'duplicate' | 'not_open';
    submittedPeriod: string;
    nextEligiblePeriod: string;
  } | null>(null);

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
      getTeamInfoCached(teamId).then((teamInfo) => {
        setTeam(teamInfo);
        const currentPeriod = getAssessmentPeriod(new Date(), toCadence(teamInfo.cadence));
        setAutoAssessmentPeriod(currentPeriod);
        setAssessmentPeriod(currentPeriod);
      }).catch(() => {});
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
                trendMap.set(entry.assessmentPeriod, {
                  period: entry.assessmentPeriod,
                  label: formatPeriodLabel(entry.assessmentPeriod),
                });
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

  const periodOptions = useMemo(() => {
    if (!team) return [];
    // getSelectablePeriods walks back several years of history for the dropdown, but the
    // backend only ever considers the current year's periods open for a new submission
    // (see healthcheck.CurrentlyOpenPeriods) -- anything from an earlier year is rejected as
    // "past_period" regardless of whether it was ever submitted. Offering those older periods
    // here just sets the Team Member up to pick something that can never succeed, so they are
    // filtered out before being shown.
    const currentYear = new Date().getFullYear();
    const options = getSelectablePeriods(toCadence(team.cadence)).filter((p) => {
      const parsed = parseAssessmentPeriod(p);
      return parsed ? parsed.year >= currentYear : true;
    });
    return options.includes(assessmentPeriod) ? options : [assessmentPeriod, ...options];
  }, [team, assessmentPeriod]);

  // Checks eligibility for the currently selected period (the "Select assessment period"
  // dropdown above, scoped to this Team Member's own submissions) and, in one click, either
  // opens the survey directly or shows the blocked-period info box -- there is no separate
  // confirmation step/modal in between. If the eligibility check itself fails (e.g. network
  // error), fail open and let the authoritative server-side check at submit time (409
  // Conflict) be the backstop.
  const handleTakeSurvey = async () => {
    if (!user || checkingEligibility) return;

    const query = assessmentPeriod ? `?period=${encodeURIComponent(assessmentPeriod)}` : '';

    setCheckingEligibility(true);
    try {
      const result = await checkSurveyEligibility({
        surveyType: 'individual',
        assessmentPeriod,
        userId: user.id,
      });

      if (!result.eligible) {
        // Only a "duplicate" reason means the Team Member already submitted this period --
        // any other reason (e.g. the period isn't open) must not reuse that wording, since
        // result.submittedPeriod is empty in that case and would otherwise be papered over
        // with the period they just picked, falsely implying they'd already submitted it.
        if (result.reason === 'duplicate') {
          setBlockedInfo({
            reason: 'duplicate',
            submittedPeriod: result.submittedPeriod || assessmentPeriod,
            nextEligiblePeriod: result.nextEligiblePeriod || '',
          });
        } else {
          setBlockedInfo({
            reason: 'not_open',
            submittedPeriod: assessmentPeriod,
            nextEligiblePeriod: result.nextEligiblePeriod || '',
          });
        }
        return;
      }

      router.push(`/survey${query}`);
    } catch {
      router.push(`/survey${query}`);
    } finally {
      setCheckingEligibility(false);
    }
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
          <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-6">
            <div className="flex-1 min-w-0">
              <div className="flex items-center space-x-2 mb-2">
                <Calendar className="h-5 w-5" />
                <span data-testid="current-period" className="text-sm font-medium opacity-90">
                  {autoAssessmentPeriod ? `Current Period: ${formatPeriodLabel(autoAssessmentPeriod)}` : 'Current Period'}
                </span>
              </div>
              <h3 className="text-xl font-bold mb-2">Ready to share your feedback?</h3>
              <p className="text-blue-100 mb-4">Your input helps the team improve. Take a few minutes to complete the health check.</p>

              {team && (
                <div
                  data-testid="assessment-period-panel"
                  className="bg-white/10 border border-white/30 rounded-lg p-4 max-w-sm"
                >
                  <h4 className="text-sm font-semibold text-white mb-3">Select assessment period</h4>
                  <label htmlFor="home-period-select" className="block text-xs font-medium text-blue-100 mb-1">
                    Assessment period
                  </label>
                  <div className="relative">
                    <select
                      id="home-period-select"
                      data-testid="assessment-period-select"
                      value={assessmentPeriod}
                      onChange={(e) => setAssessmentPeriod(e.target.value)}
                      className="w-full appearance-none pl-3 pr-8 py-2 text-sm font-medium bg-white text-gray-900 border border-white/60 rounded-lg shadow-sm focus:outline-none focus:ring-2 focus:ring-white/70 cursor-pointer"
                    >
                      {periodOptions.map((p) => (
                        <option key={p} value={p}>
                          {formatPeriodLabel(p)}{p === autoAssessmentPeriod ? ' (current)' : ''}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="w-4 h-4 text-gray-500 absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                  </div>
                  <p className="text-xs text-blue-100 mt-2">
                    The period is selected automatically based on today&apos;s date. Change it if you are completing a previous assessment.
                  </p>
                </div>
              )}
            </div>
            <button
              data-testid="take-survey-btn"
              onClick={handleTakeSurvey}
              disabled={checkingEligibility}
              className="bg-white text-blue-600 px-6 py-3 rounded-lg font-semibold hover:bg-blue-50 transition-colors flex items-center space-x-2 self-start flex-shrink-0 disabled:opacity-70 disabled:cursor-not-allowed"
            >
              <ClipboardList className="h-5 w-5" />
              <span>{checkingEligibility ? 'Checking...' : 'Take Survey'}</span>
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
                      {formatMonthYear(latestSurvey.date)}
                    </p>
                    <p className="text-xs text-gray-400 mt-0.5">{formatPeriodLabel(latestSurvey.assessmentPeriod)}</p>
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
                          {formatMonthYear(nextDate)}
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
              disabled={checkingEligibility}
              className="bg-blue-600 text-white px-6 py-3 rounded-lg font-semibold hover:bg-blue-700 transition-colors inline-flex items-center space-x-2 disabled:opacity-70 disabled:cursor-not-allowed"
            >
              <ClipboardList className="h-5 w-5" />
              <span>{checkingEligibility ? 'Checking...' : 'Get Started'}</span>
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* Latest Survey Summary - Radar Chart */}
            {latestSurvey && (
              <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-lg font-semibold text-gray-900">Latest Survey Summary</h3>
                  <span className="text-sm text-gray-500">{formatPeriodLabel(latestSurvey.assessmentPeriod)}</span>
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
                      <XAxis dataKey="label" tick={{ fontSize: 10 }} />
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
                          <td className="py-3 text-sm text-gray-900">{formatPeriodLabel(entry.assessmentPeriod)}</td>
                          <td className="py-3 text-sm text-gray-600">{entry.teamName}</td>
                          <td className="py-3 text-sm text-gray-600">{formatMonthYear(entry.date)}</td>
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
      {blockedInfo && (
        <div
          data-testid="duplicate-submission-modal"
          className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="duplicate-submission-modal-title"
            className="bg-white text-gray-900 rounded-2xl shadow-2xl border-2 border-amber-200 w-full sm:w-[600px] max-w-full p-6 sm:p-8"
          >
            <div className="flex justify-between items-start mb-4">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-amber-100 flex items-center justify-center flex-shrink-0">
                  <AlertCircle className="w-5 h-5 text-amber-600" />
                </div>
                <h3 id="duplicate-submission-modal-title" className="text-xl font-semibold text-gray-900">
                  {blockedInfo.reason === 'duplicate' ? 'Already Submitted' : 'Period Not Available'}
                </h3>
              </div>
              <button
                data-testid="duplicate-submission-close-icon"
                onClick={() => setBlockedInfo(null)}
                aria-label="Close"
                className="text-gray-400 hover:text-gray-600 rounded-full p-1 transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <div data-testid="duplicate-submission-message">
              {blockedInfo.reason === 'duplicate' ? (
                <>
                  <p className="text-base text-gray-700 leading-relaxed mb-2">
                    You have already submitted the{' '}
                    <span className="font-semibold">Individual Survey</span>{' '}
                    for <span className="font-semibold">{blockedInfo.submittedPeriod}</span>.
                  </p>
                  <p className="text-base text-gray-700 leading-relaxed mb-8">
                    Your next survey is scheduled for{' '}
                    <span className="font-semibold">{blockedInfo.nextEligiblePeriod}</span>.
                  </p>
                </>
              ) : (
                <p className="text-base text-gray-700 leading-relaxed mb-8">
                  <span className="font-semibold">{blockedInfo.submittedPeriod}</span> is not open for
                  submission right now. Please choose a different assessment period.
                </p>
              )}
            </div>
            <div className="flex justify-end">
              <button
                data-testid="duplicate-submission-close-button"
                onClick={() => setBlockedInfo(null)}
                className="px-6 py-3 text-base font-semibold whitespace-nowrap rounded-lg bg-gray-100 text-gray-700 transition-colors duration-150 hover:bg-gray-200 active:bg-gray-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 focus-visible:ring-offset-2"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
