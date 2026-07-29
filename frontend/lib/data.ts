// SPDX-License-Identifier: Apache-2.0

import { HealthDimension } from './types';

/**
 * Health Check Dimensions based on Spotify's Squad Health Check Model
 * These are configuration constants that define the dimensions used in health checks.
 *
 * Note: All other data (teams, users, health check sessions) comes from the backend API.
 * Only HEALTH_DIMENSIONS remains as frontend configuration.
 */
export const HEALTH_DIMENSIONS: HealthDimension[] = [
  {
    id: 'mission',
    name: 'Mission',
    description: 'We know exactly why we are here, and we are really excited about it',
    goodDescription: 'We know exactly why we are here, and we are really excited about it',
    badDescription: 'We have no idea why we are here. There is no high level picture or focus.'
  },
  {
    id: 'value',
    name: 'Delivering Value',
    description: 'We deliver great stuff! We are proud of it and our stakeholders are really happy',
    goodDescription: 'We deliver great stuff! We are proud of it and our stakeholders are really happy',
    badDescription: 'We deliver crap. We are ashamed to deliver it. Our stakeholders hate us.'
  },
  {
    id: 'speed',
    name: 'Speed',
    description: 'We get stuff done really quickly. No waiting, no delays',
    goodDescription: 'We get stuff done really quickly. No waiting, no delays',
    badDescription: 'We never seem to get anything done. We keep getting stuck or interrupted.'
  },
  {
    id: 'fun',
    name: 'Fun',
    description: 'We love going to work, and have great fun working together',
    goodDescription: 'We love going to work, and have great fun working together',
    badDescription: 'Boooooooring'
  },
  {
    id: 'health',
    name: 'Health of Codebase',
    description: 'Our code is clean, easy to read, and has great test coverage',
    goodDescription: 'Our code is clean, easy to read, and has great test coverage',
    badDescription: 'Our code is a pile of dung, and technical debt is raging out of control'
  },
  {
    id: 'learning',
    name: 'Learning',
    description: 'We are learning lots of interesting stuff all the time',
    goodDescription: 'We are learning lots of interesting stuff all the time',
    badDescription: 'We never have time to learn anything'
  },
  {
    id: 'support',
    name: 'Support',
    description: 'We always get great support & help when we ask for it',
    goodDescription: 'We always get great support & help when we ask for it',
    badDescription: 'We keep getting stuck because we cannot get the support & help that we ask for'
  },
  {
    id: 'pawns',
    name: 'Pawns or Players',
    description: 'We are in control of our destiny! We decide what to build and how to build it',
    goodDescription: 'We are in control of our destiny! We decide what to build and how to build it',
    badDescription: 'We are just pawns in a game of chess, with no influence over what we build or how we build it'
  },
  {
    id: 'release',
    name: 'Easy to Release',
    description: 'Releasing is simple, safe, painless and mostly automated',
    goodDescription: 'Releasing is simple, safe, painless and mostly automated',
    badDescription: 'Releasing is risky, painful, lots of manual work, and takes forever'
  },
  {
    id: 'process',
    name: 'Suitable Process',
    description: 'Our way of working fits us perfectly',
    goodDescription: 'Our way of working fits us perfectly',
    badDescription: 'Our way of working sucks'
  },
  {
    id: 'teamwork',
    name: 'Teamwork',
    description: 'We are a tight-knit team that works together really well',
    goodDescription: 'We are a tight-knit team that works together really well',
    badDescription: 'We are a bunch of individuals that neither know nor care about what the others are doing'
  }
];
