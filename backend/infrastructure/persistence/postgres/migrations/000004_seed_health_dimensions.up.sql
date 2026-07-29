-- SPDX-License-Identifier: Apache-2.0

-- Seed health dimensions based on Spotify Squad Health Check Model
-- Enhanced with Team360 additions

INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight) VALUES
    ('mission', 'Mission', 'We know exactly why we are here, and we are really excited about it',
     'We know exactly why we are here, and we are really excited about it',
     'We have no idea why we are here. There is no high level picture or focus.',
     true, 1.00),

    ('value', 'Delivering Value', 'We deliver great stuff! We are proud of it and our stakeholders are really happy',
     'We deliver great stuff! We are proud of it and our stakeholders are really happy',
     'We deliver crap. We are ashamed to deliver it. Our stakeholders hate us.',
     true, 1.00),

    ('speed', 'Speed', 'We get stuff done really quickly. No waiting, no delays',
     'We get stuff done really quickly. No waiting, no delays',
     'We never seem to get anything done. We keep getting stuck or interrupted.',
     true, 1.00),

    ('fun', 'Fun', 'We love going to work, and have great fun working together',
     'We love going to work, and have great fun working together',
     'Boooooooring',
     true, 1.00),

    ('health', 'Health of Codebase', 'Our code is clean, easy to read, and has great test coverage',
     'Our code is clean, easy to read, and has great test coverage',
     'Our code is a pile of dung, and technical debt is raging out of control',
     true, 1.00),

    ('learning', 'Learning', 'We are learning lots of interesting stuff all the time',
     'We are learning lots of interesting stuff all the time',
     'We never have time to learn anything',
     true, 1.00),

    ('support', 'Support', 'We always get great support & help when we ask for it',
     'We always get great support & help when we ask for it',
     'We keep getting stuck because we cannot get the support & help that we ask for',
     true, 1.00),

    ('pawns', 'Pawns or Players', 'We are in control of our destiny! We decide what to build and how to build it',
     'We are in control of our destiny! We decide what to build and how to build it',
     'We are just pawns in a game of chess, with no influence over what we build or how we build it',
     true, 1.00),

    ('release', 'Easy to Release', 'Releasing is simple, safe, painless and mostly automated',
     'Releasing is simple, safe, painless and mostly automated',
     'Releasing is risky, painful, lots of manual work, and takes forever',
     true, 1.00),

    ('process', 'Suitable Process', 'Our way of working fits us perfectly',
     'Our way of working fits us perfectly',
     'Our way of working sucks',
     true, 1.00),

    ('teamwork', 'Teamwork', 'We are a tight-knit team that works together really well',
     'We are a tight-knit team that works together really well',
     'We are a bunch of individuals that neither know nor care about what the others are doing',
     true, 1.00);

-- Verify all dimensions were inserted
DO $$
DECLARE
    dimension_count INT;
BEGIN
    SELECT COUNT(*) INTO dimension_count FROM health_dimensions;
    IF dimension_count != 11 THEN
        RAISE EXCEPTION 'Expected 11 health dimensions, but found %', dimension_count;
    END IF;
END $$;
