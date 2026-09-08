/**
 * Pure filtering/search logic for the survey-completion table's recursive
 * leadership hierarchy. Split out of SurveyCompletionDashboard.tsx so it
 * can be unit-tested without rendering anything.
 */

import type {
  SurveyCompletionGroup,
  SurveyCompletionPersonGroup,
  SurveyCompletionTeam,
  SurveyStatus,
} from '@/lib/api/survey-completion';

export type CardFilter = 'all' | 'optedIn' | SurveyStatus;

export function isRemindable(team: SurveyCompletionTeam): boolean {
  return team.status === 'in_progress' || team.status === 'not_started';
}

/**
 * A pod's completion percentage, rounded to the nearest whole percent from
 * its own completed/total counts. Safe when total is 0 (returns 0 rather
 * than NaN).
 */
export function getPodCompletionPercent(team: SurveyCompletionTeam): number {
  if (team.total <= 0) return 0;
  return Math.round((team.completed / team.total) * 100);
}

/**
 * A pod's EFFECTIVE display status -- scoped to individual pods only,
 * never applied to a Director/Manager (leadership) row. Only ever differs
 * from team.status for a pod that's in_progress or complete: rounding a
 * pod's completed/total ratio can make it read 100% (e.g. 199 of 200
 * members) while the backend still considers it "in_progress" (completed
 * < total). Showing "100% completed" next to an "In Progress" badge would
 * look like a contradiction, so once a pod's own rounded percentage
 * reaches 100 it always displays as the existing "complete" status
 * instead -- never a new status. not_started and opted_out pods are left
 * completely alone, since the 100%-vs-partial distinction this exists for
 * doesn't apply to them.
 */
export function getPodDisplayStatus(team: SurveyCompletionTeam): SurveyStatus {
  if (team.status !== 'in_progress' && team.status !== 'complete') return team.status;
  return getPodCompletionPercent(team) >= 100 ? 'complete' : 'in_progress';
}

/**
 * A pod is fully completed when its own normalized completion percentage
 * is exactly 100%. Reuses getPodCompletionPercent -- the SAME rounding and
 * missing/invalid-data handling already used for a pod's own displayed
 * status -- so a pod's Fully-Completed classification and what its Status
 * column actually shows can never disagree.
 */
export function isPodFullyCompleted(team: SurveyCompletionTeam): boolean {
  return getPodCompletionPercent(team) >= 100;
}

/**
 * Whether a pod row should show the pale "at risk" red background --
 * Not Started or partial/In Progress completion (1-99%), the same two
 * states the Status column already renders in red (a red badge for Not
 * Started, red percentage text for partial). Never Completed or Opted
 * Out. Driven by the EXACT SAME effective display status a pod's Status
 * column already computes (getPodDisplayStatus) -- never a second,
 * independently-derived condition -- so the row background and the
 * Status column can never disagree about which pods are "at risk".
 */
export function isPaleRedPodRow(displayStatus: SurveyStatus): boolean {
  return displayStatus === 'not_started' || displayStatus === 'in_progress';
}

/**
 * The single shared rule for whether a pod row's Status-column badge is
 * the ONLY place color appears — no row background (status-driven or
 * hover) at all — under the given filter. True for "Total Teams"/"All
 * Teams" and "In Progress"; every other filter keeps the existing row
 * background/hover behavior (isPaleRedPodRow, plus the ordinary hover
 * tint) unchanged.
 */
export function isRowBackgroundHiddenFor(filter: CardFilter): boolean {
  return filter === 'all' || filter === 'in_progress';
}

/**
 * The two-tier color for a pod's own partial/in_progress percentage,
 * used only where isRowBackgroundHiddenFor is true (the percentage moves
 * from plain text into this badge's background there): under 50%, red --
 * the same red family Not Started already uses, so "0%" and a genuinely
 * low partial percentage read as the same severity; 50% and up, yellow.
 * 100% never reaches this — getPodDisplayStatus already reports "complete"
 * at that point, which keeps its own established green badge.
 */
export function getPartialCompletionBadgeClass(percent: number): string {
  return percent >= 50 ? 'bg-yellow-50 text-yellow-700' : 'bg-red-50 text-red-700';
}

/**
 * The single shared hierarchy-completion helper: recursively determines
 * whether EVERY relevant descendant pod in a person's whole reporting
 * subtree -- their own direct pods, plus every subordinate's pods,
 * recursively through however many reporting levels the organization's
 * actual hierarchy has -- is fully completed (isPodFullyCompleted). Used
 * for both the Fully Completed filter's results AND (indirectly, since a
 * qualifying subtree's pods are by definition all 100%) what gets
 * displayed, so the two can never disagree -- see requirement 5.
 *
 * A person with no descendant pods anywhere in their subtree is NOT
 * considered fully completed (there is nothing to judge them by); this is
 * the "no relevant descendant pods" default this app currently has no
 * business rule overriding.
 *
 * `visited` guards against a cycle in the reporting hierarchy. The
 * backend's own reports_to resolution should never produce one, but this
 * stays correct (returns false, never infinite-loops) even if it somehow
 * did.
 */
export function isPersonSubtreeFullyCompleted(
  group: SurveyCompletionPersonGroup,
  visited: Set<string> = new Set(),
): boolean {
  if (visited.has(group.person.id)) return false;
  visited.add(group.person.id);

  const directTeams = group.directTeams ?? [];
  const children = group.children ?? [];
  if (directTeams.length === 0 && children.length === 0) return false;

  return (
    directTeams.every(isPodFullyCompleted) &&
    children.every((child) => isPersonSubtreeFullyCompleted(child, visited))
  );
}

/**
 * Recursively collects every leaf pod in a person's whole reporting
 * subtree -- their own direct pods, plus every subordinate's, at any
 * depth -- each exactly once. This is the single flat, deduplicated leaf
 * set every hierarchy-wide aggregation (percent, status, or anything
 * else) must be built from: an aggregation must never take a subordinate's
 * own already-computed percentage/status as an input (that would
 * double-count or unevenly re-weight a subtree), only ever these raw leaf
 * pods.
 *
 * `visited` guards against a cycle in the reporting hierarchy, exactly as
 * isPersonSubtreeFullyCompleted does.
 */
export function collectDescendantPods(
  group: SurveyCompletionPersonGroup,
  visited: Set<string> = new Set(),
): SurveyCompletionTeam[] {
  if (visited.has(group.person.id)) return [];
  visited.add(group.person.id);

  const pods = [...(group.directTeams ?? [])];
  for (const child of group.children ?? []) {
    pods.push(...collectDescendantPods(child, visited));
  }
  return pods;
}

/**
 * The hierarchy-wide Status % for one leader (Manager/Director/Level-2, or
 * any other team/member node that has descendant pods): combines EVERY leaf
 * pod in their whole reporting subtree (collectDescendantPods) into one
 * synthetic, pod-shaped completion result, each leaf pod counted exactly
 * once regardless of how many reporting levels separate the leader from
 * that pod.
 *
 * AGGREGATION RULE: average every opted-in leaf pod's OWN completion
 * percentage (getPodCompletionPercent -- the exact same value already shown
 * on that pod's own row) -- sum those percentages and divide by the number
 * of pods. This is a plain, unweighted mean of pod-level percentages, taken
 * directly from the full leaf-pod set every time, at any depth -- NEVER by
 * re-averaging a subordinate leader's own already-computed percentage. A
 * leader with two children whose own aggregates read 70% (from pods at
 * 60%/80%) and 60% (from pods at 40%/60%/80%) must NOT read (70+60)/2=65%;
 * it must read (60+80+40+60+80)/5=64% -- the same five leaf pods their
 * subordinates' own aggregates were built from, just combined once more at
 * this level. Re-deriving the leaf-pod set at every level (rather than
 * rolling up a child's finished percentage) is what makes this correct
 * however many hierarchy levels separate a leader from their pods.
 *
 * Opted-out pods are excluded entirely from the average -- exactly like
 * every other completion statistic in this app (the backend's own
 * aggregateTeams, optedInTeams, etc.) -- so an opted-out pod can never pull
 * a hierarchy's percentage toward 0. Returns null when there is no opted-in
 * pod anywhere in the subtree (nothing meaningful to compute -- handles a
 * leader with no descendant pods safely, with no division by zero), so the
 * caller can fall back to the existing display.
 *
 * The result reuses getPodCompletionPercent/getPodDisplayStatus -- the SAME
 * helpers a pod's own Status column already uses -- via a synthetic
 * SurveyCompletionTeam carrying the averaged percentage as completed-out-of
 * -100, so a hierarchy row's displayed percentage and displayed status can
 * never disagree with how an individual pod's own percentage/status would
 * be computed from the same underlying number.
 */
export function computeHierarchyAggregate(
  group: SurveyCompletionPersonGroup,
): { percent: number; displayStatus: SurveyStatus } | null {
  const pods = collectDescendantPods(group).filter((pod) => pod.status !== 'opted_out');
  return aggregatePodPercentages(group.person.id, group.person.name, pods);
}

/**
 * Shared aggregation core for computeHierarchyAggregate and
 * computeVisibleGroupAggregate -- both need the exact same "average of leaf
 * pod percentages" formula, differing only in WHICH pods they collect it
 * from (every descendant pod vs. only the ones currently visible under a
 * filter). Kept in one place so the two can never quietly drift into
 * disagreeing formulas.
 */
function aggregatePodPercentages(
  id: string,
  name: string,
  pods: SurveyCompletionTeam[],
): { percent: number; displayStatus: SurveyStatus } | null {
  if (pods.length === 0) return null;

  const percentSum = pods.reduce((sum, pod) => sum + getPodCompletionPercent(pod), 0);
  const percent = Math.round(percentSum / pods.length);

  // Mirrors the backend's own deriveSurveyCompletionStatus ordering for a
  // canonical status, applied to the AVERAGED percentage rather than one
  // pod's own counts: no completion anywhere yet -> not_started; every pod
  // averaging out to 100% -> complete (via getPodDisplayStatus below);
  // otherwise in_progress.
  const canonicalStatus: SurveyStatus = percent === 0 ? 'not_started' : 'in_progress';

  const syntheticPod: SurveyCompletionTeam = {
    teamId: `hierarchy:${id}`,
    teamName: name,
    completed: percent,
    total: 100,
    postWorkshopCompleted: null,
    status: canonicalStatus,
  };

  return {
    percent: getPodCompletionPercent(syntheticPod),
    displayStatus: getPodDisplayStatus(syntheticPod),
  };
}

/**
 * Indexes every person node (at any depth) in the recursive hierarchy by
 * computeHierarchyAggregate, computed once from the RAW (unfiltered) tree
 * so a filtered/pruned VisibleGroup can still look up its own true
 * hierarchy-wide aggregate by person id at render time -- see
 * SurveyCompletionDashboard's Status-column wiring.
 */
export function buildHierarchyAggregateIndex(
  groups: SurveyCompletionGroup[],
): Map<string, { percent: number; displayStatus: SurveyStatus } | null> {
  const index = new Map<string, { percent: number; displayStatus: SurveyStatus } | null>();
  const walk = (group: SurveyCompletionPersonGroup) => {
    index.set(group.person.id, computeHierarchyAggregate(group));
    (group.children ?? []).forEach(walk);
  };
  for (const group of groups) {
    if (group.type === 'person') walk(group);
  }
  return index;
}

/**
 * A leader (Manager/Director/Level-2) row is deliberately NEVER labeled
 * with a pod-level status like "Not started" or "Opted out", no matter how
 * uniform its descendant pods are -- that label is reserved for the actual
 * pod rows (see TeamRow). A leader stays visible purely as HIERARCHY
 * CONTEXT for its matching pods (buildVisibleGroups, below). There is
 * therefore no separate "hierarchy status" aggregation for leader rows --
 * matchesCardFilter below is the only place a pod's canonical status is
 * ever compared.
 *
 * The one exception is the Total Teams / All Teams filter's own
 * hierarchical percentage/status (computeHierarchyAggregate, above) --
 * that IS a per-leader aggregate label, but it is scoped to that one
 * filter only (see SurveyCompletionDashboard) and is never a bare
 * "Not started"/"Opted out" word, always a percent-based (or, at 100%,
 * green "Completed") display exactly like a pod's own.
 */

/**
 * The single shared rule for whether a leader (Manager/Director/Level-2)
 * row's own Status column shows ANYTHING at all under the given filter.
 *
 * Under the Not Started or Opted Out filters, a leader row is pure
 * hierarchy context for its matching pods -- it shows no percent badge, no
 * derived status, nothing -- because a percent/aggregate badge on the
 * parent would read as the parent's OWN status, which is never what either
 * filter means for a row that is only there to give a pod somewhere to
 * nest. Every other filter (Fully Completed, In Progress, all, optedIn)
 * keeps showing the leader's existing percent-based (or, for Fully
 * Completed, green "Completed") badge exactly as before -- this is the
 * ONE place that decision is made, so the rule can never drift out of
 * sync between filters or duplicate itself across components.
 */
export function isParentStatusHiddenFor(filter: CardFilter): boolean {
  return filter === 'not_started' || filter === 'opted_out';
}

export function matchesCardFilter(team: SurveyCompletionTeam, filter: CardFilter): boolean {
  switch (filter) {
    case 'optedIn':
      return team.status !== 'opted_out';
    case 'complete':
    case 'in_progress':
    case 'not_started':
    case 'opted_out':
      return team.status === filter;
    default:
      return true;
  }
}

export type VisibleGroup =
  | {
      type: 'person';
      key: string;
      id: string;
      name: string;
      level: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      visibleDirectTeams: SurveyCompletionTeam[];
      visibleChildren: VisibleGroup[];
    }
  | {
      type: 'other';
      key: string;
      id: string;
      name: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      visibleTeams: SurveyCompletionTeam[];
    };

function teamMatches(team: SurveyCompletionTeam, query: string): boolean {
  return team.teamName.toLowerCase().includes(query);
}

function visibleTeamsFor(
  teams: SurveyCompletionTeam[],
  filter: CardFilter,
  query: string,
  ancestorMatches: boolean,
): SurveyCompletionTeam[] {
  return teams.filter(
    (team) => matchesCardFilter(team, filter) && (!query || ancestorMatches || teamMatches(team, query)),
  );
}

/**
 * totalTeams/optedInTeams/remindCount for one flat list of CURRENTLY VISIBLE
 * teams — the leaf-level half of the fix for a group header showing stale
 * counts from its raw, unfiltered data. A hidden (filtered/searched-out)
 * team must never contribute to any of these three numbers.
 */
function countVisibleTeamStats(teams: SurveyCompletionTeam[]): { total: number; optedIn: number; remind: number } {
  let optedIn = 0;
  let remind = 0;
  for (const t of teams) {
    if (t.status !== 'opted_out') optedIn++;
    if (isRemindable(t)) remind++;
  }
  return { total: teams.length, optedIn, remind };
}

/**
 * Rolls a leader's own visible-team stats together with its already-recomputed
 * visible children's stats (each child VisibleGroup's totalTeams/optedInTeams/
 * remindCount are themselves visible-only, by construction, since every
 * VisibleGroup is built through this same recomputation) — the recursive half
 * of the fix: a leader's counts always sum from the CURRENTLY VISIBLE subtree,
 * never a subordinate's raw/unfiltered numbers.
 */
function combineVisibleStats(
  direct: SurveyCompletionTeam[],
  children: VisibleGroup[],
): { totalTeams: number; optedInTeams: number; remindCount: number } {
  const directStats = countVisibleTeamStats(direct);
  return children.reduce(
    (acc, child) => ({
      totalTeams: acc.totalTeams + child.totalTeams,
      optedInTeams: acc.optedInTeams + child.optedInTeams,
      remindCount: acc.remindCount + child.remindCount,
    }),
    { totalTeams: directStats.total, optedInTeams: directStats.optedIn, remindCount: directStats.remind },
  );
}

/**
 * Recursively filters one person node (and its whole subtree). A node
 * whose own name matches the search query — or that inherited a match from
 * an ancestor — shows all of its (filter-matching) teams and children in
 * full, exactly like the un-searched view; a node with no match of its own
 * still appears (to keep the path to a matching descendant visible) but
 * only with the children/teams that themselves matched or contain a match.
 * Returns null when nothing in this subtree should be visible at all, so
 * the caller drops it entirely rather than rendering an empty branch.
 */
function toVisiblePersonGroup(
  group: SurveyCompletionPersonGroup,
  filter: CardFilter,
  query: string,
  ancestorMatches: boolean,
): VisibleGroup | null {
  const selfMatches = ancestorMatches || (!!query && group.person.name.toLowerCase().includes(query));

  const visibleDirectTeams = visibleTeamsFor(group.directTeams ?? [], filter, query, selfMatches);
  const visibleChildren = (group.children ?? [])
    .map((child) => toVisiblePersonGroup(child, filter, query, selfMatches))
    .filter((child): child is VisibleGroup => child !== null);

  if (visibleDirectTeams.length === 0 && visibleChildren.length === 0) return null;

  const stats = combineVisibleStats(visibleDirectTeams, visibleChildren);

  return {
    type: 'person',
    key: `person:${group.person.id}`,
    id: group.person.id,
    name: group.person.name,
    level: group.person.level,
    totalTeams: stats.totalTeams,
    optedInTeams: stats.optedInTeams,
    completionPercent: group.completionPercent,
    remindCount: stats.remindCount,
    visibleDirectTeams,
    visibleChildren,
  };
}

/**
 * Filters the recursive group hierarchy by the active metric-card filter
 * and search query, dropping any node (at any depth) left with no visible
 * teams anywhere in its subtree. A search match anywhere in a root's
 * subtree — its own name, any descendant leader's name, "Other", or a pod
 * name — automatically keeps that whole path visible (see
 * toVisiblePersonGroup), which is what lets the dashboard auto-expand
 * straight to a match regardless of how many levels deep it is.
 */
export function buildVisibleGroups(
  groups: SurveyCompletionGroup[],
  searchQuery: string,
  filter: CardFilter,
): VisibleGroup[] {
  const query = searchQuery.trim().toLowerCase();

  const visible: VisibleGroup[] = [];

  for (const group of groups) {
    if (group.type === 'person') {
      const visibleGroup = toVisiblePersonGroup(group, filter, query, false);
      if (visibleGroup) visible.push(visibleGroup);
    } else {
      const labelMatches = !!query && group.label.toLowerCase().includes(query);
      const visibleTeams = visibleTeamsFor(group.teams ?? [], filter, query, labelMatches);
      if (visibleTeams.length === 0) continue;

      const stats = countVisibleTeamStats(visibleTeams);

      visible.push({
        type: 'other',
        key: 'other',
        id: 'other',
        name: group.label,
        totalTeams: stats.total,
        optedInTeams: stats.optedIn,
        completionPercent: group.completionPercent,
        remindCount: stats.remind,
        visibleTeams,
      });
    }
  }

  return visible;
}

/**
 * Recursively collects every pod actually rendered in this ALREADY-FILTERED
 * VisibleGroup subtree -- its own visible direct pods, plus every visible
 * child's, at any depth. Unlike collectDescendantPods (which reads from the
 * raw, unfiltered SurveyCompletionPersonGroup tree), this only ever sees the
 * teams a filter/search has left standing, since VisibleGroup itself is the
 * pruned output of buildVisibleGroups.
 */
export function collectVisiblePods(group: VisibleGroup): SurveyCompletionTeam[] {
  if (group.type === 'other') return group.visibleTeams;
  return [...group.visibleDirectTeams, ...group.visibleChildren.flatMap(collectVisiblePods)];
}

/**
 * The In Progress filter's own per-leader aggregate: the exact same
 * average-of-pod-percentages rule as computeHierarchyAggregate
 * (aggregatePodPercentages), but scoped to only the pods CURRENTLY LISTED
 * under this leader in the filtered view (collectVisiblePods) rather than
 * every descendant pod in their raw, unfiltered subtree. This is what lets a
 * leader's percentage answer "of the teams shown here, what's the average
 * completion" instead of silently pulling in complete/not_started/opted_out
 * pods the filter has already hidden -- e.g. a manager with Danville (5/9 =
 * 56%) and Sausalito (5/9 = 56%) shown under In Progress reads 56%, not a
 * lower number diluted by other, hidden pods.
 *
 * Opted-out pods are excluded as a defensive measure only -- matchesCardFilter
 * never lets one match the in_progress filter in the first place, so in
 * practice every pod collectVisiblePods returns here already has status
 * 'in_progress'. Returns null when the leader has no visible pod anywhere in
 * their subtree (nothing to compute), matching computeHierarchyAggregate's
 * own null case.
 */
export function computeVisibleGroupAggregate(
  group: Extract<VisibleGroup, { type: 'person' }>,
): { percent: number; displayStatus: SurveyStatus } | null {
  const pods = collectVisiblePods(group).filter((pod) => pod.status !== 'opted_out');
  return aggregatePodPercentages(group.id, group.name, pods);
}

/**
 * Indexes every person node (at any depth) in an ALREADY-FILTERED VisibleGroup
 * tree by computeVisibleGroupAggregate -- the In Progress filter's counterpart
 * to buildHierarchyAggregateIndex, which instead indexes the raw tree for the
 * Total Teams / All Teams filter. Built fresh per render from that render's
 * own visibleGroups, since which pods are "currently listed" changes with the
 * active filter and search query.
 */
export function buildVisibleHierarchyAggregateIndex(
  groups: VisibleGroup[],
): Map<string, { percent: number; displayStatus: SurveyStatus } | null> {
  const index = new Map<string, { percent: number; displayStatus: SurveyStatus } | null>();
  const walk = (group: VisibleGroup) => {
    if (group.type !== 'person') return;
    index.set(group.id, computeVisibleGroupAggregate(group));
    group.visibleChildren.forEach(walk);
  };
  groups.forEach(walk);
  return index;
}

/**
 * True if this exact subtree (its own name, any direct pod's name, or any
 * descendant person's name / pod name at any depth) contains the search
 * query anywhere. Used only by buildFullyCompletedGroups: once a subtree
 * is confirmed fully completed, it is shown WHOLE (never pruned pod-by-pod
 * the way buildVisibleGroups does for other filters, since every pod in a
 * qualifying subtree is already 100% by definition) -- so search here only
 * ever decides whether to show the whole thing, never which parts of it.
 */
function fullyCompletedSubtreeMatchesSearch(group: SurveyCompletionPersonGroup, query: string): boolean {
  if (group.person.name.toLowerCase().includes(query)) return true;
  if ((group.directTeams ?? []).some((t) => t.teamName.toLowerCase().includes(query))) return true;
  return (group.children ?? []).some((child) => fullyCompletedSubtreeMatchesSearch(child, query));
}

/** Converts a KNOWN-fully-completed subtree into a VisibleGroup, in full -- no pruning. */
function wholeSubtreeToVisibleGroup(group: SurveyCompletionPersonGroup): VisibleGroup {
  return {
    type: 'person',
    key: `person:${group.person.id}`,
    id: group.person.id,
    name: group.person.name,
    level: group.person.level,
    totalTeams: group.totalTeams,
    optedInTeams: group.optedInTeams,
    completionPercent: group.completionPercent,
    remindCount: group.remindCount,
    visibleDirectTeams: group.directTeams ?? [],
    visibleChildren: (group.children ?? []).map(wholeSubtreeToVisibleGroup),
  };
}

/**
 * Walks down from `group`, emitting one VisibleGroup entry per subtree
 * that independently qualifies as fully completed (isPersonSubtreeFullyCompleted).
 * A node that does NOT qualify is never emitted itself -- per requirement
 * 3, a parent with even one incomplete descendant must not appear -- but
 * its children are still checked independently and promoted to top-level
 * entries in `out` if THEY qualify on their own, even though their real
 * parent didn't. This is what lets e.g. a fully-completed manager surface
 * even when their director's own subtree has some other, unrelated,
 * incomplete manager underneath it.
 */
function collectFullyCompletedGroups(
  group: SurveyCompletionPersonGroup,
  query: string,
  out: VisibleGroup[],
): void {
  if (isPersonSubtreeFullyCompleted(group)) {
    if (query && !fullyCompletedSubtreeMatchesSearch(group, query)) return;
    out.push(wholeSubtreeToVisibleGroup(group));
    return;
  }
  for (const child of group.children ?? []) {
    collectFullyCompletedGroups(child, query, out);
  }
}

/**
 * The Fully Completed filter's own builder -- structurally separate from
 * buildVisibleGroups (which every other filter still uses, unchanged)
 * because "fully completed" is a hierarchy-wide AND across a person's
 * entire subtree, not "does at least one matching pod exist somewhere
 * below this person" (what buildVisibleGroups' pod-filter answers for
 * every other status). Both builders share the same underlying
 * isPodFullyCompleted/isPersonSubtreeFullyCompleted helpers, so a person
 * or pod shown here is never inconsistent with what its own Status column
 * displays.
 */
export function buildFullyCompletedGroups(
  groups: SurveyCompletionGroup[],
  searchQuery: string,
): VisibleGroup[] {
  const query = searchQuery.trim().toLowerCase();
  const visible: VisibleGroup[] = [];

  for (const group of groups) {
    if (group.type === 'person') {
      collectFullyCompletedGroups(group, query, visible);
    } else {
      // "Other" pods have no reporting parent to roll up through -- each
      // one is judged purely on its own completion.
      const labelMatches = !!query && group.label.toLowerCase().includes(query);
      const visibleTeams = (group.teams ?? []).filter(
        (t) => isPodFullyCompleted(t) && (!query || labelMatches || t.teamName.toLowerCase().includes(query)),
      );
      if (visibleTeams.length === 0) continue;

      visible.push({
        type: 'other',
        key: 'other',
        id: 'other',
        name: group.label,
        totalTeams: group.totalTeams,
        optedInTeams: group.optedInTeams,
        completionPercent: group.completionPercent,
        remindCount: group.remindCount,
        visibleTeams,
      });
    }
  }

  return visible;
}

/**
 * Total number of visible pods across every group/subtree — used for the
 * "N teams" count above the table and to decide whether the search box
 * turned up zero results.
 */
export function countVisibleTeams(groups: VisibleGroup[]): number {
  let count = 0;
  for (const group of groups) {
    if (group.type === 'person') {
      count += group.visibleDirectTeams.length + countVisibleTeams(group.visibleChildren);
    } else {
      count += group.visibleTeams.length;
    }
  }
  return count;
}
