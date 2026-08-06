'use client';

import { useState, useEffect, useRef, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { getCurrentUser, logout } from '@/lib/auth';
import { HEALTH_DIMENSIONS } from '@/lib/data';
import { HealthCheckResponse } from '@/lib/types';
import { getAssessmentPeriod, toCadence } from '@/lib/assessment-period';
import { submitHealthCheck, formatDateForAPI, HealthCheckAPIError } from '@/lib/api/health-checks';
import { getTeamInfoCached, TeamInfo, TeamsAPIError } from '@/lib/api/teams';
import { TrendingUp, TrendingDown, Minus, ChevronLeft, ChevronRight, Save, LogOut, CheckCircle, BarChart3, Loader2, AlertCircle, Info, X } from 'lucide-react';

interface SurveyDraft {
  responses: HealthCheckResponse[];
  currentDimension: number;
  assessmentPeriod: string;
  savedAt: string;
}

function getDraftKey(userId: string, teamId: string): string {
  return `surveyDraft:${userId}:${teamId}`;
}

export default function SurveyPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
        <div className="bg-white rounded-2xl shadow-xl p-12 max-w-md w-full text-center">
          <Loader2 className="w-16 h-16 text-indigo-600 mx-auto mb-6 animate-spin" />
          <h1 className="text-2xl font-bold text-gray-900 mb-4">Loading...</h1>
        </div>
      </div>
    }>
      <SurveyPageContent />
    </Suspense>
  );
}

function SurveyPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const surveyType = searchParams.get('type') === 'post_workshop' ? 'post_workshop' : 'individual';
  const isPostWorkshop = surveyType === 'post_workshop';
  const preferredTeamId = searchParams.get('team');
  const [user, setUser] = useState<any>(null);
  const [team, setTeam] = useState<TeamInfo | null>(null);
  const [teamLoading, setTeamLoading] = useState(true);
  const [teamError, setTeamError] = useState<string | null>(null);
  const [currentDimension, setCurrentDimension] = useState(0); // Start at first health dimension
  const [responses, setResponses] = useState<HealthCheckResponse[]>([]);
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [draftRestored, setDraftRestored] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const draftInitialized = useRef(false);
  const [teamOptions, setTeamOptions] = useState<{id: string, name: string}[]>([]);
  const [showHelpPanel, setShowHelpPanel] = useState(false);
  const helpAutoShown = useRef(false);

  useEffect(() => {
    const currentUser = getCurrentUser();
    if (!currentUser) {
      router.push('/login');
    } else {
      setUser(currentUser);
      // Use preferred team from query param (e.g. navigating from dashboard) or fall back to first team
      const teamId = (preferredTeamId && currentUser.teamIds?.includes(preferredTeamId))
        ? preferredTeamId
        : (currentUser.teamIds && currentUser.teamIds.length > 0 ? currentUser.teamIds[0] : null);
      if (teamId) {
        setTeamLoading(true);
        getTeamInfoCached(teamId)
          .then((teamInfo) => {
            setTeam(teamInfo);
            // Restore draft after team info loaded
            try {
              const draftJson = localStorage.getItem(getDraftKey(currentUser.id, teamId));
              if (draftJson) {
                const draft: SurveyDraft = JSON.parse(draftJson);
                const currentPeriod = getAssessmentPeriod(new Date(), toCadence(teamInfo.cadence));
                if (draft.assessmentPeriod === currentPeriod && draft.responses.length > 0) {
                  setResponses(draft.responses);
                  setCurrentDimension(draft.currentDimension);
                  setDraftRestored(true);
                }
              }
            } catch {
              // Ignore corrupt draft
            }
            draftInitialized.current = true;
            setTeamLoading(false);
          })
          .catch((err) => {
            console.error('Failed to fetch team info:', err);
            setTeamError(err instanceof TeamsAPIError ? err.message : 'Failed to load team information');
            setTeamLoading(false);
          });
        // Fetch team names for multi-team selector
        if (currentUser.teamIds.length > 1) {
          Promise.all(
            currentUser.teamIds.map((tid: string) =>
              getTeamInfoCached(tid).then(info => ({ id: info.id, name: info.name })).catch(() => ({ id: tid, name: tid }))
            )
          ).then(setTeamOptions);
        }
      } else {
        setTeamLoading(false);
        setTeamError('No team assigned to this user');
      }
    }
  }, [router, preferredTeamId]);

  const handleTeamSwitch = (newTeamId: string) => {
    if (!user || !team) return;

    // Flush any pending draft save for the current team before switching
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
      debounceRef.current = null;
    }
    if (responses.length > 0 && draftInitialized.current && !submitted) {
      const draft: SurveyDraft = {
        responses,
        currentDimension,
        assessmentPeriod: getAssessmentPeriod(new Date(), toCadence(team.cadence)),
        savedAt: new Date().toISOString(),
      };
      localStorage.setItem(getDraftKey(user.id, team.id), JSON.stringify(draft));
    }

    setTeamLoading(true);
    setResponses([]);
    setCurrentDimension(0);
    setDraftRestored(false);
    setSubmitted(false);
    setError(null);
    setValidationError(null);
    draftInitialized.current = false;

    getTeamInfoCached(newTeamId)
      .then((teamInfo) => {
        setTeam(teamInfo);
        // Restore draft for the new team
        try {
          const draftJson = localStorage.getItem(getDraftKey(user.id, newTeamId));
          if (draftJson) {
            const draft: SurveyDraft = JSON.parse(draftJson);
            const currentPeriod = getAssessmentPeriod(new Date(), toCadence(teamInfo.cadence));
            if (draft.assessmentPeriod === currentPeriod && draft.responses.length > 0) {
              setResponses(draft.responses);
              setCurrentDimension(draft.currentDimension);
              setDraftRestored(true);
            }
          }
        } catch {
          // Ignore corrupt draft
        }
        draftInitialized.current = true;
        setTeamLoading(false);
      })
      .catch((err) => {
        console.error('Failed to fetch team info:', err);
        setTeamError(err instanceof TeamsAPIError ? err.message : 'Failed to load team information');
        setTeamLoading(false);
      });
  };

  // Autosave draft to localStorage (debounced)
  useEffect(() => {
    if (!user || !team || !draftInitialized.current || submitted) return;
    if (responses.length === 0) return;

    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      const draft: SurveyDraft = {
        responses,
        currentDimension,
        assessmentPeriod: getAssessmentPeriod(new Date(), toCadence(team.cadence)),
        savedAt: new Date().toISOString(),
      };
      localStorage.setItem(getDraftKey(user.id, team.id), JSON.stringify(draft));
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [responses, currentDimension, user, team, submitted]);

  // beforeunload warning when survey has unsaved responses
  useEffect(() => {
    if (responses.length === 0 || submitted) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [responses.length, submitted]);

  // Auto-show help panel once on the first dimension for first-time survey takers.
  // Must be here (before any early returns) to comply with Rules of Hooks.
  useEffect(() => {
    if (
      user &&
      user.hierarchyLevelId === 'level-5' &&
      !helpAutoShown.current &&
      currentDimension === 0 &&
      !localStorage.getItem(`survey_help_seen:${user.id}`)
    ) {
      helpAutoShown.current = true;
      localStorage.setItem(`survey_help_seen:${user.id}`, 'true');
      let t2: ReturnType<typeof setTimeout>;
      const t1 = setTimeout(() => {
        setShowHelpPanel(true);
        t2 = setTimeout(() => setShowHelpPanel(false), 3000);
      }, 600);
      return () => {
        clearTimeout(t1);
        clearTimeout(t2);
      };
    }
  }, [user, currentDimension]);

  const handleScoreSelect = (score: 1 | 2 | 3) => {
    const dimension = HEALTH_DIMENSIONS[currentDimension];
    const existingIndex = responses.findIndex(r => r.dimensionId === dimension.id);

    if (existingIndex >= 0) {
      const newResponses = [...responses];
      newResponses[existingIndex] = { ...newResponses[existingIndex], score };
      setResponses(newResponses);
    } else {
      setResponses([...responses, {
        dimensionId: dimension.id,
        score,
        trend: 'stable',
        comment: ''
      }]);
    }

    // Clear validation error when user makes a selection
    setValidationError(null);
  };

  const handleTrendSelect = (trend: 'improving' | 'stable' | 'declining') => {
    const dimension = HEALTH_DIMENSIONS[currentDimension];
    const existingIndex = responses.findIndex(r => r.dimensionId === dimension.id);

    if (existingIndex >= 0) {
      const newResponses = [...responses];
      newResponses[existingIndex].trend = trend;
      setResponses(newResponses);
    }

    // Clear validation error when user makes a selection
    setValidationError(null);
  };

  const handleCommentChange = (comment: string) => {
    const dimension = HEALTH_DIMENSIONS[currentDimension];
    const existingIndex = responses.findIndex(r => r.dimensionId === dimension.id);
    
    if (existingIndex >= 0) {
      const newResponses = [...responses];
      newResponses[existingIndex].comment = comment;
      setResponses(newResponses);
    }
  };

  const getCurrentResponse = () => {
    const dimension = HEALTH_DIMENSIONS[currentDimension];
    return responses.find(r => r.dimensionId === dimension.id);
  };

  const handleNext = () => {
    const currentResponse = getCurrentResponse();

    // Validate that both score and trend are selected
    if (!currentResponse?.score) {
      setValidationError('Please select a score (Red, Yellow, or Green) before continuing.');
      return;
    }

    if ((!isTeamMember || isPostWorkshop) && !currentResponse?.trend) {
      setValidationError('Please select a trend (Improving, Stable, or Declining) before continuing.');
      return;
    }

    // Clear validation error and proceed
    setValidationError(null);
    if (currentDimension < HEALTH_DIMENSIONS.length - 1) {
      setCurrentDimension(currentDimension + 1);
    }
  };

  const handlePrevious = () => {
    // Clear validation error when navigating
    setValidationError(null);
    if (currentDimension > 0) {
      setCurrentDimension(currentDimension - 1);
    }
  };

  const handleSubmit = async () => {
    if (!user || !team || submitting) return;

    // Validate all responses are complete
    if (responses.length !== HEALTH_DIMENSIONS.length) {
      setValidationError('Please fill out all health check dimensions before submitting.');
      return;
    }

    // Validate each response has both score and trend (trend only required for non-team-members)
    const incompleteResponses = responses.filter(r => !r.score || ((!isTeamMember || isPostWorkshop) && !r.trend));
    if (incompleteResponses.length > 0) {
      setValidationError((!isTeamMember || isPostWorkshop)
        ? 'Please select both a score and trend for all dimensions before submitting.'
        : 'Please select a score for all dimensions before submitting.'
      );
      return;
    }

    setSubmitting(true);
    setError(null);
    setValidationError(null);

    try {
      // Automatically determine assessment period based on submission date
      const submissionDate = new Date();
      const assessmentPeriod = getAssessmentPeriod(submissionDate, toCadence(team.cadence));

      // Submit to backend API using team from API response
      const session = await submitHealthCheck({
        teamId: team.id,
        userId: user.id,
        date: formatDateForAPI(submissionDate),
        assessmentPeriod,
        surveyType: isPostWorkshop ? 'post_workshop' : 'individual',
        responses: responses.map(r => ({
          dimensionId: r.dimensionId,
          score: r.score,
          trend: r.trend,
          comment: r.comment || ''
        })),
        completed: true
      });

      // Clear draft on successful submit
      localStorage.removeItem(getDraftKey(user.id, team.id));

      // Store session ID and redirect
      setSessionId(session.id);
      setSubmitted(true);

      // Post-workshop redirects to dashboard, individual to home
      router.push(isPostWorkshop ? '/dashboard' : '/home');
    } catch (err) {
      console.error('Failed to submit health check:', err);

      if (err instanceof HealthCheckAPIError) {
        setError(err.message || 'Failed to submit your responses. Please try again.');
      } else {
        setError('An unexpected error occurred. Please check your connection and try again.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = () => {
    logout();
    router.push('/login');
  };

  if (!user) return null;

  // Show loading state while fetching team info
  if (teamLoading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
        <div className="bg-white rounded-2xl shadow-xl p-12 max-w-md w-full text-center">
          <Loader2 className="w-16 h-16 text-indigo-600 mx-auto mb-6 animate-spin" />
          <h1 className="text-2xl font-bold text-gray-900 mb-4">Loading...</h1>
          <p className="text-gray-600">Fetching your team information</p>
        </div>
      </div>
    );
  }

  // Show error state if team fetch failed
  if (teamError || !team) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-red-50 to-orange-100 flex items-center justify-center p-4">
        <div className="bg-white rounded-2xl shadow-xl p-12 max-w-md w-full text-center">
          <AlertCircle className="w-16 h-16 text-red-500 mx-auto mb-6" />
          <h1 className="text-2xl font-bold text-gray-900 mb-4">Unable to Load Team</h1>
          <p className="text-gray-600 mb-6">{teamError || 'No team assigned to your account. Please contact your administrator.'}</p>
          <button
            onClick={handleLogout}
            className="bg-gray-600 text-white px-6 py-3 rounded-lg font-semibold hover:bg-gray-700 transition-colors"
          >
            Back to Login
          </button>
        </div>
      </div>
    );
  }

  if (submitted) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100 flex items-center justify-center p-4">
        <div className="bg-white rounded-2xl shadow-xl p-12 max-w-md w-full text-center">
          <CheckCircle className="w-20 h-20 text-green-500 mx-auto mb-6" />
          <h1 className="text-3xl font-bold text-gray-900 mb-4">Thank You!</h1>
          <p className="text-gray-600 mb-4">Your health check responses have been submitted successfully.</p>
          {sessionId && (
            <p className="text-sm text-gray-500 mb-8 font-mono">Session ID: {sessionId}</p>
          )}
          <button
            onClick={() => router.push('/home')}
            className="bg-indigo-600 text-white px-6 py-3 rounded-lg font-semibold hover:bg-indigo-700 transition-colors"
          >
            Back to Home
          </button>
        </div>
      </div>
    );
  }

  const totalQuestions = HEALTH_DIMENSIONS.length; // Total health dimensions
  const currentQuestionNumber = currentDimension + 1; // +1 for display (1-indexed)
  const progress = (currentQuestionNumber / totalQuestions) * 100;
  const isLastDimension = currentDimension === HEALTH_DIMENSIONS.length - 1;
  const canSubmit = responses.length === HEALTH_DIMENSIONS.length;

  const dimension = HEALTH_DIMENSIONS[currentDimension];
  const currentResponse = getCurrentResponse();

  const isTeamLead = user.hierarchyLevelId === 'level-4';
  const isTeamMember = user.hierarchyLevelId === 'level-5';

  // Compute display period from team cadence
  const surveyPeriod = team ? getAssessmentPeriod(new Date(), toCadence(team.cadence)) : '';

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="container mx-auto p-4 max-w-4xl">
        <div className="bg-white rounded-2xl shadow-xl overflow-hidden">
          <div className={`${isPostWorkshop ? 'bg-amber-600' : 'bg-indigo-600'} p-6 text-white`}>
            {isPostWorkshop && (
              <div className="mb-3 px-3 py-2 bg-amber-700/50 rounded-lg text-sm">
                Record your team&apos;s consensus from the workshop discussion
              </div>
            )}
            <div className="flex justify-between items-center mb-4">
              <div>
                <h1 className="text-2xl font-bold">{isPostWorkshop ? 'Post-Workshop Survey' : 'Squad Health Check'}</h1>
                <p className={isPostWorkshop ? 'text-amber-200' : 'text-indigo-200'}>
                  Team: {teamOptions.length > 1 ? (
                    <select
                      data-testid="team-selector"
                      value={team?.id || ''}
                      onChange={(e) => handleTeamSwitch(e.target.value)}
                      className="ml-1 px-2 py-0.5 text-sm bg-white/20 border border-white/30 rounded text-white focus:ring-2 focus:ring-white/50"
                    >
                      {teamOptions.map((t) => (
                        <option key={t.id} value={t.id} className="text-gray-900">{t.name}</option>
                      ))}
                    </select>
                  ) : (team?.name || 'Unknown Team')}
                </p>
                <p className={`${isPostWorkshop ? 'text-amber-100' : 'text-indigo-100'} text-sm mt-1`}>
                  Period: {surveyPeriod}
                  {team?.cadence && ` • ${team.cadence.charAt(0).toUpperCase() + team.cadence.slice(1)} Check`}
                </p>
              </div>
              <div className="text-right">
                <p className={`text-sm ${isPostWorkshop ? 'text-amber-200' : 'text-indigo-200'}`}>Logged in as</p>
                <p className="font-semibold">{user.name}</p>
                <div className="mt-2 flex flex-col gap-1">
                  {isTeamLead && (
                    <button
                      onClick={() => router.push('/dashboard')}
                      className={`flex items-center gap-1 text-sm ${isPostWorkshop ? 'text-amber-200' : 'text-indigo-200'} hover:text-white transition-colors`}
                    >
                      <BarChart3 className="w-4 h-4" />
                      View Team Dashboard
                    </button>
                  )}
                  <button
                    onClick={handleLogout}
                    className={`flex items-center gap-1 text-sm ${isPostWorkshop ? 'text-amber-200' : 'text-indigo-200'} hover:text-white transition-colors`}
                  >
                    <LogOut className="w-4 h-4" />
                    Logout
                  </button>
                </div>
              </div>
            </div>
            <div className={`w-full ${isPostWorkshop ? 'bg-amber-800' : 'bg-indigo-800'} rounded-full h-2`}>
              <div
                className="bg-white h-2 rounded-full transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
            <p className={`text-sm mt-2 ${isPostWorkshop ? 'text-amber-200' : 'text-indigo-200'}`}>
              Question {currentQuestionNumber} of {totalQuestions}
            </p>
          </div>

          <div className="p-8">
            {draftRestored && (
              <div
                data-testid="draft-restored-banner"
                className="mb-6 flex items-center justify-between gap-2 text-blue-700 text-sm bg-blue-50 border border-blue-200 p-3 rounded-lg"
              >
                <div className="flex items-center gap-2">
                  <Info className="w-4 h-4 flex-shrink-0" />
                  <span>Your previous progress has been restored from a saved draft.</span>
                </div>
                <button
                  onClick={() => setDraftRestored(false)}
                  className="text-blue-400 hover:text-blue-600 flex-shrink-0"
                  aria-label="Dismiss"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            )}

            {/* Score guidance panel (Team Members only) */}
            {isTeamMember && (
              <div className="mb-6">
                <div className="flex justify-end">
                  <button
                    onClick={() => setShowHelpPanel((v) => !v)}
                    className={`flex items-center gap-1.5 text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
                      showHelpPanel
                        ? 'bg-indigo-600 text-white border-indigo-600'
                        : 'text-indigo-600 border-indigo-300 hover:bg-indigo-50'
                    }`}
                    aria-label="Toggle scoring guide"
                    data-testid="survey-help-toggle"
                  >
                    <Info className="w-3.5 h-3.5" />
                    Scoring guide
                  </button>
                </div>
                {showHelpPanel && (
                  <div
                    className="mt-2 rounded-xl border border-indigo-100 bg-indigo-50 px-4 py-3 text-sm text-gray-700 space-y-2"
                    data-testid="survey-help-panel"
                  >
                    <p className="font-semibold text-indigo-800 text-xs uppercase tracking-wide">How to score</p>
                    <div className="flex items-start gap-2">
                      <span className="mt-1 w-3 h-3 rounded-full bg-red-500 flex-shrink-0" />
                      <span><span className="font-medium text-red-700">Red</span> — {dimension.badDescription}</span>
                    </div>
                    <div className="flex items-start gap-2">
                      <span className="mt-1 w-3 h-3 rounded-full bg-yellow-400 flex-shrink-0" />
                      <span><span className="font-medium text-yellow-700">Yellow</span> — Some problems, but we are working on it</span>
                    </div>
                    <div className="flex items-start gap-2">
                      <span className="mt-1 w-3 h-3 rounded-full bg-green-500 flex-shrink-0" />
                      <span><span className="font-medium text-green-700">Green</span> — {dimension.goodDescription}</span>
                    </div>
                  </div>
                )}
              </div>
            )}

            <div className="mb-8">
              <h2 className="text-3xl font-bold text-gray-900 mb-4">{dimension?.name}</h2>
              <p className="text-gray-600 text-lg">{dimension?.description}</p>
            </div>

            <div className="grid md:grid-cols-3 gap-4 mb-8">
              <button
                onClick={() => handleScoreSelect(1)}
                data-dimension={dimension.id}
                data-score="1"
                className={`p-6 rounded-xl border-2 transition-all ${
                  currentResponse?.score === 1
                    ? 'border-red-500 bg-red-50'
                    : 'border-gray-300 hover:border-red-300 hover:bg-red-50'
                }`}
              >
                <div className="w-12 h-12 mx-auto mb-3 rounded-full bg-red-500" />
                <h3 className="font-bold text-red-900 mb-2">Red</h3>
                <p className="text-sm text-gray-600">{dimension.badDescription}</p>
              </button>

              <button
                onClick={() => handleScoreSelect(2)}
                data-dimension={dimension.id}
                data-score="2"
                className={`p-6 rounded-xl border-2 transition-all ${
                  currentResponse?.score === 2
                    ? 'border-yellow-500 bg-yellow-50'
                    : 'border-gray-300 hover:border-yellow-300 hover:bg-yellow-50'
                }`}
              >
                <div className="w-12 h-12 mx-auto mb-3 rounded-full bg-yellow-500" />
                <h3 className="font-bold text-yellow-900 mb-2">Yellow</h3>
                <p className="text-sm text-gray-600">Some problems, but we are working on it</p>
              </button>

              <button
                onClick={() => handleScoreSelect(3)}
                data-dimension={dimension.id}
                data-score="3"
                className={`p-6 rounded-xl border-2 transition-all ${
                  currentResponse?.score === 3
                    ? 'border-green-500 bg-green-50'
                    : 'border-gray-300 hover:border-green-300 hover:bg-green-50'
                }`}
              >
                <div className="w-12 h-12 mx-auto mb-3 rounded-full bg-green-500" />
                <h3 className="font-bold text-green-900 mb-2">Green</h3>
                <p className="text-sm text-gray-600">{dimension.goodDescription}</p>
              </button>
            </div>

            {currentResponse?.score && (
              <div className="mb-8 p-6 bg-gray-50 rounded-xl">
                {(!isTeamMember || isPostWorkshop) && (
                  <>
                    <h3 className="font-semibold text-gray-900 mb-4">Trend</h3>
                    <div className="flex gap-4">
                      <button
                        onClick={() => handleTrendSelect('improving')}
                        data-dimension={dimension.id}
                        data-trend="improving"
                        className={`flex-1 p-3 rounded-lg border-2 flex items-center justify-center gap-2 transition-all ${
                          currentResponse?.trend === 'improving'
                            ? 'border-green-500 bg-green-50 text-green-700'
                            : 'border-gray-300 bg-white text-gray-700 hover:border-green-300 hover:bg-green-50'
                        }`}
                      >
                        <TrendingUp className="w-5 h-5" />
                        Improving
                      </button>
                      <button
                        onClick={() => handleTrendSelect('stable')}
                        data-dimension={dimension.id}
                        data-trend="stable"
                        className={`flex-1 p-3 rounded-lg border-2 flex items-center justify-center gap-2 transition-all ${
                          currentResponse?.trend === 'stable'
                            ? 'border-blue-500 bg-blue-50 text-blue-700'
                            : 'border-gray-300 bg-white text-gray-700 hover:border-blue-300 hover:bg-blue-50'
                        }`}
                      >
                        <Minus className="w-5 h-5" />
                        Stable
                      </button>
                      <button
                        onClick={() => handleTrendSelect('declining')}
                        data-dimension={dimension.id}
                        data-trend="declining"
                        className={`flex-1 p-3 rounded-lg border-2 flex items-center justify-center gap-2 transition-all ${
                          currentResponse?.trend === 'declining'
                            ? 'border-red-500 bg-red-50 text-red-700'
                            : 'border-gray-300 bg-white text-gray-700 hover:border-red-300 hover:bg-red-50'
                        }`}
                      >
                        <TrendingDown className="w-5 h-5" />
                        Declining
                      </button>
                    </div>
                  </>
                )}

                <div className={!isTeamMember ? 'mt-4' : ''}>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    Comments (optional)
                  </label>
                  <textarea
                    value={currentResponse?.comment || ''}
                    onChange={(e) => {
                      if (e.target.value.length <= 1000) {
                        handleCommentChange(e.target.value);
                      }
                    }}
                    maxLength={1000}
                    data-dimension={dimension.id}
                    className="w-full p-3 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent placeholder:text-gray-400"
                    rows={3}
                    placeholder="Add any additional context..."
                  />
                  <p className={`text-xs mt-1 text-right ${(currentResponse?.comment?.length || 0) >= 950 ? 'text-red-500' : 'text-gray-400'}`}>
                    {currentResponse?.comment?.length || 0}/1000
                  </p>
                </div>
              </div>
            )}

            {validationError && (
              <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3">
                <AlertCircle className="w-5 h-5 text-red-600 flex-shrink-0 mt-0.5" />
                <div className="flex-1">
                  <p className="text-sm font-semibold text-red-900 mb-1">Validation Error</p>
                  <p className="text-sm text-red-700">{validationError}</p>
                </div>
              </div>
            )}

            {error && (
              <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3">
                <AlertCircle className="w-5 h-5 text-red-600 flex-shrink-0 mt-0.5" />
                <div className="flex-1">
                  <p className="text-sm font-semibold text-red-900 mb-1">Submission Failed</p>
                  <p className="text-sm text-red-700">{error}</p>
                </div>
              </div>
            )}

            <div className="flex justify-between">
              <button
                onClick={handlePrevious}
                disabled={currentDimension === 0 || submitting}
                className={`flex items-center gap-2 px-6 py-3 rounded-lg font-semibold transition-colors ${
                  currentDimension === 0 || submitting
                    ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                    : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
                }`}
              >
                <ChevronLeft className="w-5 h-5" />
                Previous
              </button>

              {isLastDimension ? (
                <button
                  onClick={handleSubmit}
                  disabled={submitting}
                  type="submit"
                  className={`flex items-center gap-2 px-6 py-3 rounded-lg font-semibold transition-colors ${
                    submitting
                      ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                      : 'bg-green-600 text-white hover:bg-green-700'
                  }`}
                >
                  {submitting ? (
                    <>
                      <Loader2 className="w-5 h-5 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    <>
                      <Save className="w-5 h-5" />
                      Submit Responses
                    </>
                  )}
                </button>
              ) : (
                <button
                  onClick={handleNext}
                  disabled={submitting}
                  className={`flex items-center gap-2 px-6 py-3 rounded-lg font-semibold transition-colors ${
                    submitting
                      ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                      : 'bg-indigo-600 text-white hover:bg-indigo-700'
                  }`}
                >
                  Next
                  <ChevronRight className="w-5 h-5" />
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}