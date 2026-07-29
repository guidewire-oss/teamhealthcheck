// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

'use client';

import { useState, useEffect } from 'react';
import { User, Team, HierarchicalSummary, OrganizationNode } from '@/lib/types';
import { getOrgConfig, getUserPermissions, getSubordinates } from '@/lib/org-config';
import { ChevronRight, ChevronDown, Users, TrendingUp, TrendingDown, Minus, Activity, Eye, Filter, Download } from 'lucide-react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, PieChart, Pie, Cell, RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar } from 'recharts';

interface Props {
  currentUser: User;
  users: User[];
  teams: Team[];
}

export default function HierarchicalDashboard({ currentUser, users, teams }: Props) {
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'tree' | 'summary' | 'comparison'>('tree');
  const [filterLevel, setFilterLevel] = useState<string>('all');
  const config = getOrgConfig();
  const permissions = getUserPermissions(currentUser);

  // Build organization tree starting from current user or top level
  const buildOrgTree = (userId: string): OrganizationNode | null => {
    const user = users.find(u => u.id === userId);
    if (!user) return null;

    const level = config.hierarchyLevels.find(l => l.id === user.hierarchyLevelId);
    if (!level) return null;

    const node: OrganizationNode = {
      id: user.id,
      user,
      level,
      children: [],
      teams: [],
      metrics: undefined
    };

    // Get direct reports
    const directReports = users.filter(u => u.reportsTo === userId);
    for (const report of directReports) {
      const childNode = buildOrgTree(report.id);
      if (childNode) {
        node.children.push(childNode);
      }
    }

    // Get teams under this user
    const userTeams = teams.filter(team => 
      team.supervisorChain.some(s => s.userId === userId)
    );
    node.teams = userTeams;

    // Calculate metrics
    node.metrics = calculateMetrics(node);

    return node;
  };

  // Calculate roll-up metrics
  const calculateMetrics = (node: OrganizationNode) => {
    let totalTeams = node.teams.length;
    let totalMembers = 0;
    let totalHealth = 0;
    let healthCount = 0;
    let improving = 0;
    let stable = 0;
    let declining = 0;

    // Count from direct teams
    node.teams.forEach(team => {
      totalMembers += team.members.length;
      // Mock health data (in production, this would come from actual health check sessions)
      const mockHealth = 2.5 + Math.random();
      totalHealth += mockHealth;
      healthCount++;
      
      // Mock trend data
      const trend = Math.random();
      if (trend < 0.33) improving++;
      else if (trend < 0.66) stable++;
      else declining++;
    });

    // Roll up from children
    node.children.forEach(child => {
      if (child.metrics) {
        totalTeams += child.metrics.totalTeams;
        totalMembers += child.metrics.totalMembers;
        
        if (child.metrics.avgHealth > 0) {
          totalHealth += child.metrics.avgHealth * child.metrics.totalTeams;
          healthCount += child.metrics.totalTeams;
        }
        
        improving += child.metrics.trends.improving;
        stable += child.metrics.trends.stable;
        declining += child.metrics.trends.declining;
      }
    });

    const dimensionScores = new Map<string, number>();
    // Mock dimension scores
    ['mission', 'value', 'speed', 'fun', 'health', 'learning', 'support', 'pawns'].forEach(dim => {
      dimensionScores.set(dim, 2 + Math.random());
    });

    return {
      avgHealth: healthCount > 0 ? totalHealth / healthCount : 0,
      totalTeams,
      totalMembers,
      completionRate: 0.75 + Math.random() * 0.2,
      trends: { improving, stable, declining },
      dimensionScores
    };
  };

  const toggleNode = (nodeId: string) => {
    const newExpanded = new Set(expandedNodes);
    if (newExpanded.has(nodeId)) {
      newExpanded.delete(nodeId);
    } else {
      newExpanded.add(nodeId);
    }
    setExpandedNodes(newExpanded);
  };

  const getHealthColor = (score: number) => {
    if (score >= 2.5) return 'text-green-600 bg-green-50';
    if (score >= 1.5) return 'text-yellow-600 bg-yellow-50';
    return 'text-red-600 bg-red-50';
  };

  const renderNode = (node: OrganizationNode, depth: number = 0) => {
    const isExpanded = expandedNodes.has(node.id);
    const hasChildren = node.children.length > 0 || node.teams.length > 0;
    const healthScore = node.metrics?.avgHealth || 0;

    return (
      <div key={node.id} className={`${depth > 0 ? 'ml-8' : ''}`}>
        <div
          className={`flex items-center justify-between p-4 mb-2 bg-white rounded-lg border hover:shadow-md transition-shadow cursor-pointer ${
            selectedNode === node.id ? 'ring-2 ring-indigo-500' : ''
          }`}
          onClick={() => setSelectedNode(node.id)}
        >
          <div className="flex items-center gap-3">
            {hasChildren && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  toggleNode(node.id);
                }}
                className="p-1 hover:bg-gray-100 rounded"
              >
                {isExpanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
              </button>
            )}
            
            <div
              className="w-3 h-3 rounded-full"
              style={{ backgroundColor: node.level.color }}
            />
            
            <div>
              <div className="font-semibold text-gray-900">{node.user.name}</div>
              <div className="text-sm text-gray-500">{node.level.name}</div>
            </div>
          </div>

          <div className="flex items-center gap-6">
            <div className="text-center">
              <div className="text-xs text-gray-500">Teams</div>
              <div className="font-semibold">{node.metrics?.totalTeams || 0}</div>
            </div>
            
            <div className="text-center">
              <div className="text-xs text-gray-500">Members</div>
              <div className="font-semibold">{node.metrics?.totalMembers || 0}</div>
            </div>
            
            <div className="text-center">
              <div className="text-xs text-gray-500">Health</div>
              <div className={`font-semibold px-2 py-1 rounded ${getHealthColor(healthScore)}`}>
                {healthScore.toFixed(1)}
              </div>
            </div>
            
            <div className="flex gap-1">
              {node.metrics?.trends.improving && node.metrics.trends.improving > 0 && (
                <div className="flex items-center gap-1 text-green-600">
                  <TrendingUp className="w-4 h-4" />
                  <span className="text-sm">{node.metrics.trends.improving}</span>
                </div>
              )}
              {node.metrics?.trends.stable && node.metrics.trends.stable > 0 && (
                <div className="flex items-center gap-1 text-blue-600">
                  <Minus className="w-4 h-4" />
                  <span className="text-sm">{node.metrics.trends.stable}</span>
                </div>
              )}
              {node.metrics?.trends.declining && node.metrics.trends.declining > 0 && (
                <div className="flex items-center gap-1 text-red-600">
                  <TrendingDown className="w-4 h-4" />
                  <span className="text-sm">{node.metrics.trends.declining}</span>
                </div>
              )}
            </div>
          </div>
        </div>

        {isExpanded && (
          <div>
            {node.children.map(child => renderNode(child, depth + 1))}
            {node.teams.map(team => (
              <div
                key={team.id}
                className={`ml-${8 * (depth + 1)} flex items-center justify-between p-3 mb-1 bg-gray-50 rounded-lg border-l-4 border-gray-300`}
              >
                <div className="flex items-center gap-3">
                  <Users className="w-4 h-4 text-gray-500" />
                  <div>
                    <div className="font-medium text-gray-900">{team.name}</div>
                    <div className="text-sm text-gray-500">{team.members.length} members</div>
                  </div>
                </div>
                <div className="text-sm text-gray-500">{team.cadence}</div>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  const rootNode = buildOrgTree(currentUser.id);

  if (!rootNode) {
    return <div>No organization data available</div>;
  }

  // Prepare data for charts
  const trendData = [
    { name: 'Improving', value: rootNode.metrics?.trends.improving || 0, color: '#10B981' },
    { name: 'Stable', value: rootNode.metrics?.trends.stable || 0, color: '#3B82F6' },
    { name: 'Declining', value: rootNode.metrics?.trends.declining || 0, color: '#EF4444' }
  ];

  const dimensionData = Array.from(rootNode.metrics?.dimensionScores || []).map(([key, value]) => ({
    dimension: key,
    score: value
  }));

  return (
    <div className="space-y-6">
      {/* Header Controls */}
      <div className="flex justify-between items-center">
        <div className="flex gap-2">
          <button
            onClick={() => setViewMode('tree')}
            className={`px-4 py-2 rounded-lg transition-colors ${
              viewMode === 'tree' ? 'bg-indigo-600 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            Hierarchy View
          </button>
          <button
            onClick={() => setViewMode('summary')}
            className={`px-4 py-2 rounded-lg transition-colors ${
              viewMode === 'summary' ? 'bg-indigo-600 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            Summary View
          </button>
          <button
            onClick={() => setViewMode('comparison')}
            className={`px-4 py-2 rounded-lg transition-colors ${
              viewMode === 'comparison' ? 'bg-indigo-600 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            Comparison
          </button>
        </div>

        <div className="flex gap-2">
          <select
            value={filterLevel}
            onChange={(e) => setFilterLevel(e.target.value)}
            className="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500"
          >
            <option value="all">All Levels</option>
            {config.hierarchyLevels.map(level => (
              <option key={level.id} value={level.id}>{level.name}</option>
            ))}
          </select>

          {permissions.canExportData && (
            <button className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700">
              <Download className="w-4 h-4" />
              Export
            </button>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="bg-white p-6 rounded-xl shadow-sm border">
          <div className="flex items-center justify-between mb-2">
            <Users className="w-8 h-8 text-indigo-600" />
            <span className="text-2xl font-bold text-gray-900">{rootNode.metrics?.totalMembers || 0}</span>
          </div>
          <p className="text-gray-600">Total Members</p>
        </div>

        <div className="bg-white p-6 rounded-xl shadow-sm border">
          <div className="flex items-center justify-between mb-2">
            <Activity className="w-8 h-8 text-green-600" />
            <span className="text-2xl font-bold text-gray-900">
              {((rootNode.metrics?.avgHealth || 0) / 3 * 100).toFixed(0)}%
            </span>
          </div>
          <p className="text-gray-600">Overall Health</p>
        </div>

        <div className="bg-white p-6 rounded-xl shadow-sm border">
          <div className="flex items-center justify-between mb-2">
            <Eye className="w-8 h-8 text-blue-600" />
            <span className="text-2xl font-bold text-gray-900">
              {((rootNode.metrics?.completionRate || 0) * 100).toFixed(0)}%
            </span>
          </div>
          <p className="text-gray-600">Participation</p>
        </div>

        <div className="bg-white p-6 rounded-xl shadow-sm border">
          <div className="flex items-center justify-between mb-2">
            <Filter className="w-8 h-8 text-purple-600" />
            <span className="text-2xl font-bold text-gray-900">{rootNode.metrics?.totalTeams || 0}</span>
          </div>
          <p className="text-gray-600">Total Teams</p>
        </div>
      </div>

      {/* Main Content */}
      {viewMode === 'tree' && (
        <div className="bg-white p-6 rounded-xl shadow-sm border">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Organization Hierarchy</h3>
          {renderNode(rootNode)}
        </div>
      )}

      {viewMode === 'summary' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="bg-white p-6 rounded-xl shadow-sm border">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Health Trends</h3>
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={trendData}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, value }) => `${name}: ${value}`}
                  outerRadius={80}
                  fill="#8884d8"
                  dataKey="value"
                >
                  {trendData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </div>

          <div className="bg-white p-6 rounded-xl shadow-sm border">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Dimension Scores</h3>
            <ResponsiveContainer width="100%" height={300}>
              <RadarChart data={dimensionData}>
                <PolarGrid strokeDasharray="3 3" />
                <PolarAngleAxis dataKey="dimension" />
                <PolarRadiusAxis angle={90} domain={[0, 3]} />
                <Radar name="Score" dataKey="score" stroke="#6366F1" fill="#6366F1" fillOpacity={0.6} />
                <Tooltip />
              </RadarChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}
    </div>
  );
}