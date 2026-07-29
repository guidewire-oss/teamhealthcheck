#!/bin/bash
# SPDX-License-Identifier: Apache-2.0


echo "🔧 Team360 Health Check - Mac ARM64 Fix Script"
echo "=============================================="

# Check if we're on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    echo "❌ This script is designed for macOS only"
    exit 1
fi

# Check if we're on ARM64 (Apple Silicon)
if [[ $(uname -m) != "arm64" ]]; then
    echo "⚠️  This script is optimized for Apple Silicon Macs"
    echo "   It may still help with Intel Macs"
fi

echo ""
echo "🧹 Step 1: Cleaning cache and build artifacts..."
npm cache clean --force
rm -rf node_modules package-lock.json .next

echo ""
echo "📦 Step 2: Reinstalling dependencies..."
npm install

echo ""
echo "🔄 Step 3: Checking for SWC issues..."
if npm list @next/swc-darwin-arm64 2>/dev/null; then
    echo "✅ SWC binary found, attempting to reinstall..."
    npm uninstall @next/swc-darwin-arm64
    npm install --force @next/swc-darwin-arm64
else
    echo "⚠️  SWC binary not found, installing..."
    npm install --force @next/swc-darwin-arm64
fi

echo ""
echo "🔧 Step 4: Setting up alternative build configuration..."

# Create .babelrc.js as fallback
cat > .babelrc.js << 'EOF'
module.exports = {
  presets: ['next/babel'],
}
EOF

echo "✅ Created Babel fallback configuration"

echo ""
echo "🌍 Step 5: Setting environment variables..."
echo "export NODE_TLS_REJECT_UNAUTHORIZED=0" >> ~/.zshrc
echo "export NODE_TLS_REJECT_UNAUTHORIZED=0" >> ~/.bash_profile

echo ""
echo "✨ Setup complete! Try running:"
echo "   npm run dev"
echo ""
echo "If you still see SWC errors, the app will fallback to Babel."
echo "If you see module resolution errors, make sure you're in the project root."
echo ""
echo "🚀 Happy coding!"