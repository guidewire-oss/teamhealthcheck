'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { getCurrentUser, logout, authenticatedFetch } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { getOrgConfig, getHierarchyLevel, getUserPermissions } from '@/lib/org-config';
import { LogOut, Building2, ChevronDown, BarChart3, LineChart as LineChartIcon, Users as UsersIcon, Activity, ClipboardList, TrendingUp, TrendingDown, Minus, LayoutGrid, List, Info, CheckCircle, Download, ListTodo } from 'lucide-react';
import * as XLSX from 'xlsx';
import { AlertCircle } from 'lucide-react';
import { getTeamSubmissionStatus, getAssessmentPeriods, TeamSubmissionStatus } from '@/lib/api/health-checks';
import { API_BASE_URL } from '@/lib/api/client';
import { getAssessmentPeriod, toCadence } from '@/lib/assessment-period';
import { getTeamInfoCached } from '@/lib/api/teams';
import { RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, LineChart, Line, ResponsiveContainer } from 'recharts';
import OnboardingModal from '@/components/OnboardingModal';
import ActionItemsTab from '@/components/ActionItemsTab';

type TabType = 'radar' | 'distribution' | 'responses' | 'trends' | 'actions';

interface HealthSummary {
  dimension: string;
  averageScore: number;
}

interface ResponseDistribution {
  dimension: string;
  red: number;
  yellow: number;
  green: number;
}

interface IndividualResponse {
  userId: string;
  userName: string;
  sessionId: string;
  date: string;
  surveyType?: string;
  responses: {
    dimensionId: string;
    dimensionName: string;
    score: number;
    trend: string;
    comment: string;
  }[];
}

interface TrendData {
  period: string;
  [key: string]: string | number;
}

export default function DashboardPage() {
  const router = useRouter();
  const [user, setUser] = useState<any>(null);
  const [activeTab, setActiveTab] = useState<TabType>('radar');
  const [showUserInfo, setShowUserInfo] = useState(false);
  const [teamId, setTeamId] = useState<string>('');

  // Data states
  const [healthSummary, setHealthSummary] = useState<HealthSummary[]>([]);
  const [distribution, setDistribution] = useState<ResponseDistribution[]>([]);
  const [individualResponses, setIndividualResponses] = useState<IndividualResponse[]>([]);
  const [trends, setTrends] = useState<TrendData[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedPeriod, setSelectedPeriod] = useState<string>('');
  const [assessmentPeriodOptions, setAssessmentPeriodOptions] = useState<string[]>([]);
  const [submissionStatus, setSubmissionStatus] = useState<TeamSubmissionStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [responseView, setResponseView] = useState<'matrix' | 'cards'>('matrix');
  const [teamOptions, setTeamOptions] = useState<{id: string, name: string}[]>([]);
  const [teamMembers, setTeamMembers] = useState<{id: string, name: string}[]>([]);
  const [brandingName, setBrandingName] = useState<string>('');
  const [brandingLogo, setBrandingLogo] = useState<string | null>(null);
  const [collapsedCards, setCollapsedCards] = useState<Set<number>>(new Set());
  const [distributionView, setDistributionView] = useState<'chart' | 'breakdown'>('breakdown');
  const [trendsView, setTrendsView] = useState<'overview' | 'dimensions'>('dimensions');
  const [tooltip, setTooltip] = useState<{
    x: number;
    y: number;
    dimensionName: string;
    score: number;
    trend: string;
    comment: string;
  } | null>(null);
  const [showOnboarding, setShowOnboarding] = useState(false);

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

    // Fetch branding from public config endpoint
    fetch(`${API_BASE_URL}/api/v1/config`)
      .then(res => res.json())
      .then(data => {
        if (data.companyName) setBrandingName(data.companyName);
        if (data.logoURL) setBrandingLogo(data.logoURL);
      })
      .catch(() => {});

    // Get the team ID from user's first team
    if (currentUser.teamIds && currentUser.teamIds.length > 0) {
      const firstTeamId = currentUser.teamIds[0];
      setTeamId(firstTeamId);
      fetchDashboardData(firstTeamId, '');
      // Fetch team info for cadence, members, and submission status
      getTeamInfoCached(firstTeamId)
        .then((teamInfo) => {
          setTeamMembers(teamInfo.members.map(m => ({ id: m.id, name: m.fullName })));
          const currentPeriod = getAssessmentPeriod(new Date(), toCadence(teamInfo.cadence));
          return getTeamSubmissionStatus(firstTeamId, currentPeriod);
        })
        .catch(() => {
          // Fallback: use default cadence if team info fetch fails; clear stale members
          setTeamMembers([]);
          const currentPeriod = getAssessmentPeriod(new Date());
          return getTeamSubmissionStatus(firstTeamId, currentPeriod);
        })
        .then(setSubmissionStatus)
        .catch((err) => console.error('Failed to fetch submission status:', err));

      // Fetch team names for multi-team selector
      if (currentUser.teamIds.length > 1) {
        Promise.all(
          currentUser.teamIds.map((tid: string) =>
            getTeamInfoCached(tid).then(info => ({ id: info.id, name: info.name })).catch(() => ({ id: tid, name: tid }))
          )
        ).then(setTeamOptions);
      }
    } else {
      setLoading(false);
    }
  }, [router]);

  // Fetch assessment period options from database
  useEffect(() => {
    getAssessmentPeriods()
      .then(setAssessmentPeriodOptions)
      .catch((err) => console.error('Failed to fetch assessment periods:', err));
  }, []);

  const fetchDashboardData = async (teamId: string, assessmentPeriod: string) => {
    try {
      setLoading(true);
      setError(null);

      // Build query string for assessment period filter
      const periodQuery = assessmentPeriod ? `?assessmentPeriod=${encodeURIComponent(assessmentPeriod)}` : '';

      // Fetch health summary for radar chart
      const healthRes = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/dashboard/health-summary${periodQuery}`);
      if (healthRes.ok) {
        const data = await healthRes.json();
        // Transform backend format to frontend format
        // Backend: { dimensions: [{ dimensionId, avgScore, responseCount }] }
        // Frontend: [{ dimension, averageScore }]
        if (data.dimensions && Array.isArray(data.dimensions)) {
          const transformed = data.dimensions.map((d: { dimensionId: string; avgScore: number }) => {
            // Find dimension name from HEALTH_DIMENSIONS
            const dimInfo = HEALTH_DIMENSIONS.find(hd => hd.id === d.dimensionId);
            return {
              dimension: dimInfo?.name || d.dimensionId,
              averageScore: d.avgScore,
            };
          });
          setHealthSummary(transformed);
        }
      } else if (healthRes.status >= 500) {
        setError('Unable to load dashboard data. Please refresh the page.');
      }

      // Fetch response distribution
      const distRes = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/dashboard/response-distribution${periodQuery}`);
      if (distRes.ok) {
        const data = await distRes.json();
        // Transform backend format to frontend format
        // Backend: { distribution: [{ dimensionId, red, yellow, green }] }
        // Frontend: [{ dimension, red, yellow, green }]
        if (data.distribution && Array.isArray(data.distribution)) {
          const transformed = data.distribution.map((d: { dimensionId: string; red: number; yellow: number; green: number }) => {
            const dimInfo = HEALTH_DIMENSIONS.find(hd => hd.id === d.dimensionId);
            return {
              dimension: dimInfo?.name || d.dimensionId,
              red: d.red,
              yellow: d.yellow,
              green: d.green,
            };
          });
          setDistribution(transformed);
        }
      } else if (distRes.status >= 500) {
        setError('Unable to load dashboard data. Please refresh the page.');
      }

      // Fetch individual responses
      const respRes = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/dashboard/individual-responses${periodQuery}`);
      if (respRes.ok) {
        const data = await respRes.json();
        // Transform backend format to frontend format
        // Backend: { responses: [{ sessionId, userId, userName, date, dimensions: [...] }] }
        // Frontend: [{ sessionId, userId, userName, date, responses: [...] }]
        if (data.responses && Array.isArray(data.responses)) {
          const transformed = data.responses.map((r: {
            sessionId: string;
            userId: string;
            userName: string;
            date: string;
            surveyType?: string;
            dimensions: { dimensionId: string; score: number; trend: string; comment: string }[];
          }) => ({
            sessionId: r.sessionId,
            userId: r.userId,
            userName: r.userName,
            date: r.date,
            surveyType: r.surveyType || 'individual',
            responses: (r.dimensions || []).map((d) => {
              const dimInfo = HEALTH_DIMENSIONS.find(hd => hd.id === d.dimensionId);
              return {
                dimensionId: d.dimensionId,
                dimensionName: dimInfo?.name || d.dimensionId,
                score: d.score,
                trend: d.trend,
                comment: d.comment || '',
              };
            }),
          }));
          setIndividualResponses(transformed);
          setCollapsedCards(new Set()); // Reset collapsed state when data changes
        }
      } else if (respRes.status >= 500) {
        setError('Unable to load dashboard data. Please refresh the page.');
      }

      // Fetch trends (trends don't filter by period - they show all periods)
      const trendsRes = await authenticatedFetch(`${API_BASE_URL}/api/v1/teams/${teamId}/dashboard/trends`);
      if (trendsRes.ok) {
        const data = await trendsRes.json();
        // Transform backend format to frontend format
        // Backend: { periods: [...], dimensions: [{ dimensionId, scores: [...] }] }
        // Frontend: [{ period, mission: 2.5, value: 3.0, ... }]
        if (data.periods && Array.isArray(data.periods) && data.dimensions) {
          const transformed = data.periods.map((period: string, idx: number) => {
            const row: TrendData = { period };
            (data.dimensions || []).forEach((dim: { dimensionId: string; scores: number[] }) => {
              row[dim.dimensionId] = dim.scores[idx] || 0;
            });
            return row;
          });
          setTrends(transformed);
        }
      } else if (trendsRes.status >= 500) {
        setError('Unable to load dashboard data. Please refresh the page.');
      }
    } catch (err) {
      console.error('Error fetching dashboard data:', err);
      setError('Unable to load dashboard data. Please refresh the page.');
    } finally {
      setLoading(false);
    }
  };

  const handlePeriodChange = (period: string) => {
    setSelectedPeriod(period);
    if (teamId) {
      fetchDashboardData(teamId, period);
    }
  };

  const handleTeamChange = (newTeamId: string) => {
    // Clear stale data before switching so a failed reload doesn't show old team's data
    setHealthSummary([]);
    setDistribution([]);
    setIndividualResponses([]);
    setTrends([]);
    setSubmissionStatus(null);
    setError(null);
    setTeamId(newTeamId);
    setSelectedPeriod('');
    fetchDashboardData(newTeamId, '');
    // Re-fetch team info (members + submission status) for the new team
    getTeamInfoCached(newTeamId)
      .then((teamInfo) => {
        setTeamMembers(teamInfo.members.map(m => ({ id: m.id, name: m.fullName })));
        const currentPeriod = getAssessmentPeriod(new Date(), toCadence(teamInfo.cadence));
        return getTeamSubmissionStatus(newTeamId, currentPeriod);
      })
      .catch(() => {
        const currentPeriod = getAssessmentPeriod(new Date());
        return getTeamSubmissionStatus(newTeamId, currentPeriod);
      })
      .then(setSubmissionStatus)
      .catch((err) => console.error('Failed to fetch submission status:', err));
  };

  const handleLogout = async () => {
    await logout();
    router.push('/login');
  };

  // All useMemo hooks must be called unconditionally — before any early return
  const matrixDims = useMemo(
    () => individualResponses[0]?.responses || [],
    [individualResponses]
  );

  const matrixDimAvgs = useMemo(
    () => matrixDims.map((dim) => {
      const scores = individualResponses.map(
        (r) => r.responses.find((resp) => resp.dimensionId === dim.dimensionId)?.score ?? 0
      );
      const nonZero = scores.filter((s) => s > 0);
      return nonZero.length > 0 ? nonZero.reduce((a, b) => a + b, 0) / nonZero.length : 0;
    }),
    [matrixDims, individualResponses]
  );

  // Breakdown view: percentage bars sorted worst-first
  const breakdownData = useMemo(
    () => [...distribution]
      .map((d) => {
        const total = d.red + d.yellow + d.green;
        const healthScore = total > 0 ? (d.green * 3 + d.yellow * 2 + d.red * 1) / total : 0;
        return {
          ...d,
          total,
          greenPct: total > 0 ? (d.green / total) * 100 : 0,
          yellowPct: total > 0 ? (d.yellow / total) * 100 : 0,
          redPct: total > 0 ? (d.red / total) * 100 : 0,
          healthScore,
        };
      })
      .sort((a, b) => a.healthScore - b.healthScore),
    [distribution]
  );

  // Small multiples: one sparkline card per dimension
  const dimSparklines = useMemo(
    () => HEALTH_DIMENSIONS.map((dim) => {
      const data = trends.map((t) => ({
        period: t.period as string,
        value: (t[dim.id] as number) || 0,
      }));
      const validData = data.filter((p) => p.value > 0);
      const latest = validData.length > 0 ? validData[validData.length - 1].value : 0;
      const prev = validData.length > 1 ? validData[validData.length - 2].value : latest;
      const direction: 'up' | 'down' | 'stable' =
        latest > prev + 0.1 ? 'up' : latest < prev - 0.1 ? 'down' : 'stable';
      return { dim, data, latest, direction };
    }).filter((d) => d.data.some((p) => p.value > 0)),
    [trends]
  );

  if (!user) return null;

  const config = getOrgConfig();
  const userLevel = getHierarchyLevel(user.hierarchyLevelId || '');
  const userPermissions = getUserPermissions(user);

  // Get score color
  const getScoreColor = (score: number) => {
    if (score === 3) return 'text-green-600';
    if (score === 2) return 'text-yellow-600';
    return 'text-red-600';
  };

  const getScoreLabel = (score: number) => {
    if (score === 3) return 'Green';
    if (score === 2) return 'Yellow';
    return 'Red';
  };

  // Inline styles used for dynamic colors to avoid Tailwind JIT purging dynamic class strings
  const getScoreDotColor = (score: number): string =>
    score === 3 ? '#10B981' : score === 2 ? '#F59E0B' : '#EF4444';

  const getAvgBadgeStyle = (avg: number): { backgroundColor: string; color: string } =>
    avg >= 2.5
      ? { backgroundColor: '#D1FAE5', color: '#065F46' }
      : avg >= 1.5
      ? { backgroundColor: '#FEF3C7', color: '#92400E' }
      : { backgroundColor: '#FEE2E2', color: '#991B1B' };

  const getScoreBandLabel = (avg: number): string => {
    if (avg >= 2.7) return 'Excellent';
    if (avg >= 2.3) return 'Good';
    if (avg >= 1.7) return 'Fair';
    return 'Poor';
  };

  const handleExportToExcel = async () => {
    const teamName = teamOptions.find(t => t.id === teamId)?.name || teamId;
    const periodLabel = selectedPeriod || 'All Periods';

    // Sheet 1: Summary — dimension averages with band labels
    const summaryRows = healthSummary.map(h => ({
      Dimension: h.dimension,
      'Average Score': parseFloat(h.averageScore.toFixed(2)),
      Band: getScoreBandLabel(h.averageScore),
    }));
    const summarySheet = XLSX.utils.json_to_sheet(summaryRows);

    // Sheet 2: Individual responses — one row per member per dimension
    const responseRows = individualResponses.flatMap(r =>
      r.responses.map(resp => ({
        Member: r.userName,
        'Survey Type': r.surveyType === 'post_workshop' ? 'Post-Workshop' : 'Individual',
        Date: new Date(r.date).toLocaleDateString(),
        Dimension: resp.dimensionName,
        Score: resp.score,
        'Score Label': resp.score === 3 ? 'Green' : resp.score === 2 ? 'Yellow' : 'Red',
        Trend: resp.trend || '',
        Comment: resp.comment || '',
      }))
    );
    const responsesSheet = XLSX.utils.json_to_sheet(responseRows);

    // Sheet 3: Distribution
    const distributionRows = breakdownData.map(d => ({
      Dimension: d.dimension,
      Green: d.green,
      Yellow: d.yellow,
      Red: d.red,
      Total: d.total,
      'Health Score': parseFloat(d.healthScore.toFixed(2)),
      Band: getScoreBandLabel(d.healthScore),
    }));
    const distributionSheet = XLSX.utils.json_to_sheet(distributionRows);

    // Sheet 4: Action items
    const { listActionItems } = await import('@/lib/api/action-items');
    const actionItems = await listActionItems(teamId).catch(() => []);
    const actionRows = actionItems.map(a => ({
      Title: a.title,
      Dimension: a.dimensionName || '',
      Status: a.status === 'in_progress' ? 'In Progress' : a.status === 'done' ? 'Done' : 'Open',
      'Assigned To': a.assigneeName || '',
      'Due Date': a.dueDate ? new Date(a.dueDate).toLocaleDateString() : '',
      Description: a.description || '',
      'Created By': a.createdByName || '',
      'Created At': new Date(a.createdAt).toLocaleDateString(),
    }));
    const actionsSheet = XLSX.utils.json_to_sheet(actionRows.length > 0 ? actionRows : [{ Title: 'No action items' }]);

    const wb = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(wb, summarySheet, 'Summary');
    XLSX.utils.book_append_sheet(wb, responsesSheet, 'Individual Responses');
    XLSX.utils.book_append_sheet(wb, distributionSheet, 'Distribution');
    XLSX.utils.book_append_sheet(wb, actionsSheet, 'Action Items');

    const fileName = `health-check-${teamName.replace(/\s+/g, '-').toLowerCase()}-${periodLabel.replace(/\s+/g, '-').toLowerCase()}.xlsx`;
    XLSX.writeFile(wb, fileName);
  };

  const getShortDimName = (name: string) => {
    const map: Record<string, string> = {
      'Delivering Value': 'D.Value',
      'Health of Codebase': 'Codebase',
      'Pawns or Players': 'Autonomy',
      'Easy to Release': 'Release',
      'Suitable Process': 'Process',
    };
    return map[name] || name;
  };

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <div className="bg-white shadow-sm border-b">
        <div className="container mx-auto px-4 py-4">
          <div className="flex justify-between items-center">
            <div className="flex items-center gap-4">
              {brandingLogo ? (
                <img src={brandingLogo} alt="Company logo" className="w-8 h-8 object-contain rounded" />
              ) : (
                <Building2 className="w-8 h-8 text-indigo-600" />
              )}
              <div>
                <h1 className="text-2xl font-bold text-gray-900">Team Lead Dashboard</h1>
                <div className="flex items-center gap-2">
                  <p className="text-gray-500">{brandingName || config.companyName} Health Metrics</p>
                  {teamOptions.length > 1 && (
                    <select
                      data-testid="team-selector"
                      value={teamId}
                      onChange={(e) => handleTeamChange(e.target.value)}
                      className="ml-2 px-2 py-1 text-sm border border-gray-300 rounded-lg text-gray-700 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    >
                      {teamOptions.map((t) => (
                        <option key={t.id} value={t.id}>{t.name}</option>
                      ))}
                    </select>
                  )}
                </div>
              </div>
            </div>

            <div className="flex items-center gap-4">
              {/* Take Survey Button */}
              <button
                onClick={() => router.push(teamId ? `/survey?team=${teamId}` : '/survey')}
                data-testid="take-survey-button"
                className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
              >
                <ClipboardList className="w-4 h-4" />
                Take Survey
              </button>

              {/* Post-Workshop Survey Button */}
              {submissionStatus?.postWorkshopExists ? (
                <span
                  data-testid="post-workshop-submitted-badge"
                  className="flex items-center gap-2 px-4 py-2 bg-green-50 text-green-700 rounded-lg border border-green-200"
                >
                  <CheckCircle className="w-4 h-4" />
                  Workshop Submitted
                </span>
              ) : (
                <button
                  onClick={() => router.push(teamId ? `/survey?type=post_workshop&team=${teamId}` : '/survey?type=post_workshop')}
                  data-testid="post-workshop-survey-button"
                  title="Record your team's workshop consensus"
                  className="flex items-center gap-2 px-4 py-2 rounded-lg transition-colors bg-amber-500 text-white hover:bg-amber-600"
                >
                  <ClipboardList className="w-4 h-4" />
                  Post-Workshop Survey
                </button>
              )}

              <div className="relative">
                <button
                  onClick={() => setShowUserInfo(!showUserInfo)}
                  className="flex items-center gap-2 px-4 py-2 bg-gray-100 rounded-lg hover:bg-gray-200 transition-colors"
                >
                  <div
                    className="w-3 h-3 rounded-full"
                    style={{ backgroundColor: userLevel?.color }}
                  />
                  <div className="text-right">
                    <p className="text-sm font-semibold text-gray-900">{user.name}</p>
                    <p className="text-xs text-gray-500">{userLevel?.name}</p>
                  </div>
                  <ChevronDown className="w-4 h-4" />
                </button>

                {showUserInfo && (
                  <div className="absolute right-0 mt-2 w-64 bg-white rounded-lg shadow-lg border p-4 z-10">
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-gray-500">Level:</span>
                        <span className="font-medium">{userLevel?.name}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-500">Teams:</span>
                        <span className="font-medium">{user.teamIds?.length || 0}</span>
                      </div>
                    </div>
                  </div>
                )}
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
        {/* Assessment Period Filter + Export */}
        <div className="mb-6 flex justify-between items-center">
          <h2 className="text-xl font-semibold text-gray-900">Team Health Overview</h2>
          <div className="flex items-center gap-3">
            <label htmlFor="period-filter" className="text-sm text-gray-600">
              Assessment Period:
            </label>
            <select
              id="period-filter"
              data-testid="period-filter"
              value={selectedPeriod}
              onChange={(e) => handlePeriodChange(e.target.value)}
              className="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent text-gray-900 bg-white"
            >
              <option value="">All Periods</option>
              {assessmentPeriodOptions.map((period) => (
                <option key={period} value={period}>{period}</option>
              ))}
            </select>
            {userPermissions.canExportData && healthSummary.length > 0 && (
              <button
                data-testid="export-excel-btn"
                onClick={handleExportToExcel}
                className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors text-sm font-medium"
              >
                <Download className="w-4 h-4" />
                Export to Excel
              </button>
            )}
          </div>
        </div>

        {/* Tabs */}
        <div className="bg-white rounded-xl shadow-sm border mb-8">
          <div className="border-b">
            <div className="flex gap-2 p-2">
              <button
                data-testid="radar-tab"
                onClick={() => setActiveTab('radar')}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  activeTab === 'radar'
                    ? 'bg-indigo-50 text-indigo-600 font-medium'
                    : 'text-gray-600 hover:bg-gray-50'
                }`}
              >
                <Activity className="w-4 h-4" />
                Radar Chart
              </button>
              <button
                data-testid="distribution-tab"
                onClick={() => setActiveTab('distribution')}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  activeTab === 'distribution'
                    ? 'bg-indigo-50 text-indigo-600 font-medium'
                    : 'text-gray-600 hover:bg-gray-50'
                }`}
              >
                <BarChart3 className="w-4 h-4" />
                Response Distribution
              </button>
              <button
                data-testid="responses-tab"
                onClick={() => setActiveTab('responses')}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  activeTab === 'responses'
                    ? 'bg-indigo-50 text-indigo-600 font-medium'
                    : 'text-gray-600 hover:bg-gray-50'
                }`}
              >
                <UsersIcon className="w-4 h-4" />
                Individual Responses
              </button>
              <button
                data-testid="trends-tab"
                onClick={() => setActiveTab('trends')}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  activeTab === 'trends'
                    ? 'bg-indigo-50 text-indigo-600 font-medium'
                    : 'text-gray-600 hover:bg-gray-50'
                }`}
              >
                <LineChartIcon className="w-4 h-4" />
                Trends
              </button>
              <button
                data-testid="actions-tab"
                onClick={() => setActiveTab('actions')}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  activeTab === 'actions'
                    ? 'bg-indigo-50 text-indigo-600 font-medium'
                    : 'text-gray-600 hover:bg-gray-50'
                }`}
              >
                <ListTodo className="w-4 h-4" />
                Actions
              </button>
            </div>
          </div>

          {/* Error Banner */}
          {error && (
            <div data-testid="dashboard-error-banner" className="mx-6 mt-4 flex items-center gap-2 text-red-700 text-sm bg-red-50 border border-red-200 p-3 rounded-lg">
              <AlertCircle className="w-4 h-4 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* Tab Content */}
          <div className="p-6">
            {loading ? (
              <div className="flex justify-center items-center py-12">
                <div className="text-gray-500">Loading...</div>
              </div>
            ) : (
              <>
                {/* Radar Chart Tab */}
                {activeTab === 'radar' && (
                  <div data-testid="radar-chart-section">
                    <h2 className="text-xl font-semibold text-gray-900 mb-4">Team Health Overview</h2>
                    {/* Score Band Legend */}
                    <div data-testid="score-band-legend" className="flex flex-wrap items-center gap-3 mb-6 p-3 bg-gray-50 rounded-lg border border-gray-200 text-xs font-medium">
                      <span className="text-gray-500 font-semibold">Score bands:</span>
                      <span className="px-2.5 py-1 rounded-full bg-emerald-100 text-emerald-800">2.7 – 3.0 Excellent</span>
                      <span className="px-2.5 py-1 rounded-full bg-green-100 text-green-800">2.3 – 2.6 Good</span>
                      <span className="px-2.5 py-1 rounded-full bg-yellow-100 text-yellow-800">1.7 – 2.2 Fair</span>
                      <span className="px-2.5 py-1 rounded-full bg-red-100 text-red-800">1.0 – 1.6 Poor</span>
                    </div>
                    {healthSummary.length > 0 ? (
                      <div data-testid="radar-chart" style={{ width: '100%', height: 500 }}>
                      <ResponsiveContainer width="100%" height={500}>
                        <RadarChart data={healthSummary}>
                          <PolarGrid />
                          <PolarAngleAxis dataKey="dimension" />
                          <PolarRadiusAxis domain={[0, 3]} />
                          <Radar
                            name="Health Score"
                            dataKey="averageScore"
                            stroke="#6366f1"
                            fill="#6366f1"
                            fillOpacity={0.6}
                          />
                          <Tooltip />
                          <Legend />
                        </RadarChart>
                      </ResponsiveContainer>
                      </div>
                    ) : (
                      <p className="text-gray-500 text-center py-12">No health data available</p>
                    )}
                  </div>
                )}

                {/* Distribution Tab */}
                {activeTab === 'distribution' && (
                  <div data-testid="distribution-chart-section">
                    <div className="flex justify-between items-center mb-6">
                      <div>
                        <h2 className="text-xl font-semibold text-gray-900">Response Distribution</h2>
                        {distributionView === 'breakdown' && distribution.length > 0 && (
                          <p className="text-xs text-gray-400 mt-0.5">Sorted by health score — most attention needed first</p>
                        )}
                      </div>
                      {distribution.length > 0 && (
                        <div className="flex items-center gap-1 bg-gray-100 p-1 rounded-lg">
                          <button
                            data-testid="distribution-breakdown-btn"
                            onClick={() => setDistributionView('breakdown')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              distributionView === 'breakdown'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <List className="w-4 h-4" />
                            By Dimension
                          </button>
                          <button
                            data-testid="distribution-chart-btn"
                            onClick={() => setDistributionView('chart')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              distributionView === 'chart'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <BarChart3 className="w-4 h-4" />
                            Chart
                          </button>
                        </div>
                      )}
                    </div>

                    {distribution.length > 0 ? (
                      <>
                        {distributionView === 'breakdown' ? (
                          /* By Dimension: horizontal stacked percentage bars */
                          <div className="space-y-2.5">
                            {breakdownData.map((d) => (
                              <div key={d.dimension} className="flex items-center gap-3">
                                <span
                                  className="text-sm text-gray-700 w-36 flex-shrink-0 text-right truncate font-medium"
                                  title={d.dimension}
                                >
                                  {d.dimension}
                                </span>
                                <div className="flex-1 flex h-7 rounded-md overflow-hidden text-xs font-semibold min-w-0">
                                  {d.greenPct > 0 && (
                                    <div
                                      className="flex items-center justify-center text-white"
                                      style={{ width: `${d.greenPct}%`, backgroundColor: '#10B981' }}
                                      title={`Green: ${d.green} (${d.greenPct.toFixed(0)}%)`}
                                    >
                                      {d.greenPct >= 10 ? `${d.greenPct.toFixed(0)}%` : ''}
                                    </div>
                                  )}
                                  {d.yellowPct > 0 && (
                                    <div
                                      className="flex items-center justify-center text-white"
                                      style={{ width: `${d.yellowPct}%`, backgroundColor: '#F59E0B' }}
                                      title={`Yellow: ${d.yellow} (${d.yellowPct.toFixed(0)}%)`}
                                    >
                                      {d.yellowPct >= 10 ? `${d.yellowPct.toFixed(0)}%` : ''}
                                    </div>
                                  )}
                                  {d.redPct > 0 && (
                                    <div
                                      className="flex items-center justify-center text-white"
                                      style={{ width: `${d.redPct}%`, backgroundColor: '#EF4444' }}
                                      title={`Red: ${d.red} (${d.redPct.toFixed(0)}%)`}
                                    >
                                      {d.redPct >= 10 ? `${d.redPct.toFixed(0)}%` : ''}
                                    </div>
                                  )}
                                </div>
                                <span
                                  className="inline-block px-2 py-0.5 rounded-full text-xs font-semibold text-center flex-shrink-0"
                                  style={getAvgBadgeStyle(d.healthScore)}
                                  title={getScoreBandLabel(d.healthScore)}
                                >
                                  {d.healthScore.toFixed(1)}
                                </span>
                                <span className="text-xs text-gray-400 w-14 text-right flex-shrink-0">
                                  {d.total} resp.
                                </span>
                              </div>
                            ))}
                            {/* Legend */}
                            <div className="flex items-center gap-5 mt-5 pt-4 border-t border-gray-100 text-xs text-gray-500">
                              <div className="flex items-center gap-1.5">
                                <span className="w-3 h-3 rounded-sm inline-block" style={{ backgroundColor: '#10B981' }} />
                                Green (Good)
                              </div>
                              <div className="flex items-center gap-1.5">
                                <span className="w-3 h-3 rounded-sm inline-block" style={{ backgroundColor: '#F59E0B' }} />
                                Yellow (Medium)
                              </div>
                              <div className="flex items-center gap-1.5">
                                <span className="w-3 h-3 rounded-sm inline-block" style={{ backgroundColor: '#EF4444' }} />
                                Red (Poor)
                              </div>
                              <span className="ml-auto text-gray-400">Score = weighted average (3·green + 2·yellow + 1·red)</span>
                            </div>
                          </div>
                        ) : (
                          /* Chart view — original grouped bar chart */
                          <div data-testid="distribution-chart" style={{ width: '100%', height: 500 }}>
                          <ResponsiveContainer width="100%" height={500}>
                            <BarChart data={distribution} margin={{ top: 8, right: 16, left: 0, bottom: 80 }}>
                              <CartesianGrid strokeDasharray="3 3" />
                              <XAxis
                                dataKey="dimension"
                                interval={0}
                                angle={-35}
                                textAnchor="end"
                                tick={{ fontSize: 12, fill: '#374151' }}
                                height={80}
                              />
                              <YAxis />
                              <Tooltip />
                              <Legend verticalAlign="top" />
                              <Bar dataKey="red" fill="#EF4444" name="Red (Poor)" />
                              <Bar dataKey="yellow" fill="#F59E0B" name="Yellow (Medium)" />
                              <Bar dataKey="green" fill="#10B981" name="Green (Good)" />
                            </BarChart>
                          </ResponsiveContainer>
                          </div>
                        )}
                      </>

                    ) : (
                      <p className="text-gray-500 text-center py-12">No distribution data available</p>
                    )}
                  </div>
                )}

                {/* Individual Responses Tab */}
                {activeTab === 'responses' && (
                  <div data-testid="responses-section">
                    <div className="flex justify-between items-center mb-6">
                      <h2 className="text-xl font-semibold text-gray-900">Individual Team Responses</h2>
                      {individualResponses.length > 0 && (
                        <div className="flex items-center gap-1 bg-gray-100 p-1 rounded-lg">
                          <button
                            data-testid="matrix-view-btn"
                            onClick={() => setResponseView('matrix')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              responseView === 'matrix'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <LayoutGrid className="w-4 h-4" />
                            Matrix
                          </button>
                          <button
                            data-testid="cards-view-btn"
                            onClick={() => setResponseView('cards')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              responseView === 'cards'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <List className="w-4 h-4" />
                            Cards
                          </button>
                        </div>
                      )}
                    </div>


                    {individualResponses.length > 0 ? (
                      <>
                        {responseView === 'matrix' ? (
                          /* Matrix View */
                          <div>
                            <div className="overflow-x-auto rounded-lg border border-gray-200">
                              <table className="w-full text-sm border-collapse">
                                <thead>
                                  <tr className="bg-gray-50 border-b border-gray-200">
                                    <th className="sticky left-0 z-10 bg-gray-50 px-4 py-3 text-left font-semibold text-gray-700 min-w-[148px] border-r border-gray-200">
                                      Member
                                    </th>
                                    {matrixDims.map((dim) => (
                                      <th
                                        key={dim.dimensionId}
                                        className="px-3 py-3 text-center font-medium text-gray-600 min-w-[80px]"
                                      >
                                        <span className="block truncate max-w-[72px] mx-auto">
                                          {getShortDimName(dim.dimensionName)}
                                        </span>
                                      </th>
                                    ))}
                                  </tr>
                                </thead>
                                <tbody>
                                  {individualResponses.map((response, idx) => {
                                    return (
                                      <tr
                                        key={idx}
                                        className="border-b border-gray-100 hover:bg-indigo-50/30 transition-colors group/row"
                                        data-testid="response-card"
                                      >
                                        <td className="sticky left-0 z-10 bg-white group-hover/row:bg-indigo-50/30 px-4 py-3 border-r border-gray-200 transition-colors">
                                          <div
                                            className="font-medium text-gray-900 truncate max-w-[128px]"
                                            title={response.userName}
                                          >
                                            {response.userName}
                                          </div>
                                          <div className="text-xs text-gray-400">
                                            {new Date(response.date).toLocaleDateString()}
                                          </div>
                                        </td>
                                        {matrixDims.map((dim) => {
                                          const resp = response.responses.find(
                                            (r) => r.dimensionId === dim.dimensionId
                                          );
                                          const score = resp?.score ?? 0;
                                          const trend = resp?.trend ?? '';
                                          const comment = resp?.comment ?? '';
                                          const TrendIcon =
                                            trend === 'improving'
                                              ? TrendingUp
                                              : trend === 'declining'
                                              ? TrendingDown
                                              : Minus;
                                          const trendColor =
                                            trend === 'improving'
                                              ? 'text-green-500'
                                              : trend === 'declining'
                                              ? 'text-red-500'
                                              : 'text-gray-400';
                                          return (
                                            <td
                                              key={dim.dimensionId}
                                              className="px-3 py-3 text-center cursor-default"
                                              onMouseEnter={(e) =>
                                                score > 0 &&
                                                setTooltip({
                                                  x: e.clientX,
                                                  y: e.clientY,
                                                  dimensionName: dim.dimensionName,
                                                  score,
                                                  trend,
                                                  comment,
                                                })
                                              }
                                              onMouseMove={(e) =>
                                                score > 0 &&
                                                setTooltip((prev) =>
                                                  prev ? { ...prev, x: e.clientX, y: e.clientY } : prev
                                                )
                                              }
                                              onMouseLeave={() => setTooltip(null)}
                                            >
                                              {score > 0 ? (
                                                <div className="flex flex-col items-center gap-0.5 relative">
                                                  <span
                                                    className="inline-block w-5 h-5 rounded-full"
                                                    style={{ backgroundColor: getScoreDotColor(score) }}
                                                    aria-label={getScoreLabel(score)}
                                                    data-testid={`matrix-score-${response.sessionId}-${dim.dimensionId}`}
                                                  />
                                                  <TrendIcon
                                                    className={`w-3 h-3 ${trendColor}`}
                                                    data-testid={`matrix-trend-${response.sessionId}-${dim.dimensionId}`}
                                                  />
                                                  {comment && (
                                                    <span
                                                      className="absolute -top-1 -right-1 w-1.5 h-1.5 rounded-full bg-indigo-400"
                                                      data-testid={`matrix-comment-${response.sessionId}-${dim.dimensionId}`}
                                                    />
                                                  )}
                                                </div>
                                              ) : (
                                                <span className="text-gray-300 text-xs">—</span>
                                              )}
                                            </td>
                                          );
                                        })}
                                      </tr>
                                    );
                                  })}
                                  {/* Team Average Row */}
                                  <tr className="bg-gray-50 border-t-2 border-gray-300">
                                    <td className="sticky left-0 z-10 bg-gray-50 px-4 py-3 border-r border-gray-200 text-sm font-semibold text-gray-700">
                                      Team Average
                                    </td>
                                    {matrixDimAvgs.map((avg, i) => (
                                      <td key={i} className="px-3 py-3 text-center">
                                        <span
                                          className="inline-block px-2 py-0.5 rounded-full text-xs font-semibold"
                                          style={getAvgBadgeStyle(avg)}
                                        >
                                          {avg > 0 ? avg.toFixed(1) : '—'}
                                        </span>
                                      </td>
                                    ))}
                                  </tr>
                                </tbody>
                              </table>
                            </div>
                            {/* Legend */}
                            <div className="flex flex-wrap items-center gap-x-6 gap-y-2 mt-4 px-1 text-xs text-gray-500">
                              <span className="font-medium text-gray-600">Score:</span>
                              <div className="flex items-center gap-1.5">
                                <span className="w-4 h-4 rounded-full bg-green-500 inline-block" />
                                Green (Good)
                              </div>
                              <div className="flex items-center gap-1.5">
                                <span className="w-4 h-4 rounded-full inline-block" style={{ backgroundColor: '#F59E0B' }} />
                                Yellow (Medium)
                              </div>
                              <div className="flex items-center gap-1.5">
                                <span className="w-4 h-4 rounded-full bg-red-500 inline-block" />
                                Red (Poor)
                              </div>
                              <span className="ml-2 font-medium text-gray-600">Trend:</span>
                              <div className="flex items-center gap-1">
                                <TrendingUp className="w-3.5 h-3.5 text-green-500" />
                                Improving
                              </div>
                              <div className="flex items-center gap-1">
                                <Minus className="w-3.5 h-3.5 text-gray-400" />
                                Stable
                              </div>
                              <div className="flex items-center gap-1">
                                <TrendingDown className="w-3.5 h-3.5 text-red-500" />
                                Declining
                              </div>
                              <div className="ml-auto flex items-center gap-1.5 px-2.5 py-1 bg-indigo-50 text-indigo-600 rounded-full border border-indigo-100">
                                <Info className="w-3.5 h-3.5 flex-shrink-0" />
                                <span className="font-medium">Hover a cell for details</span>
                              </div>
                            </div>
                          </div>
                        ) : (
                          /* Cards View — collapsible per member */
                          <div className="space-y-3">
                            {/* Collapse / Expand All */}
                            <div className="flex justify-end">
                              <button
                                onClick={() =>
                                  collapsedCards.size === individualResponses.length
                                    ? setCollapsedCards(new Set())
                                    : setCollapsedCards(new Set(individualResponses.map((_, i) => i)))
                                }
                                className="flex items-center gap-1.5 text-xs text-indigo-600 hover:text-indigo-800 font-medium transition-colors"
                              >
                                <ChevronDown
                                  className={`w-3.5 h-3.5 transition-transform duration-200 ${
                                    collapsedCards.size === individualResponses.length ? '' : 'rotate-180'
                                  }`}
                                />
                                {collapsedCards.size === individualResponses.length
                                  ? 'Expand all'
                                  : 'Collapse all'}
                              </button>
                            </div>

                            {individualResponses.map((response, idx) => {
                              const isCollapsed = collapsedCards.has(idx);
                              const toggle = () =>
                                setCollapsedCards((prev) => {
                                  const next = new Set(prev);
                                  next.has(idx) ? next.delete(idx) : next.add(idx);
                                  return next;
                                });

                              // Mini score summary shown when collapsed
                              const greenCount = response.responses.filter((r) => r.score === 3).length;
                              const yellowCount = response.responses.filter((r) => r.score === 2).length;
                              const redCount = response.responses.filter((r) => r.score === 1).length;

                              return (
                                <div
                                  key={idx}
                                  className="border rounded-lg overflow-hidden"
                                  data-testid="response-card"
                                >
                                  {/* Clickable header */}
                                  <button
                                    onClick={toggle}
                                    className="w-full flex items-center justify-between px-4 py-3 hover:bg-gray-50 transition-colors text-left"
                                  >
                                    <div className="flex items-center gap-3">
                                      <div>
                                        <div className="flex items-center gap-2">
                                          <h3 className="font-semibold text-gray-900 text-sm">
                                            {response.userName}
                                          </h3>
                                          {response.surveyType === 'post_workshop' ? (
                                            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800">
                                              Post-Workshop
                                            </span>
                                          ) : (
                                            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                                              Individual
                                            </span>
                                          )}
                                        </div>
                                        <p className="text-xs text-gray-400">
                                          {new Date(response.date).toLocaleDateString()}
                                        </p>
                                      </div>

                                      {/* Score pill summary — only visible when collapsed */}
                                      {isCollapsed && (
                                        <div className="flex items-center gap-1.5 ml-2">
                                          {greenCount > 0 && (
                                            <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-700">
                                              <span className="w-2 h-2 rounded-full bg-green-500 inline-block" />
                                              {greenCount}
                                            </span>
                                          )}
                                          {yellowCount > 0 && (
                                            <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700">
                                              <span className="w-2 h-2 rounded-full inline-block" style={{ backgroundColor: '#F59E0B' }} />
                                              {yellowCount}
                                            </span>
                                          )}
                                          {redCount > 0 && (
                                            <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-700">
                                              <span className="w-2 h-2 rounded-full bg-red-500 inline-block" />
                                              {redCount}
                                            </span>
                                          )}
                                        </div>
                                      )}
                                    </div>

                                    <ChevronDown
                                      className={`w-4 h-4 text-gray-400 flex-shrink-0 transition-transform duration-200 ${
                                        isCollapsed ? '' : 'rotate-180'
                                      }`}
                                    />
                                  </button>

                                  {/* Collapsible body */}
                                  {!isCollapsed && (
                                    <div className="px-4 pb-4 border-t border-gray-100">
                                      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 pt-3">
                                        {response.responses.map((resp, respIdx) => (
                                          <div key={respIdx} className="bg-gray-50 rounded p-3">
                                            <div className="flex justify-between items-start mb-2">
                                              <span className="text-sm font-medium text-gray-700">
                                                {resp.dimensionName}
                                              </span>
                                              <span
                                                className={`text-xs font-semibold px-2 py-1 rounded ${getScoreColor(resp.score)}`}
                                                data-testid="score-indicator"
                                              >
                                                {getScoreLabel(resp.score)}
                                              </span>
                                            </div>
                                            {resp.comment && (
                                              <p className="text-xs text-gray-600 mt-2" data-testid="comment">
                                                {resp.comment}
                                              </p>
                                            )}
                                          </div>
                                        ))}
                                      </div>
                                    </div>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        )}
                      </>
                    ) : (
                      <p className="text-gray-500 text-center py-12">No individual responses available</p>
                    )}
                  </div>
                )}

                {/* Trends Tab */}
                {activeTab === 'trends' && (
                  <div data-testid="trends-chart-section">
                    <div className="flex justify-between items-center mb-6">
                      <div>
                        <h2 className="text-xl font-semibold text-gray-900">Health Trends Over Time</h2>
                        {trends.length > 0 && (
                          <p className="text-xs text-gray-400 mt-0.5 flex items-center gap-1">
                            <Info className="w-3 h-3 flex-shrink-0" />
                            Hover over a dot to see the assessment period and score
                          </p>
                        )}
                      </div>
                      {trends.length > 0 && (
                        <div className="flex items-center gap-1 bg-gray-100 p-1 rounded-lg">
                          <button
                            onClick={() => setTrendsView('dimensions')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              trendsView === 'dimensions'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <LayoutGrid className="w-4 h-4" />
                            By Dimension
                          </button>
                          <button
                            onClick={() => setTrendsView('overview')}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                              trendsView === 'overview'
                                ? 'bg-white text-indigo-600 shadow-sm'
                                : 'text-gray-600 hover:text-gray-900'
                            }`}
                          >
                            <LineChartIcon className="w-4 h-4" />
                            Overview
                          </button>
                        </div>
                      )}
                    </div>

                    {trends.length > 0 ? (
                      <>
                        {trendsView === 'dimensions' ? (
                          /* Small multiples — one sparkline card per dimension */
                          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                            {dimSparklines.map(({ dim, data, latest, direction }) => {
                              const lineColor =
                                latest >= 2.5 ? '#10B981' : latest >= 1.5 ? '#F59E0B' : '#EF4444';
                              return (
                                <div
                                  key={dim.id}
                                  className="border rounded-xl p-3 flex flex-col gap-1.5 hover:shadow-md transition-shadow"
                                >
                                  {/* Dimension name + score badge */}
                                  <div className="flex items-start justify-between gap-2">
                                    <h4 className="text-xs font-semibold text-gray-700 leading-tight">
                                      {dim.name}
                                    </h4>
                                    <span
                                      className="inline-block px-1.5 py-0.5 rounded-full text-xs font-bold flex-shrink-0"
                                      style={getAvgBadgeStyle(latest)}
                                    >
                                      {latest > 0 ? latest.toFixed(1) : '—'}
                                    </span>
                                  </div>

                                  {/* Trend direction */}
                                  <div className="flex items-center gap-1 text-xs">
                                    {direction === 'up' && (
                                      <TrendingUp className="w-3.5 h-3.5 text-green-500 flex-shrink-0" />
                                    )}
                                    {direction === 'down' && (
                                      <TrendingDown className="w-3.5 h-3.5 text-red-500 flex-shrink-0" />
                                    )}
                                    {direction === 'stable' && (
                                      <Minus className="w-3.5 h-3.5 text-gray-400 flex-shrink-0" />
                                    )}
                                    <span
                                      className={
                                        direction === 'up'
                                          ? 'text-green-600'
                                          : direction === 'down'
                                          ? 'text-red-600'
                                          : 'text-gray-400'
                                      }
                                    >
                                      {direction === 'up'
                                        ? 'Improving'
                                        : direction === 'down'
                                        ? 'Declining'
                                        : 'Stable'}
                                    </span>
                                  </div>

                                  {/* Sparkline */}
                                  {data.length > 1 ? (
                                    <ResponsiveContainer width="100%" height={52}>
                                      <LineChart
                                        data={data}
                                        margin={{ top: 4, right: 4, left: 4, bottom: 4 }}
                                      >
                                        <XAxis dataKey="period" hide />
                                        <YAxis domain={[1, 3]} hide />
                                        <Line
                                          type="monotone"
                                          dataKey="value"
                                          stroke={lineColor}
                                          strokeWidth={2}
                                          dot={{ r: 3, fill: lineColor }}
                                          activeDot={{ r: 4 }}
                                        />
                                        <Tooltip
                                          contentStyle={{
                                            fontSize: '11px',
                                            padding: '6px 10px',
                                            borderRadius: '6px',
                                            backgroundColor: '#1f2937',
                                            border: '1px solid #374151',
                                            color: '#f9fafb',
                                          }}
                                          labelStyle={{
                                            color: '#e5e7eb',
                                            fontWeight: 600,
                                            marginBottom: '2px',
                                          }}
                                          itemStyle={{ color: '#d1fae5' }}
                                          cursor={{ stroke: '#6366f1', strokeWidth: 1, strokeDasharray: '3 3' }}
                                          formatter={(v: number) => [v.toFixed(2), 'Score']}
                                          labelFormatter={(period) => `Period: ${period}`}
                                        />
                                      </LineChart>
                                    </ResponsiveContainer>
                                  ) : (
                                    <div className="h-[52px] flex items-center justify-center text-xs text-gray-300">
                                      Single period
                                    </div>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        ) : (
                          /* Overview — 11-line chart with perceptual palette + focused tooltip */
                          (() => {
                            // Hand-picked perceptually-distinct colours (hue + lightness varied)
                            const DIM_COLORS = [
                              '#6366f1', // indigo
                              '#10b981', // emerald
                              '#f59e0b', // amber
                              '#ef4444', // red
                              '#3b82f6', // blue
                              '#8b5cf6', // violet
                              '#14b8a6', // teal
                              '#f97316', // orange
                              '#ec4899', // pink
                              '#84cc16', // lime
                              '#64748b', // slate
                            ];
                            // Dash patterns to distinguish lines beyond colour alone
                            const DASHES = ['0', '6 3', '3 3', '8 3 3 3', '6 3 3 3', '0', '6 3', '3 3', '8 3 3 3', '6 3 3 3', '0'];
                            return (
                              <div data-testid="trends-chart">
                                <ResponsiveContainer width="100%" height={420}>
                                  <LineChart
                                    data={trends}
                                    margin={{ top: 8, right: 24, left: 0, bottom: 8 }}
                                  >
                                    <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                                    <XAxis
                                      dataKey="period"
                                      tick={{ fontSize: 12, fill: '#6b7280' }}
                                    />
                                    <YAxis
                                      domain={[0.8, 3.2]}
                                      ticks={[1, 1.5, 2, 2.5, 3]}
                                      tickFormatter={(v) => v === 1 ? 'Red' : v === 2 ? 'Yellow' : v === 3 ? 'Green' : String(v)}
                                      tick={{ fontSize: 11, fill: '#9ca3af' }}
                                      width={52}
                                    />
                                    <Tooltip
                                      contentStyle={{
                                        fontSize: '12px',
                                        padding: '8px 12px',
                                        borderRadius: '8px',
                                        backgroundColor: '#1f2937',
                                        border: '1px solid #374151',
                                        color: '#f9fafb',
                                        boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
                                      }}
                                      labelStyle={{ color: '#9ca3af', fontWeight: 600, marginBottom: '6px' }}
                                      itemStyle={{ color: '#f9fafb', padding: '1px 0' }}
                                      formatter={(value: number, name: string) => [value.toFixed(2), name]}
                                      labelFormatter={(period) => `Period: ${period}`}
                                    />
                                    {HEALTH_DIMENSIONS.map((dim, idx) => (
                                      <Line
                                        key={dim.id}
                                        type="monotone"
                                        dataKey={dim.id}
                                        name={dim.name}
                                        stroke={DIM_COLORS[idx]}
                                        strokeWidth={2}
                                        strokeDasharray={DASHES[idx]}
                                        dot={{ r: 3, fill: DIM_COLORS[idx], strokeWidth: 0 }}
                                        activeDot={{ r: 5, strokeWidth: 2, stroke: '#fff' }}
                                      />
                                    ))}
                                  </LineChart>
                                </ResponsiveContainer>
                                {/* Compact legend grid below chart */}
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-x-4 gap-y-1.5 mt-3 px-1">
                                  {HEALTH_DIMENSIONS.map((dim, idx) => (
                                    <div key={dim.id} className="flex items-center gap-2 min-w-0">
                                      <svg width="20" height="10" className="flex-shrink-0">
                                        <line
                                          x1="0" y1="5" x2="20" y2="5"
                                          stroke={DIM_COLORS[idx]}
                                          strokeWidth="2"
                                          strokeDasharray={DASHES[idx]}
                                        />
                                      </svg>
                                      <span className="text-xs text-gray-600 truncate">{dim.name}</span>
                                    </div>
                                  ))}
                                </div>
                              </div>
                            );
                          })()
                        )}
                      </>

                    ) : (
                      <p className="text-gray-500 text-center py-12">No trend data available</p>
                    )}
                  </div>
                )}

                {/* Actions Tab */}
                {activeTab === 'actions' && (
                  <div data-testid="actions-tab-panel">
                    <ActionItemsTab
                      teamId={teamId}
                      assessmentPeriod={selectedPeriod || ''}
                      defaultDimensionId={
                        healthSummary.length > 0
                          ? (() => {
                              const worstName = [...healthSummary].sort((a, b) => a.averageScore - b.averageScore)[0]?.dimension;
                              return HEALTH_DIMENSIONS.find(d => d.name === worstName)?.id;
                            })()
                          : undefined
                      }
                      teamMembers={teamMembers}
                      canEdit={true}
                    />
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </div>

      {/* Fixed-position tooltip — rendered outside overflow containers so it's never clipped */}
      {tooltip && (
        <div
          className="fixed z-50 pointer-events-none"
          style={{ left: tooltip.x + 14, top: tooltip.y - 8 }}
        >
          <div className="bg-gray-900 text-white text-xs rounded-lg px-3 py-2 shadow-xl max-w-[220px]">
            <p className="font-semibold mb-1">{tooltip.dimensionName}</p>
            <p>
              Score: <span className="font-medium">{getScoreLabel(tooltip.score)}</span>
            </p>
            {tooltip.trend && (
              <p>
                Trend: <span className="capitalize">{tooltip.trend}</span>
              </p>
            )}
            {tooltip.comment && (
              <p className="mt-1 text-gray-300 whitespace-normal" data-testid="comment">
                {tooltip.comment}
              </p>
            )}
          </div>
        </div>
      )}
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
