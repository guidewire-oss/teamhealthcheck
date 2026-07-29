-- SPDX-License-Identifier: Apache-2.0

-- Remove demo users
DELETE FROM users WHERE id IN (
    'vp1', 'dir1', 'dir2', 'mgr1', 'mgr2', 'mgr3',
    'lead1', 'lead2', 'lead3', 'lead4', 'lead5',
    'member1', 'member2', 'member3', 'member4', 'member5', 'demo',
    'admin1'
);
