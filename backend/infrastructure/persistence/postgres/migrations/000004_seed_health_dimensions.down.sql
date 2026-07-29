-- SPDX-License-Identifier: Apache-2.0

-- Remove seeded health dimensions
DELETE FROM health_dimensions WHERE id IN (
    'mission', 'value', 'speed', 'fun', 'health',
    'learning', 'support', 'pawns', 'release', 'process', 'teamwork'
);
