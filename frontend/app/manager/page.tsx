'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { getCurrentUser, logout, authenticatedFetch, User } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { API_BASE_URL } from '@/lib/api/client';
import { getAssessmentPeriods } from '@/lib/api/health-checks';
import { LogOut, Users, ChevronDown, AlertCircle, Activity, LineChart as LineChartIcon, CheckCircle, Clock, ClipboardList, TrendingUp, TrendingDown, Minus, LayoutGrid, Download, ListTodo } from 'lucide-react';
import { RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import OnboardingModal from '@/components/OnboardingModal';
import { listManagerTeamsActionSummary, TeamActionSummary } from '@/lib/api/action-items';
import * as XLSX from 'xlsx';

// Types matching backend API response
interface DimensionSummary {
  dimensionId: string;
  avgScore: number;
  responseCount: number;
}

interface TeamHealthSummary {
  teamId: string;
  teamName: string;
  overallHealth: number;
  submissionCount: number;
  dimensions: DimensionSummary[];
  postWorkshopStatus?: string;
}

interface ManagerDashboardResponse {
  managerId: string;
  teams: TeamHealthSummary[];
  totalTeams: number;
  assessmentPeriod?: string;
}

interface RadarData {
  dimension: string;
  averageScore: number;
}

interface TrendData {
  period: string;
  [key: string]: string | number;
}

interface Subordinate {
  id: string;
  username: string;
  name: string;
  hierarchyLevelId: string;
  reportsTo?: string;
  teamIds: string[];
}

type TabView = 'teams' | 'hierarchy' | 'summary' | 'comparison' | 'radar' | 'trends' | 'actions';

const LEVEL_STYLES: Record<string, { border: string; icon: string; label: string }> = {
  'level-1': { border: 'border-purple-500', icon: 'text-purple-600', label: 'VP' },
  'level-2': { border: 'border-blue-500', icon: 'text-blue-600', label: 'Director' },
  'level-3': { border: 'border-teal-500', icon: 'text-teal-600', label: 'Manager' },
  'level-4': { border: 'border-green-500', icon: 'text-green-600', label: 'Team Lead' },
  'level-5': { border: 'border-gray-400', icon: 'text-gray-500', label: 'Team Member' },
};

function HierarchyNode({
  person,
  childrenByParent,
  teamsByIdMap,
  depth,
}: {
  person: { id: string; name: string; hierarchyLevelId: string; username: string; teamIds: string[] };
  childrenByParent: Map<string, Subordinate[]>;
  teamsByIdMap: Map<string, TeamHealthSummary>;
  depth: number;
}) {
  const style = LEVEL_STYLES[person.hierarchyLevelId] || LEVEL_STYLES['level-5'];
  const directReports = childrenByParent.get(person.id) || [];
  const personTeams = (person.teamIds || [])
    .map((tid) => teamsByIdMap.get(tid))
    .filter((t): t is TeamHealthSummary => !!t);

  return (
    <div data-testid={`org-node-${person.id}`} className={`border-l-4 ${style.border} pl-4 py-2`}>
      <div className="flex items-center gap-3">
        <Users className={`w-5 h-5 ${style.icon}`} />
        <div>
          <div className="font-semibold text-gray-900">
            {person.name}
            <span className={`ml-2 text-xs font-normal px-2 py-0.5 rounded-full bg-gray-100 ${style.icon}`}>
              {style.label}
            </span>
          </div>
          {personTeams.length > 0 && (
            <div className="text-xs text-gray-500 mt-0.5">
              {personTeams.map((t) => t.teamName).join(', ')}
            </div>
          )}
        </div>
      </div>

      {directReports.length > 0 && (
        <div className={`mt-3 ${depth < 4 ? 'ml-6' : 'ml-4'} space-y-2`}>
          {directReports.map((sub) => (
            <HierarchyNode
              key={sub.id}
              person={sub}
              childrenByParent={childrenByParent}
              teamsByIdMap={teamsByIdMap}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export default function ManagerPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [dashboardData, setDashboardData] = useState<ManagerDashboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<string>('');
  const [assessmentPeriodOptions, setAssessmentPeriodOptions] = useState<string[]>([]);
  const [expandedTeam, setExpandedTeam] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabView>('teams');
  const [selectedTeamsForComparison, setSelectedTeamsForComparison] = useState<string[]>([]);

  // Subordinate tree state
  const [subordinates, setSubordinates] = useState<Subordinate[]>([]);

  // Pre-group subordinates by parent and teams by ID for O(1) lookups in hierarchy tree
  const childrenByParent = useMemo(() => {
    const map = new Map<string, Subordinate[]>();
    for (const sub of subordinates) {
      const parent = sub.reportsTo || '';
      if (!map.has(parent)) map.set(parent, []);
      map.get(parent)!.push(sub);
    }
    return map;
  }, [subordinates]);

  const teamsByIdMap = useMemo(() => {
    const map = new Map<string, TeamHealthSummary>();
    for (const team of dashboardData?.teams || []) {
      map.set(team.teamId, team);
    }
    return map;
  }, [dashboardData?.teams]);

  // Radar and Trends data states
  const [radarData, setRadarData] = useState<RadarData[]>([]);
  const [trendsData, setTrendsData] = useState<TrendData[]>([]);
  const [radarLoading, setRadarLoading] = useState(false);
  const [trendsLoading, setTrendsLoading] = useState(false);
  const [brandingName, setBrandingName] = useState<string>('');
  const [brandingLogo, setBrandingLogo] = useState<string | null>(null);
  const [trendsView, setTrendsView] = useState<'overview' | 'dimensions'>('dimensions');
  const [selectedTrendTeam, setSelectedTrendTeam] = useState<string>(''); // '' = all teams averaged
  const [showOnboarding, setShowOnboarding] = useState(false);
  const [actionSummary, setActionSummary] = useState<TeamActionSummary[]>([]);
  const [actionSummaryLoading, setActionSummaryLoading] = useState(false);

  useEffect(() => {
    const currentUser = getCurrentUser();
    if (!currentUser) {
      router.push('/login');
    } else {
      setUser(currentUser);
      fetchDashboardData(currentUser.id, '');
      fetchSubordinates(currentUser.id);
      if (!localStorage.getItem(`onboarding_complete:${currentUser.id}`)) {
        setShowOnboarding(true);
      }
    }

    // Fetch branding from public config endpoint
    fetch(`${API_BASE_URL}/api/v1/config`)
      .then(res => res.json())
      .then(data => {
        if (data.companyName) setBrandingName(data.companyName);
        if (data.logoURL) setBrandingLogo(data.logoURL);
      })
      .catch(() => {});
  }, [router]);

  // Fetch assessment period options from database
  useEffect(() => {
    getAssessmentPeriods()
      .then(setAssessmentPeriodOptions)
      .catch((err) => console.error('Failed to fetch assessment periods:', err));
  }, []);

  const fetchDashboardData = async (managerId: string, assessmentPeriod: string) => {
    setLoading(true);
    setError(null);

    try {
      const url = assessmentPeriod
        ? `/api/v1/managers/${managerId}/teams/health?assessmentPeriod=${encodeURIComponent(assessmentPeriod)}`
        : `/api/v1/managers/${managerId}/teams/health`;

      const response = await authenticatedFetch(url);

      if (!response.ok) {
        throw new Error(`Failed to fetch dashboard data: ${response.statusText}`);
      }

      const data: ManagerDashboardResponse = await response.json();
      setDashboardData(data);
    } catch (err) {
      setError('Unable to load team health data. Please refresh the page.');
      console.error('Error fetching dashboard data:', err);
    } finally {
      setLoading(false);
    }
  };

  const fetchSubordinates = async (managerId: string) => {
    try {
      const response = await authenticatedFetch(`/api/v1/managers/${managerId}/subordinates`);
      if (response.ok) {
        const data = await response.json();
        setSubordinates(data.subordinates || []);
      }
    } catch (err) {
      console.error('Error fetching subordinates:', err);
    }
  };

  const fetchRadarData = async (managerId: string, assessmentPeriod: string) => {
    setRadarLoading(true);
    try {
      const url = assessmentPeriod
        ? `/api/v1/managers/${managerId}/dashboard/radar?assessmentPeriod=${encodeURIComponent(assessmentPeriod)}`
        : `/api/v1/managers/${managerId}/dashboard/radar`;

      const response = await authenticatedFetch(url);
      if (response.ok) {
        const data = await response.json();
        // Transform backend format to frontend format
        // Backend: { dimensions: [{ dimensionId, avgScore, responseCount }] }
        // Frontend: [{ dimension, averageScore }]
        if (data.dimensions && Array.isArray(data.dimensions)) {
          const transformed = data.dimensions.map((d: DimensionSummary) => {
            const dimInfo = HEALTH_DIMENSIONS.find(hd => hd.id === d.dimensionId);
            return {
              dimension: dimInfo?.name || d.dimensionId,
              averageScore: d.avgScore,
            };
          });
          setRadarData(transformed);
        } else {
          setRadarData([]);
        }
      }
    } catch (error) {
      console.error('Error fetching radar data:', error);
      setRadarData([]);
    } finally {
      setRadarLoading(false);
    }
  };

  const fetchTrendsData = async (managerId: string, teamId?: string) => {
    setTrendsLoading(true);
    try {
      // When a specific team is selected, use the team-level endpoint (same as Team Lead dashboard).
      // Otherwise fall back to the manager-level aggregated endpoint.
      const url = teamId
        ? `${API_BASE_URL}/api/v1/teams/${teamId}/dashboard/trends`
        : `${API_BASE_URL}/api/v1/managers/${managerId}/dashboard/trends`;

      const response = await authenticatedFetch(url);
      if (response.ok) {
        const data = await response.json();
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
          setTrendsData(transformed);
        } else {
          setTrendsData([]);
        }
      }
    } catch (error) {
      console.error('Error fetching trends data:', error);
      setTrendsData([]);
    } finally {
      setTrendsLoading(false);
    }
  };

  // Fetch radar/trends data when tab or team filter changes
  useEffect(() => {
    if (user && activeTab === 'radar') {
      fetchRadarData(user.id, selectedPeriod);
    } else if (user && activeTab === 'trends') {
      fetchTrendsData(user.id, selectedTrendTeam || undefined);
    } else if (user && activeTab === 'actions') {
      setActionSummaryLoading(true);
      listManagerTeamsActionSummary(user.id)
        .then(setActionSummary)
        .catch(() => setActionSummary([]))
        .finally(() => setActionSummaryLoading(false));
    }
  }, [user, activeTab, selectedPeriod, selectedTrendTeam]);

  const handlePeriodChange = (period: string) => {
    setSelectedPeriod(period);
    if (user) {
      fetchDashboardData(user.id, period);
      if (activeTab === 'radar') {
        fetchRadarData(user.id, period);
      }
    }
  };

  const handleLogout = async () => {
    await logout();
    router.push('/login');
  };

  // Calculate health score color and percentage
  const getHealthColor = (score: number) => {
    const percentage = (score / 3) * 100;
    if (percentage >= 66) return 'bg-green-100 text-green-800 border-green-300';
    if (percentage >= 33) return 'bg-yellow-100 text-yellow-800 border-yellow-300';
    return 'bg-red-100 text-red-800 border-red-300';
  };

  const formatHealthScore = (score: number) => {
    return score.toFixed(1);
  };

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
    if (!dashboardData) return;
    const periodLabel = selectedPeriod || 'All Periods';

    // Sheet 1: Team summary
    const teamRows = dashboardData.teams.map(t => ({
      Team: t.teamName,
      'Overall Health Score': parseFloat(t.overallHealth.toFixed(2)),
      Band: getScoreBandLabel(t.overallHealth),
      'Submission Count': t.submissionCount,
    }));
    const teamsSheet = XLSX.utils.json_to_sheet(teamRows);

    // Sheet 2: Per-team dimension breakdown
    const dimensionRows = dashboardData.teams.flatMap(t =>
      t.dimensions.map(d => {
        const dimInfo = HEALTH_DIMENSIONS.find(hd => hd.id === d.dimensionId);
        return {
          Team: t.teamName,
          Dimension: dimInfo?.name || d.dimensionId,
          'Average Score': parseFloat(d.avgScore.toFixed(2)),
          Band: getScoreBandLabel(d.avgScore),
          'Response Count': d.responseCount,
        };
      })
    );
    const dimensionsSheet = XLSX.utils.json_to_sheet(dimensionRows);

    // Sheet 3: Action items across all teams
    const { listActionItems } = await import('@/lib/api/action-items');
    const allActionItems = await Promise.all(
      dashboardData.teams.map(t =>
        listActionItems(t.teamId).then(items => items.map(a => ({ ...a, teamName: t.teamName }))).catch(() => [])
      )
    );
    const actionRows = allActionItems.flat().map(a => ({
      Team: a.teamName,
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
    XLSX.utils.book_append_sheet(wb, teamsSheet, 'Team Summary');
    XLSX.utils.book_append_sheet(wb, dimensionsSheet, 'Dimension Breakdown');
    XLSX.utils.book_append_sheet(wb, actionsSheet, 'Action Items');

    const fileName = `health-check-manager-${periodLabel.replace(/\s+/g, '-').toLowerCase()}.xlsx`;
    XLSX.writeFile(wb, fileName);
  };

  const dimSparklines = HEALTH_DIMENSIONS.map((dim) => {
    const data = trendsData.map((t) => ({
      period: t.period as string,
      value: (t[dim.id] as number) || 0,
    }));
    const validData = data.filter((p) => p.value > 0);
    const latest = validData.length > 0 ? validData[validData.length - 1].value : 0;
    const prev = validData.length > 1 ? validData[validData.length - 2].value : latest;
    const direction: 'up' | 'down' | 'stable' =
      latest > prev + 0.1 ? 'up' : latest < prev - 0.1 ? 'down' : 'stable';
    return { dim, data, latest, direction };
  }).filter((d) => d.data.some((p) => p.value > 0));

  if (!user) return null;

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
                <Users className="w-8 h-8 text-indigo-600" />
              )}
              <div>
                <h1 className="text-2xl font-bold text-gray-900">Manager Dashboard</h1>
                <p className="text-gray-500">{brandingName ? `${brandingName} Health Overview` : 'Team Health Overview'}</p>
              </div>
            </div>

            <div className="flex items-center gap-4">
              <div className="text-right">
                <p className="text-sm font-semibold text-gray-900">{user.name}</p>
                <p className="text-xs text-gray-500">Manager</p>
              </div>

              {user.canTakeSurvey && (
                <button
                  onClick={() => router.push('/survey')}
                  data-testid="take-survey-button"
                  className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
                >
                  <ClipboardList className="w-4 h-4" />
                  Take Survey
                </button>
              )}

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
        {/* Assessment Period Filter + Score Band Legend + Export */}
        <div className="mb-4 flex justify-between items-center">
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
            {dashboardData && dashboardData.teams.length > 0 && (
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

        {/* Score Band Legend */}
        <div data-testid="score-band-legend" className="mb-6 flex flex-wrap items-center gap-3 p-3 bg-gray-50 rounded-lg border border-gray-200 text-xs font-medium">
          <span className="text-gray-500 font-semibold">Score bands:</span>
          <span className="px-2.5 py-1 rounded-full bg-emerald-100 text-emerald-800">2.7 – 3.0 Excellent</span>
          <span className="px-2.5 py-1 rounded-full bg-green-100 text-green-800">2.3 – 2.6 Good</span>
          <span className="px-2.5 py-1 rounded-full bg-yellow-100 text-yellow-800">1.7 – 2.2 Fair</span>
          <span className="px-2.5 py-1 rounded-full bg-red-100 text-red-800">1.0 – 1.6 Poor</span>
        </div>

        {/* Tab Navigation */}
        <div className="mb-6 border-b border-gray-200">
          <nav className="-mb-px flex space-x-8 overflow-x-auto">
            <button
              onClick={() => setActiveTab('teams')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap ${
                activeTab === 'teams'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Team Cards
            </button>
            <button
              data-testid="radar-tab"
              onClick={() => setActiveTab('radar')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center gap-2 ${
                activeTab === 'radar'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <Activity className="w-4 h-4" />
              Radar
            </button>
            <button
              data-testid="trends-tab"
              onClick={() => setActiveTab('trends')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center gap-2 ${
                activeTab === 'trends'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <LineChartIcon className="w-4 h-4" />
              Trends
            </button>
            <button
              data-testid="hierarchy-tab"
              onClick={() => setActiveTab('hierarchy')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap ${
                activeTab === 'hierarchy'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Hierarchy View
            </button>
            <button
              data-testid="summary-tab"
              onClick={() => setActiveTab('summary')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap ${
                activeTab === 'summary'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Summary View
            </button>
            <button
              data-testid="comparison-tab"
              onClick={() => setActiveTab('comparison')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap ${
                activeTab === 'comparison'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Comparison
            </button>
            <button
              data-testid="manager-actions-tab"
              onClick={() => setActiveTab('actions')}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center gap-2 ${
                activeTab === 'actions'
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <ListTodo className="w-4 h-4" />
              Actions
            </button>
          </nav>
        </div>

        {/* Loading State */}
        {loading && (
          <div className="bg-white rounded-xl shadow-sm border p-12 text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-gray-600">Loading team health data...</p>
          </div>
        )}

        {/* Error State */}
        {error && !loading && (
          <div className="bg-red-50 border border-red-300 rounded-xl p-6 flex items-center gap-4">
            <AlertCircle className="w-8 h-8 text-red-600" />
            <div>
              <h3 className="font-semibold text-red-900">Error Loading Dashboard</h3>
              <p className="text-red-700">{error}</p>
            </div>
          </div>
        )}

        {/* Empty State */}
        {!loading && !error && dashboardData && dashboardData.teams.length === 0 && activeTab !== 'radar' && activeTab !== 'trends' && (
          <div className="bg-white rounded-xl shadow-sm border p-12 text-center">
            <Users className="w-16 h-16 text-gray-400 mx-auto mb-4" />
            <h3 className="text-xl font-semibold text-gray-900 mb-2">No teams found</h3>
            <p className="text-gray-600">
              {selectedPeriod
                ? `No teams have submitted health checks for ${selectedPeriod}`
                : 'You are not currently supervising any teams'}
            </p>
          </div>
        )}

        {/* Radar Tab */}
        {!loading && !error && activeTab === 'radar' && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h3 className="text-xl font-semibold text-gray-900 mb-6">Aggregated Health Overview</h3>
            {radarLoading ? (
              <div className="flex justify-center items-center py-12">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
              </div>
            ) : radarData.length > 0 ? (
              <div data-testid="vp-radar-chart">
                <ResponsiveContainer width="100%" height={500}>
                  <RadarChart data={radarData}>
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
              <p className="text-gray-500 text-center py-12">No health data available for radar chart</p>
            )}
          </div>
        )}

        {/* Trends Tab */}
        {!loading && !error && activeTab === 'trends' && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <div className="flex justify-between items-center mb-6">
              <div>
                <h3 className="text-xl font-semibold text-gray-900">Health Trends Over Time</h3>
                {trendsView === 'dimensions' && trendsData.length > 0 && (
                  <p className="text-xs text-gray-400 mt-0.5">
                    One card per dimension — score vs. time
                    {selectedTrendTeam === ''
                      ? ' · averaged across all teams'
                      : ''}
                  </p>
                )}
              </div>
              {trendsData.length > 0 && !trendsLoading && (
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

            {/* Team filter pills */}
            {dashboardData && dashboardData.teams.length > 0 && (
              <div className="mb-5">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="text-xs font-medium text-gray-500 mr-1">Filter by team:</span>
                  <button
                    onClick={() => setSelectedTrendTeam('')}
                    className={`px-3 py-1 rounded-full text-xs font-medium transition-colors border ${
                      selectedTrendTeam === ''
                        ? 'bg-indigo-600 text-white border-indigo-600'
                        : 'bg-white text-gray-600 border-gray-300 hover:border-indigo-400 hover:text-indigo-600'
                    }`}
                  >
                    All Teams
                  </button>
                  {dashboardData.teams.map((team) => (
                    <button
                      key={team.teamId}
                      onClick={() => setSelectedTrendTeam(team.teamId)}
                      className={`px-3 py-1 rounded-full text-xs font-medium transition-colors border ${
                        selectedTrendTeam === team.teamId
                          ? 'bg-indigo-600 text-white border-indigo-600'
                          : 'bg-white text-gray-600 border-gray-300 hover:border-indigo-400 hover:text-indigo-600'
                      }`}
                    >
                      {team.teamName}
                    </button>
                  ))}
                </div>
                {selectedTrendTeam !== '' && (
                  <p className="text-xs text-indigo-500 mt-2 flex items-center gap-1">
                    <span className="inline-block w-1.5 h-1.5 rounded-full bg-indigo-500"></span>
                    Showing trends for <strong>{dashboardData.teams.find(t => t.teamId === selectedTrendTeam)?.teamName}</strong> only
                  </p>
                )}
              </div>
            )}

            {trendsLoading ? (
              <div className="flex justify-center items-center py-12">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
              </div>
            ) : trendsData.length > 0 ? (
              <>
                {trendsView === 'dimensions' ? (
                  /* Small multiples — one sparkline card per dimension */
                  <div data-testid="vp-trends-chart" className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
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
                                  dot={{ r: 2, fill: lineColor }}
                                  activeDot={{ r: 3 }}
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
                                  labelStyle={{ color: '#e5e7eb', fontWeight: 600, marginBottom: '2px' }}
                                  itemStyle={{ color: '#d1fae5' }}
                                  cursor={{ stroke: '#6366f1', strokeWidth: 1, strokeDasharray: '3 3' }}
                                  formatter={(v: number) => [v.toFixed(2), 'Score']}
                                  labelFormatter={(period) => period}
                                />
                              </LineChart>
                            </ResponsiveContainer>
                          ) : (
                            <div className="h-[52px] flex items-center justify-center text-xs text-gray-300">
                              Single period
                            </div>
                          )}

                          {/* Latest period label */}
                          {data.length > 0 && (
                            <p className="text-xs text-gray-400 truncate text-right">
                              {data[data.length - 1].period}
                            </p>
                          )}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  /* Overview — original 11-line chart */
                  <div data-testid="vp-trends-chart">
                    <ResponsiveContainer width="100%" height={500}>
                      <LineChart data={trendsData}>
                        <CartesianGrid strokeDasharray="3 3" />
                        <XAxis dataKey="period" />
                        <YAxis domain={[0, 3]} />
                        <Tooltip />
                        <Legend />
                        {HEALTH_DIMENSIONS.map((dim, idx) => (
                          <Line
                            key={dim.id}
                            type="monotone"
                            dataKey={dim.id}
                            name={dim.name}
                            stroke={`hsl(${idx * 32}, 70%, 50%)`}
                            strokeWidth={2}
                          />
                        ))}
                      </LineChart>
                    </ResponsiveContainer>
                  </div>
                )}
              </>
            ) : (
              <p className="text-gray-500 text-center py-12">No trend data available</p>
            )}
          </div>
        )}

        {/* Team Cards Tab */}
        {!loading && !error && dashboardData && dashboardData.teams.length > 0 && activeTab === 'teams' && (
          <div className="space-y-4">
            {dashboardData.teams.map((team) => (
              <div
                key={team.teamId}
                data-testid="team-health-card"
                className="bg-white rounded-xl shadow-sm border p-6 hover:shadow-md transition-shadow"
              >
                <div className="flex justify-between items-start mb-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h3
                        data-testid="team-name"
                        className="text-xl font-semibold text-gray-900"
                      >
                        {team.teamName}
                      </h3>
                      {team.postWorkshopStatus === 'submitted' ? (
                        <span
                          data-testid="workshop-status-badge"
                          className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800"
                        >
                          <CheckCircle className="w-3 h-3" />
                          Workshop Complete
                        </span>
                      ) : team.postWorkshopStatus === 'pending' ? (
                        <span
                          data-testid="workshop-status-badge"
                          className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800"
                        >
                          <Clock className="w-3 h-3" />
                          Pending Workshop
                        </span>
                      ) : null}
                    </div>
                    <div className="flex items-center gap-4 text-sm text-gray-600">
                      <span data-testid="submission-count">
                        <strong>{team.submissionCount}</strong>{' '}
                        {team.submissionCount === 1 ? 'submission' : 'submissions'}
                      </span>
                    </div>
                  </div>

                  <div className="text-right">
                    <div className="text-sm text-gray-600 mb-1">Overall Health</div>
                    <div
                      data-testid="team-health-score"
                      className={`text-3xl font-bold px-4 py-2 rounded-lg border-2 ${getHealthColor(
                        team.overallHealth
                      )}`}
                    >
                      {formatHealthScore(team.overallHealth)}
                    </div>
                    <div className="text-xs text-gray-500 mt-1">
                      {((team.overallHealth / 3) * 100).toFixed(0)}% health
                    </div>
                  </div>
                </div>

                {/* Dimension Breakdown */}
                {team.dimensions.length > 0 && (
                  <div className="mt-4 pt-4 border-t">
                    <button
                      data-testid="view-details-button"
                      onClick={() =>
                        setExpandedTeam(expandedTeam === team.teamId ? null : team.teamId)
                      }
                      className="flex items-center gap-2 text-sm text-indigo-600 hover:text-indigo-800 font-medium"
                    >
                      <ChevronDown
                        className={`w-4 h-4 transition-transform ${
                          expandedTeam === team.teamId ? 'rotate-180' : ''
                        }`}
                      />
                      {expandedTeam === team.teamId ? 'Hide' : 'View'} Dimension Details (
                      {team.dimensions.length})
                    </button>

                    {expandedTeam === team.teamId && (
                      <div className="mt-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                        {team.dimensions.map((dimension) => (
                          <div
                            key={dimension.dimensionId}
                            className="bg-gray-50 rounded-lg p-3 border"
                          >
                            <div className="flex justify-between items-center mb-1">
                              <h4 className="font-medium text-gray-900 capitalize text-sm">
                                {dimension.dimensionId === 'value'
                                  ? 'Delivering Value'
                                  : dimension.dimensionId}
                              </h4>
                              <span
                                className={`text-lg font-bold px-2 py-1 rounded ${getHealthColor(
                                  dimension.avgScore
                                )}`}
                              >
                                {formatHealthScore(dimension.avgScore)}
                              </span>
                            </div>
                            <div className="text-xs text-gray-500">
                              {dimension.responseCount}{' '}
                              {dimension.responseCount === 1 ? 'response' : 'responses'}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* No Dimensions State */}
                {team.dimensions.length === 0 && (
                  <div className="mt-4 pt-4 border-t">
                    <p className="text-sm text-gray-500 italic">No dimension data available</p>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Summary Stats (only show on team cards tab) */}
        {!loading && !error && dashboardData && dashboardData.teams.length > 0 && activeTab === 'teams' && (
          <div className="mt-8 bg-indigo-50 border border-indigo-200 rounded-xl p-6">
            <h3 className="font-semibold text-indigo-900 mb-3">Summary</h3>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
              <div>
                <span className="text-indigo-700 font-medium">Total Teams:</span>{' '}
                <span className="text-indigo-900 font-bold">{dashboardData.totalTeams}</span>
              </div>
              <div>
                <span className="text-indigo-700 font-medium">Total Submissions:</span>{' '}
                <span className="text-indigo-900 font-bold">
                  {dashboardData.teams.reduce((sum, t) => sum + t.submissionCount, 0)}
                </span>
              </div>
              <div>
                <span className="text-indigo-700 font-medium">Average Health:</span>{' '}
                <span className="text-indigo-900 font-bold">
                  {dashboardData.teams.length > 0
                    ? formatHealthScore(
                        dashboardData.teams.reduce((sum, t) => sum + t.overallHealth, 0) /
                          dashboardData.teams.length
                      )
                    : 'N/A'}
                </span>
              </div>
            </div>
            {dashboardData.assessmentPeriod && (
              <div className="mt-3 pt-3 border-t border-indigo-200">
                <span className="text-indigo-700 font-medium">Filtered by:</span>{' '}
                <span className="text-indigo-900 font-bold">{dashboardData.assessmentPeriod}</span>
              </div>
            )}
          </div>
        )}

        {/* Hierarchy View Tab */}
        {!loading && !error && activeTab === 'hierarchy' && (
          <div data-testid="hierarchy-tree" className="bg-white rounded-xl shadow-sm border p-6">
            <h3 className="text-xl font-semibold text-gray-900 mb-6">Organization Hierarchy</h3>

            {subordinates.length > 0 || user ? (
              <div className="space-y-4">
                {/* Current user as root */}
                {user && (
                  <HierarchyNode
                    person={{ id: user.id, name: user.name, hierarchyLevelId: user.hierarchyLevelId || user.hierarchyLevel || 'level-3', username: user.username, teamIds: user.teamIds || [] }}
                    childrenByParent={childrenByParent}
                    teamsByIdMap={teamsByIdMap}
                    depth={0}
                  />
                )}
              </div>
            ) : (
              <div className="text-center py-8 text-gray-500">
                <p>No organizational structure to display</p>
              </div>
            )}
          </div>
        )}

        {/* Summary View Tab */}
        {!loading && !error && activeTab === 'summary' && dashboardData && (
          <div className="space-y-6">
            {/* Overall Health Score */}
            <div className="bg-white rounded-xl shadow-sm border p-6">
              <h3 className="text-xl font-semibold text-gray-900 mb-4">Overall Health Metrics</h3>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                <div data-testid="overall-health-score" className="text-center p-6 bg-gradient-to-br from-indigo-50 to-indigo-100 rounded-lg">
                  <div className="text-sm text-indigo-700 font-medium mb-2">Overall Health Score</div>
                  <div className={`text-5xl font-bold mb-2 ${
                    dashboardData.teams.length > 0
                      ? getHealthColor(dashboardData.teams.reduce((sum, t) => sum + t.overallHealth, 0) / dashboardData.teams.length).replace('bg-', 'text-').replace('-100', '-600')
                      : 'text-gray-400'
                  }`}>
                    {dashboardData.teams.length > 0
                      ? formatHealthScore(
                          dashboardData.teams.reduce((sum, t) => sum + t.overallHealth, 0) /
                            dashboardData.teams.length
                        )
                      : 'N/A'}
                  </div>
                  <div className="text-sm text-indigo-600">
                    {dashboardData.teams.length > 0
                      ? `${((dashboardData.teams.reduce((sum, t) => sum + t.overallHealth, 0) / dashboardData.teams.length / 3) * 100).toFixed(0)}% health`
                      : 'No data'}
                  </div>
                </div>

                <div data-testid="total-teams" className="text-center p-6 bg-gradient-to-br from-blue-50 to-blue-100 rounded-lg">
                  <div className="text-sm text-blue-700 font-medium mb-2">Total Teams</div>
                  <div className="text-5xl font-bold text-blue-600 mb-2">
                    {dashboardData.totalTeams}
                  </div>
                  <div className="text-sm text-blue-600">Active teams</div>
                </div>

                <div className="text-center p-6 bg-gradient-to-br from-purple-50 to-purple-100 rounded-lg">
                  <div className="text-sm text-purple-700 font-medium mb-2">Total Submissions</div>
                  <div className="text-5xl font-bold text-purple-600 mb-2">
                    {dashboardData.teams.reduce((sum, t) => sum + t.submissionCount, 0)}
                  </div>
                  <div className="text-sm text-purple-600">Health check responses</div>
                </div>
              </div>
            </div>

            {/* Recent Activity */}
            <div data-testid="recent-activity" className="bg-white rounded-xl shadow-sm border p-6">
              <h3 className="text-xl font-semibold text-gray-900 mb-4">Recent Activity</h3>

              {dashboardData.teams.length > 0 ? (
                <div className="space-y-3">
                  {dashboardData.teams
                    .sort((a, b) => b.submissionCount - a.submissionCount)
                    .slice(0, 5)
                    .map((team) => (
                      <div key={team.teamId} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg border">
                        <div className="flex items-center gap-3">
                          <Users className="w-5 h-5 text-gray-600" />
                          <div>
                            <div className="font-medium text-gray-900">{team.teamName}</div>
                            <div className="text-sm text-gray-600">
                              {team.submissionCount} submission{team.submissionCount !== 1 ? 's' : ''}
                            </div>
                          </div>
                        </div>
                        <div className={`px-4 py-2 rounded-lg font-bold ${getHealthColor(team.overallHealth)}`}>
                          {formatHealthScore(team.overallHealth)}
                        </div>
                      </div>
                    ))}
                </div>
              ) : (
                <div className="text-center py-8 text-gray-500">
                  <p>No recent activity to display</p>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Comparison Tab */}
        {!loading && !error && activeTab === 'comparison' && dashboardData && (
          <div className="bg-white rounded-xl shadow-sm border p-6">
            <h3 className="text-xl font-semibold text-gray-900 mb-4">Team Comparison</h3>

            {/* Team Selector */}
            <div className="mb-6">
              <label htmlFor="team-selector" className="block text-sm font-medium text-gray-700 mb-2">
                Select teams to compare:
              </label>
              <select
                id="team-selector"
                data-testid="team-selector"
                multiple
                value={selectedTeamsForComparison}
                onChange={(e) => {
                  const selected = Array.from(e.target.selectedOptions, option => option.value);
                  setSelectedTeamsForComparison(selected);
                }}
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent text-gray-900 bg-white"
                size={Math.min(dashboardData.teams.length, 5)}
              >
                {dashboardData.teams.map((team) => (
                  <option key={team.teamId} value={team.teamId}>
                    {team.teamName} - Health: {formatHealthScore(team.overallHealth)}
                  </option>
                ))}
              </select>
              <p className="mt-1 text-sm text-gray-500">Hold Ctrl (Cmd on Mac) to select multiple teams</p>
            </div>

            {/* Comparison Table */}
            {selectedTeamsForComparison.length > 0 ? (
              <div data-testid="comparison-table" className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Metric
                      </th>
                      {selectedTeamsForComparison.map((teamId) => {
                        const team = dashboardData.teams.find((t) => t.teamId === teamId);
                        return (
                          <th key={teamId} className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            {team?.teamName}
                          </th>
                        );
                      })}
                    </tr>
                  </thead>
                  <tbody className="bg-white divide-y divide-gray-200">
                    <tr>
                      <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                        Overall Health
                      </td>
                      {selectedTeamsForComparison.map((teamId) => {
                        const team = dashboardData.teams.find((t) => t.teamId === teamId);
                        return (
                          <td key={teamId} className="px-6 py-4 whitespace-nowrap">
                            <span className={`px-3 py-1 rounded-lg font-bold ${team ? getHealthColor(team.overallHealth) : ''}`}>
                              {team ? formatHealthScore(team.overallHealth) : 'N/A'}
                            </span>
                          </td>
                        );
                      })}
                    </tr>
                    <tr>
                      <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                        Submissions
                      </td>
                      {selectedTeamsForComparison.map((teamId) => {
                        const team = dashboardData.teams.find((t) => t.teamId === teamId);
                        return (
                          <td key={teamId} className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                            {team?.submissionCount || 0}
                          </td>
                        );
                      })}
                    </tr>
                    <tr>
                      <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                        Dimensions Tracked
                      </td>
                      {selectedTeamsForComparison.map((teamId) => {
                        const team = dashboardData.teams.find((t) => t.teamId === teamId);
                        return (
                          <td key={teamId} className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                            {team?.dimensions.length || 0}
                          </td>
                        );
                      })}
                    </tr>
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="text-center py-8 text-gray-500">
                <p>Select teams from the dropdown above to compare their metrics</p>
              </div>
            )}
          </div>
        )}
        {/* Actions Tab */}
        {!loading && !error && activeTab === 'actions' && (
          <div className="bg-white rounded-xl shadow-sm border p-6" data-testid="manager-actions-panel">
            <h3 className="text-xl font-semibold text-gray-900 mb-1">Action Items Across Teams</h3>
            <p className="text-sm text-gray-500 mb-6">Open and in-progress action items per team supervised by you.</p>
            {actionSummaryLoading ? (
              <div className="flex items-center justify-center py-12">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
              </div>
            ) : actionSummary.length === 0 ? (
              <div className="text-center py-12 text-gray-400">
                <ListTodo className="w-10 h-10 mx-auto mb-3 text-gray-200" />
                <p className="font-medium text-gray-500">No action items found</p>
                <p className="text-sm mt-1">Team Leads create action items on their team dashboards.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Team</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Open / In Progress</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                    </tr>
                  </thead>
                  <tbody className="bg-white divide-y divide-gray-200">
                    {actionSummary.map((row) => (
                      <tr key={row.teamId} data-testid="manager-action-row">
                        <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">{row.teamName}</td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-700">{row.openCount}</td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          {row.openCount === 0 ? (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-green-50 text-green-700">
                              <CheckCircle className="w-3 h-3" /> All done
                            </span>
                          ) : row.openCount <= 3 ? (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-yellow-50 text-yellow-700">
                              <Clock className="w-3 h-3" /> In progress
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-red-50 text-red-700">
                              <AlertCircle className="w-3 h-3" /> Needs attention
                            </span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
      {showOnboarding && user && (
        <OnboardingModal
          userLevel={(user as any).hierarchyLevelId}
          onDismiss={() => {
            localStorage.setItem(`onboarding_complete:${(user as any).id}`, 'true');
            setShowOnboarding(false);
          }}
        />
      )}
    </div>
  );
}
