-- SPDX-License-Identifier: Apache-2.0

-- Create health_dimensions table
CREATE TABLE IF NOT EXISTS health_dimensions (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    good_description TEXT NOT NULL,
    bad_description TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    weight NUMERIC(3,2) DEFAULT 1.00,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index for active dimensions
CREATE INDEX idx_dimensions_active ON health_dimensions(is_active) WHERE is_active = true;

-- Add comment for documentation
COMMENT ON TABLE health_dimensions IS 'Health check dimensions based on Spotify Squad Health Check Model';
COMMENT ON COLUMN health_dimensions.id IS 'Unique identifier for dimension (e.g., mission, value, speed)';
COMMENT ON COLUMN health_dimensions.weight IS 'Weight for aggregated scoring (default 1.00)';
