#!/bin/bash
# SPDX-License-Identifier: Apache-2.0

# Script to start development server on host machine with SSL bypass

echo "Starting Team360 Health Check Application..."
echo "----------------------------------------"

# Set environment variable to bypass SSL certificate check
export NODE_TLS_REJECT_UNAUTHORIZED=0

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

# Start the development server
echo "Starting Next.js development server..."
echo "Application will be available at http://localhost:3000"
echo ""
echo "Login credentials:"
echo "  Team Member: demo/demo"
echo "  Manager: manager/manager"
echo "  Admin: admin/admin"
echo ""
npm run dev