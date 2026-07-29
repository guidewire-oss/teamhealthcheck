// SPDX-License-Identifier: Apache-2.0

import { Team, HealthCheckSession } from './types';

// Mock teams with proper supervisor chain
export const TEAMS_DATA: Team[] = [
  // Teams under Manager John Smith (mgr1) -> Director Mike Chen (dir1) -> VP Sarah Johnson (vp1)
  {
    id: 'team1',
    name: 'Phoenix Squad',
    cadence: 'quarterly',
    nextCheckDate: '2024-03-31',
    members: ['lead1', 'mem1', 'mem2', 'mem3', 'mem4', 'mem5'],
    supervisorChain: [
      { userId: 'lead1', levelId: 'level-4' }, // Team Lead
      { userId: 'mgr1', levelId: 'level-3' },  // Manager
      { userId: 'dir1', levelId: 'level-2' },  // Director
      { userId: 'vp1', levelId: 'level-1' }    // VP
    ],
    department: 'Engineering',
    division: 'Product Development'
  },
  {
    id: 'team2',
    name: 'Dragon Squad',
    cadence: 'monthly',
    nextCheckDate: '2024-02-01',
    members: ['lead2', 'mem6', 'mem7', 'mem8', 'mem9', 'mem10'],
    supervisorChain: [
      { userId: 'lead2', levelId: 'level-4' },
      { userId: 'mgr1', levelId: 'level-3' },
      { userId: 'dir1', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Engineering',
    division: 'Product Development'
  },
  // Team 3 under Manager John Smith (mgr1) -> Director Mike Chen (dir1) -> VP Sarah Johnson (vp1)
  {
    id: 'team3',
    name: 'Titan Squad',
    cadence: 'quarterly',
    nextCheckDate: '2024-03-31',
    members: ['lead3', 'mem11', 'mem12', 'mem13', 'mem14', 'mem15'],
    supervisorChain: [
      { userId: 'lead3', levelId: 'level-4' },
      { userId: 'mgr1', levelId: 'level-3' },
      { userId: 'dir1', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Engineering',
    division: 'Product Development'
  },

  // Teams under Manager Emma Wilson (mgr2) -> Director Mike Chen (dir1) -> VP Sarah Johnson (vp1)
  {
    id: 'team4',
    name: 'Falcon Squad',
    cadence: 'monthly',
    nextCheckDate: '2024-01-15',
    members: ['lead4', 'mem16', 'mem17', 'mem18', 'mem19', 'mem20'],
    supervisorChain: [
      { userId: 'lead4', levelId: 'level-4' },
      { userId: 'mgr2', levelId: 'level-3' },
      { userId: 'dir1', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Engineering',
    division: 'Product Development'
  },
  {
    id: 'team5',
    name: 'Eagle Squad',
    cadence: 'quarterly',
    nextCheckDate: '2024-03-31',
    members: ['lead5', 'mem21', 'mem22', 'mem23', 'mem24', 'mem25'],
    supervisorChain: [
      { userId: 'lead5', levelId: 'level-4' },
      { userId: 'mgr2', levelId: 'level-3' },
      { userId: 'dir1', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Engineering',
    division: 'Product Development'
  },
  {
    id: 'team6',
    name: 'Hawk Squad',
    cadence: 'monthly',
    nextCheckDate: '2024-02-15',
    members: ['lead6', 'mem26', 'mem27', 'mem28', 'mem29', 'mem30'],
    supervisorChain: [
      { userId: 'lead6', levelId: 'level-4' },
      { userId: 'mgr2', levelId: 'level-3' },
      { userId: 'dir1', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Engineering',
    division: 'Product Development'
  },

  // Teams under Manager David Brown (mgr3) -> Director Lisa Anderson (dir2) -> VP Sarah Johnson (vp1)
  {
    id: 'team7',
    name: 'Raven Squad',
    cadence: 'quarterly',
    nextCheckDate: '2024-03-20',
    members: ['lead7', 'mem31', 'mem32', 'mem33', 'mem34', 'mem35'],
    supervisorChain: [
      { userId: 'lead7', levelId: 'level-4' },
      { userId: 'mgr3', levelId: 'level-3' },
      { userId: 'dir2', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Quality Assurance',
    division: 'Product Development'
  },
  {
    id: 'team8',
    name: 'Wolf Squad',
    cadence: 'monthly',
    nextCheckDate: '2024-01-25',
    members: ['lead8', 'mem36', 'mem37', 'mem38', 'mem39', 'mem40'],
    supervisorChain: [
      { userId: 'lead8', levelId: 'level-4' },
      { userId: 'mgr3', levelId: 'level-3' },
      { userId: 'dir2', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Quality Assurance',
    division: 'Product Development'
  },
  {
    id: 'team9',
    name: 'Panther Squad',
    cadence: 'monthly',
    nextCheckDate: '2024-02-10',
    members: ['lead9', 'mem41', 'mem42', 'mem43', 'mem44', 'mem45'],
    supervisorChain: [
      { userId: 'lead9', levelId: 'level-4' },
      { userId: 'mgr3', levelId: 'level-3' },
      { userId: 'dir2', levelId: 'level-2' },
      { userId: 'vp1', levelId: 'level-1' }
    ],
    department: 'Quality Assurance',
    division: 'Product Development'
  }
];

// Generate mock health check sessions with varied scores
export function generateMockHealthSessions(): HealthCheckSession[] {
  const sessions: HealthCheckSession[] = [];
  const dimensions = ['mission', 'value', 'speed', 'fun', 'health', 'learning', 'support', 'pawns', 'release', 'process', 'teamwork'];
  
  TEAMS_DATA.forEach(team => {
    // Generate 3 months of history for each team
    for (let monthsAgo = 0; monthsAgo < 3; monthsAgo++) {
      const date = new Date();
      date.setMonth(date.getMonth() - monthsAgo);
      
      // Generate sessions for some team members
      const memberCount = Math.floor(team.members.length * (0.7 + Math.random() * 0.3)); // 70-100% participation

      // Determine assessment period based on date
      const year = date.getFullYear();
      const month = date.getMonth();
      const assessmentPeriod = month < 6 ? `${year} - 1st Half` : `${year} - 2nd Half`;

      for (let i = 0; i < memberCount; i++) {
        const session: HealthCheckSession = {
          id: `session-${team.id}-${monthsAgo}-${i}`,
          teamId: team.id,
          userId: team.members[i] || 'mem1',
          date: date.toISOString().split('T')[0],
          assessmentPeriod,
          completed: true,
          responses: dimensions.map(dim => {
            // Generate realistic scores with some patterns
            let baseScore = 2; // Start with yellow
            
            // Teams under dir1 tend to score higher
            if (team.supervisorChain.some(s => s.userId === 'dir1')) {
              baseScore += 0.3;
            }
            
            // QA teams (dir2) have different patterns
            if (team.department === 'Quality Assurance') {
              if (dim === 'health' || dim === 'speed') {
                baseScore += 0.5; // QA teams excel at code health
              }
              if (dim === 'process') {
                baseScore += 0.4; // QA teams have good processes
              }
            }

            // Engineering teams have better release processes
            if (team.department === 'Engineering') {
              if (dim === 'release') {
                baseScore += 0.4; // Engineering teams have better CI/CD
              }
              if (dim === 'teamwork') {
                baseScore += 0.3; // Strong collaboration
              }
            }
            
            // Add some randomness
            baseScore += (Math.random() - 0.5);
            
            // Convert to 1-3 scale
            const score = Math.max(1, Math.min(3, Math.round(baseScore))) as 1 | 2 | 3;
            
            // Determine trend based on month comparison
            let trend: 'improving' | 'stable' | 'declining' = 'stable';
            if (monthsAgo === 0) {
              const random = Math.random();
              if (random < 0.3) trend = 'improving';
              else if (random > 0.7) trend = 'declining';
            }
            
            return {
              dimensionId: dim,
              score,
              trend,
              comment: Math.random() > 0.8 ? `Comment about ${dim}` : undefined
            };
          })
        };
        sessions.push(session);
      }
    }
  });
  
  return sessions;
}

// Calculate aggregated metrics for a set of teams
export function calculateAggregatedMetrics(teams: Team[], sessions: HealthCheckSession[]) {
  const metrics = {
    totalTeams: teams.length,
    totalMembers: teams.reduce((sum, team) => sum + team.members.length, 0),
    avgHealth: 0,
    participation: 0,
    trends: {
      improving: 0,
      stable: 0,
      declining: 0
    },
    dimensionScores: new Map<string, { score: number; count: number }>()
  };
  
  // Filter sessions for these teams
  const teamIds = teams.map(t => t.id);
  const relevantSessions = sessions.filter(s => teamIds.includes(s.teamId) && s.completed);
  
  // Get latest sessions only
  const latestSessions = relevantSessions.filter(s => {
    const sessionDate = new Date(s.date);
    const oneMonthAgo = new Date();
    oneMonthAgo.setMonth(oneMonthAgo.getMonth() - 1);
    return sessionDate > oneMonthAgo;
  });
  
  if (latestSessions.length === 0) {
    return metrics;
  }
  
  // Calculate metrics
  let totalScore = 0;
  let scoreCount = 0;
  
  latestSessions.forEach(session => {
    session.responses.forEach(response => {
      totalScore += response.score;
      scoreCount++;
      
      // Update trends
      if (response.trend === 'improving') metrics.trends.improving++;
      else if (response.trend === 'stable') metrics.trends.stable++;
      else if (response.trend === 'declining') metrics.trends.declining++;
      
      // Update dimension scores
      const current = metrics.dimensionScores.get(response.dimensionId) || { score: 0, count: 0 };
      current.score += response.score;
      current.count++;
      metrics.dimensionScores.set(response.dimensionId, current);
    });
  });
  
  metrics.avgHealth = scoreCount > 0 ? totalScore / scoreCount : 0;
  metrics.participation = latestSessions.length / metrics.totalMembers;
  
  return metrics;
}